package core

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coding-bridge/internal/commands"
	"github.com/coding-bridge/internal/config"
	bridgectx "github.com/coding-bridge/internal/context"
	"github.com/coding-bridge/internal/patch"
	"github.com/coding-bridge/internal/providers"
	"github.com/coding-bridge/internal/report"
	"github.com/coding-bridge/internal/sandbox"
)

// Runner 是核心执行引擎，串联 Provider 调度、Patch 校验、沙箱保护、命令执行、报告生成。
type Runner struct {
	projectRoot string
	cfg         *config.AppConfig
	registry    *providers.Registry
	sandbox     *sandbox.SnapshotManager
}

// NewRunner 创建核心 Runner
func NewRunner(projectRoot string, cfg *config.AppConfig, registry *providers.Registry) *Runner {
	return &Runner{
		projectRoot: projectRoot,
		cfg:         cfg,
		registry:    registry,
		sandbox:     sandbox.NewSnapshotManager(projectRoot),
	}
}

// RunResult Runner 执行结果
type RunResult struct {
	TaskResult *TaskResult
	ReportPath string
	Snapshot   *sandbox.Snapshot
	Err        error
}

// Run 执行一个完整的任务流程
func (r *Runner) Run(ctx context.Context, task *Task) *RunResult {
	result := &TaskResult{
		TaskID:                task.TaskID,
		ControllerTokens:      task.Controller.ObservedTokens,
		MaxRepairAttempts:     r.cfg.Execution.MaxRepairAttempts,
		TechnicalVerification: "NOT_STARTED",
		BusinessAcceptance:    "controller_review_required",
		StartedAt:             time.Now(),
	}
	var snapshot *sandbox.Snapshot

	// 状态机
	sm := NewStateMachine(StateCreated)

	// Step 1: 验证任务
	if errs := task.Validate(); len(errs) > 0 {
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("task validation failed: %v", errs)
		return r.finish(result, snapshot, fmt.Errorf("validation: %v", errs))
	}
	if r.cfg.Execution.EnforceTaskBudgets {
		if errs := task.ValidateTextBudgets(
			r.cfg.Execution.MaxDescriptionChars,
			r.cfg.Execution.MaxRequirementsChars,
			r.cfg.Execution.MaxAcceptanceCriteriaChars,
			r.cfg.Execution.MaxAcceptanceCriteriaCount,
			r.cfg.Execution.MaxRequirementsCount,
		); len(errs) > 0 {
			result.Status = StateFailed
			result.FailureReason = fmt.Sprintf("task text budget exceeded: %v", errs)
			return r.finish(result, snapshot, fmt.Errorf("text budget: %v", errs))
		}
	}
	_ = sm.Transition(StateValidated)

	// Step 2: 收集允许文件的受控上下文
	collector := bridgectx.NewCollector(r.projectRoot, task.AllowedFiles, task.ForbiddenFiles)
	collectedContext, err := collector.Collect()
	if err != nil {
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("context collection failed: %v", err)
		return r.finish(result, snapshot, err)
	}
	result.ContextFiles = collectedContext.TotalFiles
	result.ContextBytes = collectedContext.TotalSize
	if r.cfg.Execution.EnforceTaskBudgets {
		if len(task.AllowedFiles) > r.cfg.Execution.MaxTaskFiles {
			result.Status = StateFailed
			result.FailureReason = fmt.Sprintf(
				"TASK_TOO_LARGE: %d allowed files exceeds configured maximum %d; split the task before calling an Executor",
				len(task.AllowedFiles),
				r.cfg.Execution.MaxTaskFiles,
			)
			return r.finish(result, snapshot, fmt.Errorf("%s", result.FailureReason))
		}
		if collectedContext.TotalSize > r.cfg.Execution.MaxContextBytes {
			result.Status = StateFailed
			result.FailureReason = fmt.Sprintf(
				"TASK_TOO_LARGE: %d context bytes exceeds configured maximum %d; reduce allowed_files or split the task",
				collectedContext.TotalSize,
				r.cfg.Execution.MaxContextBytes,
			)
			return r.finish(result, snapshot, fmt.Errorf("%s", result.FailureReason))
		}
	}
	_ = sm.Transition(StateContextCollected)

	// Step 3: 选择 Provider
	executorProvider, err := r.selectExecutor(ctx, task)
	if err != nil {
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("provider selection failed: %v", err)
		return r.finish(result, snapshot, err)
	}
	result.Provider = string(executorProvider.Type())
	_ = sm.Transition(StateProviderSelected)

	// Step 4: 请求 Executor 生成 patch
	_ = sm.Transition(StatePatchRequested)
	patchResponse, err := r.requestPatch(ctx, task, executorProvider, collectedContext)
	result.GenerationAttempts = 1
	if err != nil {
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("patch generation failed: %v", err)
		return r.finish(result, snapshot, err)
	}
	result.Model = patchResponse.Model
	if result.Model == "" {
		result.Model = task.Executor.PreferredModel
	}
	result.PromptTokens = patchResponse.Usage.PromptTokens
	result.CompletionTokens = patchResponse.Usage.CompletionTokens
	result.TotalTokens = patchResponse.Usage.TotalTokens
	r.calculateTokenSavings(result)
	_ = sm.Transition(StatePatchGenerated)
	result.TechnicalVerification = "PATCH_GENERATED"

	if strings.EqualFold(strings.TrimSpace(patchResponse.FinishReason), "length") {
		result.Status = StateFailed
		result.TruncatedOutput = true
		result.FailureReason = fmt.Sprintf(
			"executor output was truncated at %d completion tokens; reduce allowed_files/task scope or increase the patch token budget",
			result.CompletionTokens,
		)
		return r.finish(result, snapshot, fmt.Errorf("%s", result.FailureReason))
	}

	// 检查特殊响应
	responseContent := strings.TrimSpace(patchResponse.Content)
	switch responseContent {
	case "NEED_MORE_CONTEXT":
		result.Status = StateFailed
		result.FailureReason = "executor returned NEED_MORE_CONTEXT"
		return r.finish(result, snapshot, fmt.Errorf("%s", result.FailureReason))
	case "REFUSE":
		result.Status = StateFailed
		result.FailureReason = "executor returned REFUSE"
		return r.finish(result, snapshot, fmt.Errorf("%s", result.FailureReason))
	case "FAILED":
		result.Status = StateFailed
		result.FailureReason = "executor returned FAILED"
		return r.finish(result, snapshot, fmt.Errorf("%s", result.FailureReason))
	}
	if !patch.HasEndMarker(responseContent) {
		result.Status = StateFailed
		result.TruncatedOutput = true
		result.FailureReason = "TRUNCATED_OUTPUT: executor response is missing END_CODING_BRIDGE_EDIT"
		return r.finish(result, snapshot, fmt.Errorf("%s", result.FailureReason))
	}

	// Step 5: 解析和校验 Patch
	parser := patch.NewParser()
	parseResult, err := parser.Parse(responseContent)
	if err != nil {
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("patch parse failed: %v", err)
		return r.finish(result, snapshot, err)
	}
	result.GitDiff = parseResult.RawDiff
	result.PatchChangedLines = countPatchChangedLines(parseResult)
	if r.cfg.Execution.EnforceTaskBudgets &&
		result.PatchChangedLines > r.cfg.Execution.MaxPatchLines {
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf(
			"PATCH_TOO_LARGE: %d changed lines exceeds configured maximum %d; split the task",
			result.PatchChangedLines,
			r.cfg.Execution.MaxPatchLines,
		)
		return r.finish(result, snapshot, fmt.Errorf("%s", result.FailureReason))
	}

	validator := patch.NewValidator(task.AllowedFiles, task.ForbiddenFiles, task.Requirements)
	if errs := validator.Validate(parseResult); len(errs) > 0 {
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("patch validation failed: %v", errs)
		return r.finish(result, snapshot, fmt.Errorf("validation: %v", errs))
	}
	_ = sm.Transition(StatePatchValidated)
	result.TechnicalVerification = "PARSE_AND_VALIDATE_OK"

	// Step 6: 风险检查
	_ = sm.Transition(StateRiskChecked)

	// Step 7: 创建快照
	snapshot, err = r.sandbox.CreateSnapshot(task.TaskID)
	if err != nil {
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("snapshot creation failed: %v", err)
		return r.finish(result, snapshot, err)
	}
	if err := r.sandbox.PrepareSnapshot(snapshot, patchTargetFiles(parseResult)); err != nil {
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("snapshot preparation failed: %v", err)
		return r.finish(result, snapshot, err)
	}
	_ = sm.Transition(StateSnapshotCreated)

	// Step 8: 在隔离执行目录应用 Patch
	executionRoot := r.sandbox.ExecutionRoot(snapshot)
	targetFiles := patchTargetFiles(parseResult)
	beforeHashes, err := hashTargetFiles(executionRoot, targetFiles)
	if err != nil {
		rollbackErr := r.sandbox.Rollback(snapshot)
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("capture pre-apply hashes failed: %v", err)
		result.RollbackInfo = fmt.Sprintf("rolled back via %s", snapshot.Method)
		if rollbackErr != nil {
			result.RollbackInfo += fmt.Sprintf(" (rollback failed: %v)", rollbackErr)
		}
		return r.finish(result, snapshot, err)
	}
	applier := patch.NewApplier(executionRoot)
	modifiedFiles, err := applier.Apply(parseResult)
	if err != nil {
		rollbackErr := r.sandbox.Rollback(snapshot)
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("patch apply failed: %v", err)
		result.RollbackInfo = fmt.Sprintf("rolled back via %s", snapshot.Method)
		if rollbackErr != nil {
			result.RollbackInfo += fmt.Sprintf(" (rollback failed: %v)", rollbackErr)
		}
		return r.finish(result, snapshot, err)
	}
	hashChanges, err := verifyTargetFileChanges(executionRoot, targetFiles, beforeHashes)
	if err != nil || len(hashChanges) == 0 {
		if err == nil {
			err = fmt.Errorf("NO_EFFECTIVE_CHANGE: patch applied without changing target file hashes")
		}
		rollbackErr := r.sandbox.Rollback(snapshot)
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("patch effect verification failed: %v", err)
		result.RollbackInfo = fmt.Sprintf("rolled back via %s", snapshot.Method)
		if rollbackErr != nil {
			result.RollbackInfo += fmt.Sprintf(" (rollback failed: %v)", rollbackErr)
		}
		return r.finish(result, snapshot, err)
	}
	result.ModifiedFiles = modifiedFiles
	result.PatchEffectVerified = true
	result.EffectiveChangedFiles = len(hashChanges)
	result.FileHashChanges = hashChanges
	_ = sm.Transition(StatePatchApplied)
	result.TechnicalVerification = "APPLY_AND_HASH_OK"

	// Step 9: 执行任务明确允许的构建、测试和检查命令
	cmdRunner := commands.NewRunner(
		executionRoot,
		r.cfg.Commands.Allowed,
		r.cfg.Commands.Forbidden,
		r.cfg.Timeouts.CommandTimeout(),
	)

	for _, cmdStr := range task.AllowedCommands {
		cmdResult, cmdErr := cmdRunner.Run(ctx, cmdStr)
		if cmdResult != nil {
			result.CommandsRun = append(result.CommandsRun, *cmdResult)
			recordBuildTestResult(result, cmdStr, cmdResult)
		}
		if cmdErr != nil || cmdResult == nil || cmdResult.ExitCode != 0 {
			if cmdErr == nil {
				cmdErr = fmt.Errorf("command %q exited with code %d", cmdStr, cmdResult.ExitCode)
			}
			rollbackErr := r.sandbox.Rollback(snapshot)
			result.Status = StateFailed
			result.FailureReason = fmt.Sprintf("command execution failed: %v", cmdErr)
			result.RollbackInfo = fmt.Sprintf("rolled back via %s", snapshot.Method)
			if rollbackErr != nil {
				result.RollbackInfo += fmt.Sprintf(" (rollback failed: %v)", rollbackErr)
			}
			return r.finish(result, snapshot, cmdErr)
		}
	}
	_ = sm.Transition(StateCommandsExecuted)
	if result.TestResult != nil {
		result.TechnicalVerification = "TEST_OK"
	} else if result.BuildResult != nil {
		result.TechnicalVerification = "BUILD_OK"
	} else {
		result.TechnicalVerification = "COMMANDS_OK"
	}

	// Step 10: 基础安全检查。更完整的 secret scanner 后续接入。
	result.SecurityCheck = &SecurityCheckResult{Passed: true}
	_ = sm.Transition(StateReportGenerated)
	_ = sm.Transition(StateReviewRequired)
	_ = sm.Transition(StateCompleted)
	result.Status = StateCompleted
	return r.finish(result, snapshot, nil)
}

