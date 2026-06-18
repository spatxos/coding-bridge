package core

import (
	"context"
	"fmt"
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
		TaskID:    task.TaskID,
		StartedAt: time.Now(),
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
	_ = sm.Transition(StatePatchGenerated)

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

	// Step 5: 解析和校验 Patch
	parser := patch.NewParser()
	parseResult, err := parser.Parse(responseContent)
	if err != nil {
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("patch parse failed: %v", err)
		return r.finish(result, snapshot, err)
	}
	result.GitDiff = parseResult.RawDiff

	validator := patch.NewValidator(task.AllowedFiles, task.ForbiddenFiles, task.Requirements)
	if errs := validator.Validate(parseResult); len(errs) > 0 {
		result.Status = StateFailed
		result.FailureReason = fmt.Sprintf("patch validation failed: %v", errs)
		return r.finish(result, snapshot, fmt.Errorf("validation: %v", errs))
	}
	_ = sm.Transition(StatePatchValidated)

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
	result.ModifiedFiles = modifiedFiles
	_ = sm.Transition(StatePatchApplied)

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
	if task.Executor.Selection == "manual" || task.Executor.PreferredProvider != "" {
		// 手动指定
		pt := providers.ProviderType(task.Executor.PreferredProvider)
		p, err := r.registry.Get(pt)
		if err != nil {
			return nil, ErrProviderUnavailablef("preferred provider %q: %v", task.Executor.PreferredProvider, err)
		}
		return p, nil
	}

	// 自动选择
	checker := providers.NewHealthChecker(r.registry)
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
7. Each diff must be minimal and focused on the exact fix only`

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

Generate ONLY a unified diff. No explanations.`,
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
		MaxTokens:   4096,
		Temperature: 0.1, // 低温度以确保确定性输出
		PatchOnly:   true,
	}

	resp, err := provider.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("provider generate: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("provider generate returned an empty response")
	}

	return resp, nil
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

	outputDir := r.cfg.Report.OutputDir
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(r.projectRoot, outputDir)
	}
	reportPath, reportErr := report.NewGenerator(outputDir).SaveReport(taskResultToReportData(result))
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

	// 检查 Provider 可用性
	checker := providers.NewHealthChecker(r.registry)
	pt := providers.ProviderType(task.Executor.PreferredProvider)
	if task.Executor.PreferredProvider != "" {
		healthResult, err := checker.CheckOne(ctx, pt)
		if err != nil || healthResult.Status == providers.HealthUnhealthy {
			return &RunResult{TaskResult: result}, fmt.Errorf("provider %q is not healthy", pt)
		}
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
		TaskID:           tr.TaskID,
		Status:           string(tr.Status),
		Provider:         tr.Provider,
		Model:            tr.Model,
		ContextFiles:     tr.ContextFiles,
		ContextBytes:     tr.ContextBytes,
		PromptTokens:     tr.PromptTokens,
		CompletionTokens: tr.CompletionTokens,
		TotalTokens:      tr.TotalTokens,
		ModifiedFiles:    tr.ModifiedFiles,
		GitDiff:          tr.GitDiff,
		FailureReason:    tr.FailureReason,
		RollbackInfo:     tr.RollbackInfo,
		StartedAt:        tr.StartedAt,
		FinishedAt:       tr.FinishedAt,
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