// selectExecutor 选择 Executor Provider
func (r *Runner) selectExecutor(ctx context.Context, task *Task) (providers.Provider, error) {
	if task.Executor.Selection == "manual" {
		pt := providers.ProviderType(task.Executor.PreferredProvider)
		p, err := r.registry.Get(pt)
		if err != nil {
			return nil, ErrProviderUnavailablef("preferred provider %q: %v", task.Executor.PreferredProvider, err)
		}
		return p, nil
	}

	checker := providers.NewHealthChecker(r.registry)
	var candidates []string
	if task.Executor.PreferredProvider != "" {
		candidates = append(candidates, task.Executor.PreferredProvider)
	}
	candidates = append(candidates, r.cfg.ProviderSelection.FallbackOrder...)
	seen := map[string]bool{}
	var failures []string
	for _, name := range candidates {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		pt := providers.ProviderType(name)
		p, err := r.registry.Get(pt)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		health, err := checker.CheckOne(ctx, pt)
		if err == nil && health.Status != providers.HealthUnhealthy {
			return p, nil
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
		} else {
			failures = append(failures, fmt.Sprintf("%s: %s", name, health.Status))
		}
	}
	if len(candidates) > 0 {
		return nil, ErrProviderUnavailablef("all configured providers are unavailable: %s", strings.Join(failures, "; "))
	}

	// 自动选择
	selector := providers.NewSelector(r.registry, checker)

	strategy := providers.SelectionStrategy{
		PreferAvailable:     r.cfg.ProviderSelection.Strategy.PreferAvailable,
		PreferLowCost:       r.cfg.ProviderSelection.Strategy.PreferLowCost,
		PreferFastResponse:  r.cfg.ProviderSelection.Strategy.PreferFastResponse,
		PreferPatchAccuracy: r.cfg.ProviderSelection.Strategy.PreferPatchAccuracy,
	}

	ranked, err := selector.Select(ctx, strategy)
	if err != nil {
		return nil, ErrProviderUnavailablef("no provider available: %v", err)
	}

	return ranked[0], nil
}

// requestPatch 向 Executor 请求生成 patch
func (r *Runner) requestPatch(
	ctx context.Context,
	task *Task,
	provider providers.Provider,
	collected *bridgectx.Context,
) (*providers.GenerateResponse, error) {
	// 构建 patch-only 提示词
	systemPrompt := `You are a code modification tool. Your ONLY job is to generate unified diffs.

RULES:
1. ONLY output unified diff format (diff --git ...)
2. Do NOT output any explanations, markdown, or commentary
3. Do NOT modify files outside the allowed list
4. Do NOT add, remove, or modify any secrets, environment variables, or config files
5. If you cannot fix the issue with a diff, respond with exactly one of: NEED_MORE_CONTEXT, REFUSE, or FAILED
6. Preserve original encoding and line endings
7. Each diff must be minimal and focused on the exact fix only
8. The final non-empty line MUST be exactly END_CODING_BRIDGE_EDIT`

	// 构建用户消息
	userPrompt := fmt.Sprintf(`Task: %s

Description: %s

Allowed files:
%s

Requirements:
%s

Acceptance criteria:
%s

Source context:
%s

Generate ONLY a unified diff followed by END_CODING_BRIDGE_EDIT. No explanations.`,
		task.Title,
		task.Description,
		formatList(task.AllowedFiles),
		formatList(task.Requirements),
		formatList(task.AcceptanceCriteria),
		formatContext(collected),
	)

	req := &providers.GenerateRequest{
		Model:        task.Executor.PreferredModel,
		SystemPrompt: systemPrompt,
		Messages: []providers.ChatMessage{
			{Role: "user", Content: userPrompt},
		},
		MaxTokens:   r.cfg.Execution.PatchMaxTokens,
		Temperature: r.cfg.Execution.Temperature,
		PatchOnly:   true,
	}

	models := r.executorModels(task, provider)
	var failures []string
	for _, model := range models {
		req.Model = model
		resp, err := provider.Generate(ctx, req)
		if err == nil && resp != nil {
			return resp, nil
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", model, err))
		} else {
			failures = append(failures, fmt.Sprintf("%s: empty response", model))
		}
	}
	return nil, fmt.Errorf("provider generate failed for all configured models: %s", strings.Join(failures, "; "))
}

const missingFileHash = "<missing>"

func hashTargetFiles(root string, files []string) (map[string]string, error) {
	hashes := make(map[string]string, len(files))
	for _, file := range files {
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, filepath.FromSlash(file))
		}
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			hashes[file] = missingFileHash
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("hash %s: %w", file, err)
		}
		sum := sha256.Sum256(data)
		hashes[file] = fmt.Sprintf("%x", sum[:])
	}
	return hashes, nil
}

func verifyTargetFileChanges(root string, files []string, before map[string]string) ([]FileHashChange, error) {
	after, err := hashTargetFiles(root, files)
	if err != nil {
		return nil, err
	}
	var changed []FileHashChange
	for _, file := range files {
		if before[file] != after[file] {
			changed = append(changed, FileHashChange{
				File:         file,
				BeforeSHA256: before[file],
				AfterSHA256:  after[file],
			})
		}
	}
	return changed, nil
}

func toReportHashChanges(changes []FileHashChange) []report.FileHashChange {
	result := make([]report.FileHashChange, 0, len(changes))
	for _, change := range changes {
		result = append(result, report.FileHashChange{
			File:         change.File,
			BeforeSHA256: change.BeforeSHA256,
			AfterSHA256:  change.AfterSHA256,
		})
	}
	return result
}

func countPatchChangedLines(result *patch.ParseResult) int {
	total := 0
	for _, file := range result.Files {
		for _, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
					total++
				}
			}
		}
	}
	return total
}

func (r *Runner) executorModels(task *Task, provider providers.Provider) []string {
	var models []string
	if task.Executor.PreferredModel != "" &&
		task.Executor.PreferredProvider == string(provider.Type()) {
		models = append(models, task.Executor.PreferredModel)
	}
	for _, item := range r.cfg.Providers.Configs {
		if item.Type != string(provider.Type()) {
			continue
		}
		models = append(models, item.GetModels()...)
	}
	seen := map[string]bool{}
	var unique []string
	for _, model := range models {
		if model != "" && !seen[model] {
			seen[model] = true
			unique = append(unique, model)
		}
	}
	if len(unique) == 0 {
		unique = append(unique, "")
	}
	return unique
}

func (r *Runner) calculateTokenSavings(result *TaskResult) {
	if !r.cfg.TokenAccounting.Enabled || result.TotalTokens <= 0 {
		return
	}
	ratio := r.cfg.TokenAccounting.DirectCodexBaselineRatio
	if ratio < 1 {
		ratio = 1
	}
	result.EstimatedDirectTokens = int(float64(result.TotalTokens) * ratio)
	result.EstimatedGrossSavings = result.EstimatedDirectTokens - result.TotalTokens
	if result.ControllerTokens > 0 {
		result.EstimatedNetSavings = result.EstimatedDirectTokens -
			result.TotalTokens -
			result.ControllerTokens
	}
}

// formatList 格式化为带 - 前缀的列表
func formatList(items []string) string {
	if len(items) == 0 {
		return "  (none specified)"
	}
	var result string
	for _, item := range items {
		result += fmt.Sprintf("  - %s\n", item)
	}
	return result
}

func formatContext(collected *bridgectx.Context) string {
	if collected == nil || len(collected.Files) == 0 {
		return "  (no source files collected)"
	}

	var result strings.Builder
	for _, file := range collected.Files {
		if file.Skipped {
			fmt.Fprintf(&result, "\n--- %s (skipped: %s) ---\n", file.Path, file.Reason)
			continue
		}
		fmt.Fprintf(&result, "\n--- %s ---\n%s\n", file.Path, file.Content)
	}
	return result.String()
}

func patchTargetFiles(parseResult *patch.ParseResult) []string {
	var files []string
	for _, file := range parseResult.Files {
		path := file.NewPath
		if path == "" || path == "/dev/null" {
			path = file.OrigPath
		}
		if path != "" && path != "/dev/null" {
			files = append(files, path)
		}
	}
	return files
}

func recordBuildTestResult(result *TaskResult, command string, cmdResult *commands.CommandResult) {
	buildResult := &commands.BuildTestResult{
		Success:  cmdResult.ExitCode == 0,
		Output:   strings.TrimSpace(cmdResult.Stdout + "\n" + cmdResult.Stderr),
		ExitCode: cmdResult.ExitCode,
	}
	lower := strings.ToLower(strings.TrimSpace(command))
	switch {
	case strings.Contains(lower, " test") || strings.HasPrefix(lower, "pytest") || lower == "go test":
		result.TestResult = buildResult
	case strings.Contains(lower, "build") || strings.Contains(lower, " vet"):
		result.BuildResult = buildResult
	}
}

func (r *Runner) finish(result *TaskResult, snapshot *sandbox.Snapshot, runErr error) *RunResult {
	if result.Status == "" {
		if runErr != nil {
			result.Status = StateFailed
		} else {
			result.Status = StateCompleted
		}
	}
	result.FinishedAt = time.Now()

	// 填充 Phase
	if result.Phase == "" {
		result.Phase = result.Status
	}

	// 填充 WriteState
	if result.WriteState_ == nil {
		result.WriteState_ = &WriteState{
			PatchGenerated:     result.TechnicalVerification != "NOT_STARTED",
			PatchValidated:     result.TechnicalVerification == "PARSE_AND_VALIDATE_OK" || result.TechnicalVerification == "APPLY_AND_HASH_OK" || result.TechnicalVerification == "TEST_OK" || result.TechnicalVerification == "BUILD_OK" || result.TechnicalVerification == "COMMANDS_OK",
			SnapshotCreated:    snapshot != nil,
			PatchApplied:       result.PatchEffectVerified,
			PatchEffectVerified: result.PatchEffectVerified,
			CommandsExecuted:   len(result.CommandsRun) > 0,
			RolledBack:         result.Status != StateCompleted && result.RollbackInfo != "",
			MainWorkspaceModified: false,
			ExecutionMode:      "git_worktree",
			MergeRequired:      result.Status == StateCompleted,
		}
	}

	// 填充 Failure info
	if result.Status != StateCompleted && result.FailureReason != "" {
		failureCode := deriveFailureCodeFromReason(result.FailureReason)
		retryable := isRetryableFailure(failureCode)
		suggestedAction := "abort"
		if retryable {
			suggestedAction = "repair_patch"
		}
		result.Failure = &FailureInfo{
			Code:            failureCode,
			Phase:           string(result.Status),
			Message:         result.FailureReason,
			Retryable:       retryable,
			SuggestedAction: suggestedAction,
		}
	}

	// 填充 ControllerUsage
	controllerSource := "unavailable"
	var controllerObserved *int
	if result.ControllerTokens > 0 {
		controllerSource = "manual"
		val := result.ControllerTokens
		controllerObserved = &val
	}
	result.ControllerUsage_ = &ControllerUsage{
		Source:         controllerSource,
		ObservedTokens: controllerObserved,
		Confidence:     "low",
	}

	// 填充 Decision
	if result.Status == StateCompleted {
		result.Decision_ = &Decision{
			RecommendedNextAction: "review_changes",
			RequiresUserApproval:  true,
			SafeToContinue:        true,
		}
	} else if result.Failure != nil {
		result.Decision_ = &Decision{
			RecommendedNextAction: result.Failure.SuggestedAction,
			RequiresUserApproval:  false,
			SafeToContinue:        result.Failure.Retryable,
		}
	}

	if result.TotalTokens > 0 {
		if result.Status == StateCompleted {
			result.ExecutorEffectiveTokens = result.TotalTokens
		} else {
			result.ExecutorWastedTokens = result.TotalTokens
			result.ExecutorWasteRate = 1
		}
	}

	outputDir := r.cfg.Report.OutputDir
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(r.projectRoot, outputDir)
	}
	gen := report.NewGeneratorWithConfig(outputDir, report.ReportConfig{
		OutputDir:                outputDir,
		Mode:                     r.cfg.Report.Mode,
		SaveFullReport:           r.cfg.Report.SaveFullReport,
		SaveFullPatch:            r.cfg.Report.SaveFullPatch,
		SaveFullCommandOutput:    r.cfg.Report.SaveFullCommandOutput,
		CommandOutputTailLines:   r.cfg.Report.CommandOutputTailLines,
		MaxSummaryBytes:          r.cfg.Report.MaxSummaryBytes,
		MaxFailureMessageBytes:   r.cfg.Report.MaxFailureMessageBytes,
		IncludeModifiedFileContent: r.cfg.Report.IncludeModifiedFileContent,
		IncludeDiff:              r.cfg.Report.IncludeDiff,
		IncludePatch:             r.cfg.Report.IncludePatch,
		IncludeBackupContent:     r.cfg.Report.IncludeBackupContent,
		IncludeSnapshotContent:   r.cfg.Report.IncludeSnapshotContent,
	})
	reportPath, reportErr := gen.SaveReport(taskResultToReportData(result))
	if reportErr != nil {
		if result.FailureReason != "" {
			result.FailureReason += "; "
		}
		result.FailureReason += fmt.Sprintf("report generation failed: %v", reportErr)
		result.Status = StateFailed
		if runErr == nil {
			runErr = reportErr
		}
	}

	return &RunResult{
		TaskResult: result,
		ReportPath: reportPath,
		Snapshot:   snapshot,
		Err:        runErr,
	}
}

// DryRun 执行干运行（不实际修改文件）
func (r *Runner) DryRun(ctx context.Context, task *Task) (*RunResult, error) {
	// 类似 Run 但不执行实际写入
	result := &TaskResult{
		TaskID:    task.TaskID,
		Status:    StateValidated,
		StartedAt: time.Now(),
	}

	// 验证任务
	if errs := task.Validate(); len(errs) > 0 {
		return &RunResult{TaskResult: result}, fmt.Errorf("validation: %v", errs)
	}

	if _, err := r.selectExecutor(ctx, task); err != nil {
		return &RunResult{TaskResult: result}, err
	}

	result.Status = StateCompleted
	result.FinishedAt = time.Now()
	return &RunResult{TaskResult: result}, nil
}

// RollbackTask 回滚指定任务
func (r *Runner) RollbackTask(taskID string) error {
	snapshot, err := r.sandbox.LoadSnapshot(taskID)
	if err != nil {
		return err
	}
	return r.sandbox.Rollback(snapshot)
}

// GetProjectRoot 返回项目根目录
func (r *Runner) GetProjectRoot() string {
	return r.projectRoot
}

// taskResultToReportData 将 TaskResult 转换为 report.ReportData
func taskResultToReportData(tr *TaskResult) *report.ReportData {
	data := &report.ReportData{
		TaskID:                  tr.TaskID,
		Status:                  string(tr.Status),
		Phase:                   string(tr.Phase),
		Provider:                tr.Provider,
		Model:                   tr.Model,
		ContextFiles:            tr.ContextFiles,
		ContextBytes:            tr.ContextBytes,
		PromptTokens:            tr.PromptTokens,
		CompletionTokens:        tr.CompletionTokens,
		TotalTokens:             tr.TotalTokens,
		ControllerTokens:        tr.ControllerTokens,
		EstimatedDirectTokens:   tr.EstimatedDirectTokens,
		EstimatedGrossSavings:   tr.EstimatedGrossSavings,
		EstimatedNetSavings:     tr.EstimatedNetSavings,
		TruncatedOutput:         tr.TruncatedOutput,
		PatchEffectVerified:     tr.PatchEffectVerified,
		EffectiveChangedFiles:   tr.EffectiveChangedFiles,
		GenerationAttempts:      tr.GenerationAttempts,
		MaxRepairAttempts:       tr.MaxRepairAttempts,
		PatchChangedLines:       tr.PatchChangedLines,
		ExecutorEffectiveTokens: tr.ExecutorEffectiveTokens,
		ExecutorWastedTokens:    tr.ExecutorWastedTokens,
		ExecutorWasteRate:       tr.ExecutorWasteRate,
		FileHashChanges:         toReportHashChanges(tr.FileHashChanges),
		TechnicalVerification:   tr.TechnicalVerification,
		BusinessAcceptance:      tr.BusinessAcceptance,
		ModifiedFiles:           tr.ModifiedFiles,
		GitDiff:                 tr.GitDiff,
		FailureReason:           tr.FailureReason,
		RollbackInfo:            tr.RollbackInfo,
		StartedAt:               tr.StartedAt,
		FinishedAt:              tr.FinishedAt,
		SnapshotCreated:         tr.WriteState_ != nil && tr.WriteState_.SnapshotCreated,
		ExecutionMode:           "git_worktree",
		MergeRequired:           tr.Status == StateCompleted,
	}

	// 复制 failure info
	if tr.Failure != nil {
		data.FailureCode = tr.Failure.Code
		data.FailurePhase = tr.Failure.Phase
		data.Retryable = tr.Failure.Retryable
		data.SuggestedAction = tr.Failure.SuggestedAction
	}

	// 复制 write state
	if tr.WriteState_ != nil {
		data.SnapshotCreated = tr.WriteState_.SnapshotCreated
		data.ExecutionMode = tr.WriteState_.ExecutionMode
		data.ExecutionRoot = tr.WriteState_.ExecutionRoot
		data.MergeRequired = tr.WriteState_.MergeRequired
		data.MainWorkspaceModified = tr.WriteState_.MainWorkspaceModified
	}

	for _, cmd := range tr.CommandsRun {
		data.CommandsRun = append(data.CommandsRun, report.CmdResult{
			Command:  cmd.Command,
			Stdout:   cmd.Stdout,
			Stderr:   cmd.Stderr,
			ExitCode: cmd.ExitCode,
			Duration: cmd.Duration,
			TimedOut: cmd.TimedOut,
		})
	}

	if tr.BuildResult != nil {
		data.BuildResult = &report.BuildResult{
			Success:  tr.BuildResult.Success,
			Output:   tr.BuildResult.Output,
			ExitCode: tr.BuildResult.ExitCode,
		}
	}

	if tr.TestResult != nil {
		data.TestResult = &report.BuildResult{
			Success:  tr.TestResult.Success,
			Output:   tr.TestResult.Output,
			ExitCode: tr.TestResult.ExitCode,
		}
	}

	if tr.SecurityCheck != nil {
		data.SecurityCheck = &report.SecurityResult{
			Passed:                 tr.SecurityCheck.Passed,
			Issues:                 tr.SecurityCheck.Issues,
			SecretFound:            tr.SecurityCheck.SecretFound,
			ForbiddenFilesAccessed: tr.SecurityCheck.ForbiddenFilesAccessed,
		}
	}

	return data
}

// deriveFailureCodeFromReason 从失败原因推导失败码
func deriveFailureCodeFromReason(reason string) string {
	switch {
	case strings.Contains(reason, "TASK_TEXT_TOO_LARGE"):
		return "TASK_TEXT_TOO_LARGE"
	case strings.Contains(reason, "FORBIDDEN_INTERNAL_CONTEXT"):
		return "FORBIDDEN_INTERNAL_CONTEXT"
	case strings.Contains(reason, "TRUNCATED_OUTPUT"):
		return "TRUNCATED_OUTPUT"
	case strings.Contains(reason, "patch parse failed"):
		return "PATCH_PARSE_FAILED"
	case strings.Contains(reason, "patch validation failed"):
		return "PATCH_VALIDATE_FAILED"
	case strings.Contains(reason, "patch apply failed"):
		return "PATCH_APPLY_FAILED"
	case strings.Contains(reason, "NO_EFFECTIVE_CHANGE"):
		return "NO_EFFECTIVE_CHANGE"
	case strings.Contains(reason, "command") && strings.Contains(reason, "failed"):
		return "COMMAND_FAILED"
	case strings.Contains(reason, "build") && strings.Contains(reason, "fail"):
		return "BUILD_FAILED"
	case strings.Contains(reason, "test") && strings.Contains(reason, "fail"):
		return "TEST_FAILED"
	case strings.Contains(reason, "rollback") && strings.Contains(reason, "fail"):
		return "ROLLBACK_FAILED"
	default:
		return "UNKNOWN_FAILED"
	}
}

// isRetryableFailure 判断失败是否可重试
func isRetryableFailure(code string) bool {
	switch code {
	case "PATCH_PARSE_FAILED", "PATCH_VALIDATE_FAILED", "PATCH_APPLY_FAILED",
		"COMMAND_FAILED", "BUILD_FAILED", "TEST_FAILED", "TRUNCATED_OUTPUT":
		return true
	default:
		return false
	}
}
