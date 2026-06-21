// Package report 提供任务执行报告的生成功能。
// 每个任务完成后必须生成报告，Controller Model 依赖报告进行下一步决策。
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CmdResult 命令执行结果（不依赖 core 包）
type CmdResult struct {
	Command  string `json:"command"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration,omitempty"`
	TimedOut bool   `json:"timed_out"`
}

// BuildResult 构建/测试结果（不依赖 core 包）
type BuildResult struct {
	Success  bool   `json:"success"`
	Output   string `json:"output,omitempty"`
	ExitCode int    `json:"exit_code"`
}

// SecurityResult 安全检查结果（不依赖 core 包）
type SecurityResult struct {
	Passed                 bool     `json:"passed"`
	Issues                 []string `json:"issues,omitempty"`
	SecretFound            bool     `json:"secret_found"`
	ForbiddenFilesAccessed bool     `json:"forbidden_files_accessed"`
}

// FileHashChange 文件哈希变更
type FileHashChange struct {
	File         string `json:"file"`
	BeforeSHA256 string `json:"before_sha256"`
	AfterSHA256  string `json:"after_sha256"`
}

// ControllerUsage Controller 的 token 使用信息
type ControllerUsage struct {
	Source             string `json:"source"`
	ObservedTokens     *int   `json:"observed_tokens"`
	EstimatedTokensMin int    `json:"estimated_tokens_min"`
	EstimatedTokensMax int    `json:"estimated_tokens_max"`
	InputChars         int    `json:"input_chars"`
	OutputChars        int    `json:"output_chars"`
	Confidence         string `json:"confidence"`
}

// WriteState 写入状态摘要
type WriteState struct {
	PatchGenerated        bool   `json:"patch_generated"`
	PatchValidated        bool   `json:"patch_validated"`
	SnapshotCreated       bool   `json:"snapshot_created"`
	PatchApplied          bool   `json:"patch_applied"`
	PatchEffectVerified   bool   `json:"patch_effect_verified"`
	CommandsExecuted      bool   `json:"commands_executed"`
	RolledBack            bool   `json:"rolled_back"`
	MainWorkspaceModified bool   `json:"main_workspace_modified"`
	ExecutionMode         string `json:"execution_mode"`
	ExecutionRoot         string `json:"execution_root"`
	MergeRequired         bool   `json:"merge_required"`
}

// FailureInfo 失败信息
type FailureInfo struct {
	Code            string `json:"code"`
	Phase           string `json:"phase"`
	Message         string `json:"message"`
	Retryable       bool   `json:"retryable"`
	SuggestedAction string `json:"suggested_action"`
}

// Decision 下一步决策
type Decision struct {
	RecommendedNextAction string `json:"recommended_next_action"`
	RequiresUserApproval  bool   `json:"requires_user_approval"`
	SafeToContinue        bool   `json:"safe_to_continue"`
}

// VerifyResult 验证结果
type VerifyResult struct {
	Ran      bool     `json:"ran"`
	Passed   bool     `json:"passed"`
	Command  string   `json:"command,omitempty"`
	ExitCode int      `json:"exit_code"`
	ErrorTail []string `json:"error_tail,omitempty"`
}

// StateReport 轻量状态报告 (state.json)
type StateReport struct {
	TaskID   string `json:"task_id"`
	Status   string `json:"status"`
	Phase    string `json:"phase,omitempty"`

	WriteState    *WriteState       `json:"write_state,omitempty"`
	Files         *StateFiles       `json:"files,omitempty"`
	Verification  *StateVerification `json:"verification,omitempty"`
	Failure       *FailureInfo      `json:"failure,omitempty"`
	Rollback      *RollbackInfo     `json:"rollback,omitempty"`
	Usage         *StateUsage       `json:"usage,omitempty"`
	Artifacts     map[string]string `json:"artifacts,omitempty"`
	Decision      *Decision         `json:"decision,omitempty"`
}

// StateFiles 文件状态
type StateFiles struct {
	ModifiedFiles         []string `json:"modified_files"`
	EffectiveChangedFiles int      `json:"effective_changed_files"`
	FileHashVerified      bool     `json:"file_hash_verified"`
}

// StateVerification 验证状态
type StateVerification struct {
	Build                *VerifyResult `json:"build,omitempty"`
	Test                 *VerifyResult `json:"test,omitempty"`
	TechnicalVerification string       `json:"technical_verification"`
	BusinessAcceptance    string       `json:"business_acceptance"`
}

// RollbackInfo 回滚信息
type RollbackInfo struct {
	Available bool   `json:"available"`
	Command   string `json:"command"`
}

// StateUsage token 使用统计
type StateUsage struct {
	ExecutorTotalTokens      int              `json:"executor_total_tokens"`
	ExecutorPromptTokens     int              `json:"executor_prompt_tokens"`
	ExecutorCompletionTokens int              `json:"executor_completion_tokens"`
	ExecutorEffectiveTokens  int              `json:"executor_effective_tokens"`
	ExecutorWastedTokens     int              `json:"executor_wasted_tokens"`
	ExecutorWasteRate        float64          `json:"executor_waste_rate"`
	ControllerObservedTokens *int             `json:"controller_observed_tokens"`
	ControllerUsageSource    string           `json:"controller_usage_source"`
	ControllerUsage          *ControllerUsage `json:"controller_usage,omitempty"`
}

// ReportData 报告输入数据（不依赖 core 包）
type ReportData struct {
	TaskID                  string
	Status                  string
	Provider                string
	Model                   string
	ContextFiles            int
	ContextBytes            int
	PromptTokens            int
	CompletionTokens        int
	TotalTokens             int
	ControllerTokens        int
	EstimatedDirectTokens   int
	EstimatedGrossSavings   int
	EstimatedNetSavings     int
	TruncatedOutput         bool
	PatchEffectVerified     bool
	EffectiveChangedFiles   int
	GenerationAttempts      int
	MaxRepairAttempts       int
	PatchChangedLines       int
	ExecutorEffectiveTokens int
	ExecutorWastedTokens    int
	ExecutorWasteRate       float64
	FileHashChanges         []FileHashChange
	TechnicalVerification   string
	BusinessAcceptance      string
	ModifiedFiles           []string
	GitDiff                 string
	PatchContent            string
	CommandsRun             []CmdResult
	BuildResult             *BuildResult
	TestResult              *BuildResult
	SecurityCheck           *SecurityResult
	FailureReason           string
	RollbackInfo            string
	StartedAt               time.Time
	FinishedAt              time.Time
	// 新增字段
	Phase          string
	FailureCode    string
	FailurePhase   string
	Retryable      bool
	SuggestedAction string
	// Write state
	SnapshotCreated        bool
	ExecutionMode          string
	ExecutionRoot          string
	MergeRequired          bool
	MainWorkspaceModified  bool
}

// ReportConfig 报告生成配置
type ReportConfig struct {
	OutputDir                string
	Mode                     string // summary, full
	SaveFullReport           bool
	SaveFullPatch            bool
	SaveFullCommandOutput    bool
	CommandOutputTailLines   int
	MaxSummaryBytes          int
	MaxFailureMessageBytes   int
	IncludeModifiedFileContent bool
	IncludeDiff              bool
	IncludePatch             bool
	IncludeBackupContent     bool
	IncludeSnapshotContent   bool
}

// Generator 报告生成器
type Generator struct {
	outputDir string
	cfg       ReportConfig
}

// NewGenerator 创建报告生成器
func NewGenerator(outputDir string) *Generator {
	return &Generator{
		outputDir: outputDir,
		cfg: ReportConfig{
			Mode:                   "summary",
			CommandOutputTailLines: 120,
			MaxSummaryBytes:        8192,
			MaxFailureMessageBytes: 4096,
		},
	}
}

// NewGeneratorWithConfig 创建带配置的报告生成器
func NewGeneratorWithConfig(outputDir string, cfg ReportConfig) *Generator {
	return &Generator{outputDir: outputDir, cfg: cfg}
}

// SaveReport 保存报告到文件（兼容旧接口，默认生成轻量报告）
func (g *Generator) SaveReport(data *ReportData) (string, error) {
	taskDir := filepath.Join(g.outputDir, data.TaskID)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return "", fmt.Errorf("create report dir: %w", err)
	}

	// 1. 始终生成 state.json
	statePath := filepath.Join(taskDir, "state.json")
	if err := g.saveStateReport(statePath, data); err != nil {
		return "", fmt.Errorf("write state.json: %w", err)
	}

	// 2. 始终生成 summary.md
	summaryPath := filepath.Join(taskDir, "summary.md")
	if err := g.saveSummary(summaryPath, data); err != nil {
		return "", fmt.Errorf("write summary.md: %w", err)
	}

	// 3. 仅在 debug/full 模式下生成完整文件
	if g.cfg.Mode == "full" || g.cfg.SaveFullReport {
		fullPath := filepath.Join(taskDir, "full.md")
		fullContent, _ := g.GenerateMarkdown(data)
		os.WriteFile(fullPath, []byte(fullContent), 0644)
	}

	if g.cfg.SaveFullPatch && data.PatchContent != "" {
		patchPath := filepath.Join(taskDir, "patch.diff")
		os.WriteFile(patchPath, []byte(data.PatchContent), 0644)
	}

	// 保存完整命令输出
	if g.cfg.SaveFullCommandOutput {
		cmdDir := filepath.Join(taskDir, "commands")
		os.MkdirAll(cmdDir, 0755)
		for _, cmd := range data.CommandsRun {
			lower := strings.ToLower(strings.TrimSpace(cmd.Command))
			prefix := "cmd"
			switch {
			case strings.Contains(lower, "build"):
				prefix = "build"
			case strings.Contains(lower, "test") || strings.Contains(lower, "pytest"):
				prefix = "test"
			case strings.Contains(lower, "lint") || strings.Contains(lower, "vet"):
				prefix = "lint"
			}
			if cmd.Stdout != "" {
				os.WriteFile(filepath.Join(cmdDir, prefix+".stdout.log"), []byte(cmd.Stdout), 0644)
			}
			if cmd.Stderr != "" {
				os.WriteFile(filepath.Join(cmdDir, prefix+".stderr.log"), []byte(cmd.Stderr), 0644)
			}
		}
	}

	// 返回 summary.md 路径（保持兼容）
	return summaryPath, nil
}

// saveStateReport 保存 state.json
func (g *Generator) saveStateReport(path string, data *ReportData) error {
	isCompleted := strings.EqualFold(data.Status, "completed")

	// 构建 write_state
	ws := &WriteState{
		PatchGenerated:        true,
		PatchValidated:        data.TechnicalVerification != "NOT_STARTED",
		SnapshotCreated:       data.SnapshotCreated,
		PatchApplied:          data.PatchEffectVerified,
		PatchEffectVerified:   data.PatchEffectVerified,
		CommandsExecuted:      len(data.CommandsRun) > 0,
		RolledBack:            !isCompleted && data.RollbackInfo != "",
		MainWorkspaceModified: data.MainWorkspaceModified,
		ExecutionMode:         data.ExecutionMode,
		ExecutionRoot:         data.ExecutionRoot,
		MergeRequired:         data.MergeRequired,
	}
	if ws.ExecutionMode == "" {
		ws.ExecutionMode = "git_worktree"
	}

	// 构建 verification
	verification := &StateVerification{
		TechnicalVerification: data.TechnicalVerification,
		BusinessAcceptance:    data.BusinessAcceptance,
	}

	if data.BuildResult != nil {
		build := &VerifyResult{
			Ran:      true,
			Passed:   data.BuildResult.Success,
			ExitCode: data.BuildResult.ExitCode,
		}
		if !data.BuildResult.Success {
			build.ErrorTail = g.tailLines(data.BuildResult.Output, g.cfg.CommandOutputTailLines)
		}
		verification.Build = build
	}

	if data.TestResult != nil {
		test := &VerifyResult{
			Ran:      true,
			Passed:   data.TestResult.Success,
			ExitCode: data.TestResult.ExitCode,
		}
		if !data.TestResult.Success {
			test.ErrorTail = g.tailLines(data.TestResult.Output, g.cfg.CommandOutputTailLines)
		}
		verification.Test = test
	}

	// 构建 failure
	var failure *FailureInfo
	if !isCompleted && data.FailureReason != "" {
		code := data.FailureCode
		if code == "" {
			code = g.deriveFailureCode(data)
		}
		msg := data.FailureReason
		if len(msg) > g.cfg.MaxFailureMessageBytes {
			msg = msg[:g.cfg.MaxFailureMessageBytes] + "..."
		}
		failure = &FailureInfo{
			Code:            code,
			Phase:           data.FailurePhase,
			Message:         msg,
			Retryable:       data.Retryable,
			SuggestedAction: data.SuggestedAction,
		}
		if failure.Phase == "" {
			failure.Phase = "command_execution"
		}
		if failure.SuggestedAction == "" {
			if failure.Retryable {
				failure.SuggestedAction = "repair_patch"
			} else {
				failure.SuggestedAction = "abort"
			}
		}
	}

	// 构建 usage
	controllerSource := "unavailable"
	var controllerObserved *int
	if data.ControllerTokens > 0 {
		controllerSource = "manual"
		val := data.ControllerTokens
		controllerObserved = &val
	}
	usage := &StateUsage{
		ExecutorTotalTokens:      data.TotalTokens,
		ExecutorPromptTokens:     data.PromptTokens,
		ExecutorCompletionTokens: data.CompletionTokens,
		ExecutorEffectiveTokens:  data.ExecutorEffectiveTokens,
		ExecutorWastedTokens:     data.ExecutorWastedTokens,
		ExecutorWasteRate:        data.ExecutorWasteRate,
		ControllerObservedTokens: controllerObserved,
		ControllerUsageSource:    controllerSource,
		ControllerUsage: &ControllerUsage{
			Source:          controllerSource,
			ObservedTokens:  controllerObserved,
			Confidence:      "low",
		},
	}

	// 构建 decision
	var decision *Decision
	if isCompleted {
		decision = &Decision{
			RecommendedNextAction: "review_changes",
			RequiresUserApproval:  true,
			SafeToContinue:        true,
		}
	} else if failure != nil {
		decision = &Decision{
			RecommendedNextAction: failure.SuggestedAction,
			RequiresUserApproval:  false,
			SafeToContinue:        failure.Retryable,
		}
	}

	// 构建回滚信息
	rollback := &RollbackInfo{
		Available: data.RollbackInfo != "" || isCompleted,
		Command:   fmt.Sprintf("coding-bridge rollback %s", data.TaskID),
	}

	// artifact 路径
	artifacts := map[string]string{
		"summary_report": filepath.Join(g.outputDir, data.TaskID, "summary.md"),
	}
	if g.cfg.SaveFullReport {
		artifacts["full_report"] = filepath.Join(g.outputDir, data.TaskID, "full.md")
	}
	if g.cfg.SaveFullPatch && data.PatchContent != "" {
		artifacts["patch"] = filepath.Join(g.outputDir, data.TaskID, "patch.diff")
	}

	report := &StateReport{
		TaskID:       data.TaskID,
		Status:       data.Status,
		Phase:        data.Phase,
		WriteState:   ws,
		Files: &StateFiles{
			ModifiedFiles:         data.ModifiedFiles,
			EffectiveChangedFiles: data.EffectiveChangedFiles,
			FileHashVerified:      data.PatchEffectVerified,
		},
		Verification: verification,
		Failure:      failure,
		Rollback:     rollback,
		Usage:        usage,
		Artifacts:    artifacts,
		Decision:     decision,
	}

	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// saveSummary 保存 summary.md
func (g *Generator) saveSummary(path string, data *ReportData) error {
	var sb strings.Builder

	isCompleted := strings.EqualFold(data.Status, "completed")

	sb.WriteString(fmt.Sprintf("# coding-bridge Report: %s\n\n", data.TaskID))
	sb.WriteString(fmt.Sprintf("Status: %s\n", data.Status))
	if data.Phase != "" {
		sb.WriteString(fmt.Sprintf("Phase: %s\n", data.Phase))
	}
	sb.WriteString("\n")

	// 失败信息
	if !isCompleted && data.FailureReason != "" {
		sb.WriteString("## Failure\n\n")
		code := data.FailureCode
		if code == "" {
			code = g.deriveFailureCode(data)
		}
		sb.WriteString(fmt.Sprintf("Code: %s\n\n", code))
		msg := data.FailureReason
		if len(msg) > g.cfg.MaxFailureMessageBytes {
			msg = msg[:g.cfg.MaxFailureMessageBytes] + "..."
		}
		sb.WriteString(msg + "\n\n")

		// Error tail
		if data.BuildResult != nil && !data.BuildResult.Success {
			tail := g.tailLines(data.BuildResult.Output, g.cfg.CommandOutputTailLines)
			if len(tail) > 0 {
				sb.WriteString("## Error Tail\n\n```text\n")
				for _, line := range tail {
					sb.WriteString(line + "\n")
				}
				sb.WriteString("```\n\n")
			}
		}
	}

	// Write State
	sb.WriteString("## Write State\n\n")
	sb.WriteString(fmt.Sprintf("- Patch generated: yes\n"))
	sb.WriteString(fmt.Sprintf("- Patch validated: %s\n", boolToYesNo(data.TechnicalVerification != "NOT_STARTED")))
	sb.WriteString(fmt.Sprintf("- Snapshot created: %s\n", boolToYesNo(data.SnapshotCreated)))
	sb.WriteString(fmt.Sprintf("- Patch applied: %s\n", boolToYesNo(data.PatchEffectVerified)))
	sb.WriteString(fmt.Sprintf("- Patch effect verified: %s\n", boolToYesNo(data.PatchEffectVerified)))
	sb.WriteString(fmt.Sprintf("- Rolled back: %s\n", boolToYesNo(!isCompleted && data.RollbackInfo != "")))
	sb.WriteString(fmt.Sprintf("- Main workspace modified: %s\n", boolToYesNo(data.MainWorkspaceModified)))
	sb.WriteString(fmt.Sprintf("- Execution mode: %s\n", data.ExecutionMode))
	if data.ExecutionRoot != "" {
		sb.WriteString(fmt.Sprintf("- Execution root: %s\n", data.ExecutionRoot))
	}
	sb.WriteString(fmt.Sprintf("- Merge required: %s\n", boolToYesNo(data.MergeRequired)))
	sb.WriteString("\n")

	// Modified Files
	if len(data.ModifiedFiles) > 0 {
		sb.WriteString("## Modified Files\n\n")
		for _, f := range data.ModifiedFiles {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}

	// Verification
	sb.WriteString("## Verification\n\n")
	if data.BuildResult != nil {
		sb.WriteString(fmt.Sprintf("- Build: %s\n", passFail(data.BuildResult.Success)))
	}
	if data.TestResult != nil {
		sb.WriteString(fmt.Sprintf("- Test: %s\n", passFail(data.TestResult.Success)))
	}
	sb.WriteString("\n")

	// Token Usage
	if data.TotalTokens > 0 {
		sb.WriteString("## Token Usage\n\n")
		sb.WriteString(fmt.Sprintf("- Executor total tokens: %d\n", data.TotalTokens))
		sb.WriteString(fmt.Sprintf("- Executor wasted tokens: %d\n", data.ExecutorWastedTokens))
		sb.WriteString(fmt.Sprintf("- Executor waste rate: %.2f%%\n", data.ExecutorWasteRate*100))
		if data.ControllerTokens > 0 {
			sb.WriteString(fmt.Sprintf("- Controller tokens: %d (observed)\n", data.ControllerTokens))
		} else {
			sb.WriteString("- Controller tokens: unavailable\n")
		}
		sb.WriteString("\n")
	}

	// Next Action
	sb.WriteString("## Next Action\n\n")
	if isCompleted {
		sb.WriteString("Review changes, then approve merge or rollback.\n\n")
	} else if data.FailureCode != "" {
		code := data.FailureCode
		switch code {
		case "BUILD_FAILED", "TEST_FAILED", "COMMAND_FAILED":
			sb.WriteString("Repair patch or create a smaller task.\n\n")
		case "PATCH_PARSE_FAILED", "PATCH_VALIDATE_FAILED":
			sb.WriteString("Fix the task definition or simplify requirements.\n\n")
		case "TASK_TEXT_TOO_LARGE":
			sb.WriteString("Reduce task size. Use allowed_files instead of embedding content in task.json.\n\n")
		case "FORBIDDEN_INTERNAL_CONTEXT":
			sb.WriteString("Remove .coding-bridge internal paths from allowed_files.\n\n")
		default:
			sb.WriteString("Check failure details and decide on next step.\n\n")
		}
	} else {
		sb.WriteString("Check failure details and decide on next step.\n\n")
	}

	// Rollback command
	sb.WriteString("Rollback command:\n\n```bash\n")
	sb.WriteString(fmt.Sprintf("coding-bridge rollback %s\n", data.TaskID))
	sb.WriteString("```\n")

	content := sb.String()
	if len(content) > g.cfg.MaxSummaryBytes {
		content = content[:g.cfg.MaxSummaryBytes] + "\n\n...(summary truncated)"
	}

	return os.WriteFile(path, []byte(content), 0644)
}

// deriveFailureCode 从 FailureReason 推导失败码
func (g *Generator) deriveFailureCode(data *ReportData) string {
	r := data.FailureReason
	switch {
	case strings.Contains(r, "TASK_TEXT_TOO_LARGE"):
		return "TASK_TEXT_TOO_LARGE"
	case strings.Contains(r, "FORBIDDEN_INTERNAL_CONTEXT"):
		return "FORBIDDEN_INTERNAL_CONTEXT"
	case strings.Contains(r, "TRUNCATED_OUTPUT"):
		return "TRUNCATED_OUTPUT"
	case strings.Contains(r, "patch parse failed"):
		return "PATCH_PARSE_FAILED"
	case strings.Contains(r, "patch validation failed"):
		return "PATCH_VALIDATE_FAILED"
	case strings.Contains(r, "patch apply failed"):
		return "PATCH_APPLY_FAILED"
	case strings.Contains(r, "NO_EFFECTIVE_CHANGE"):
		return "NO_EFFECTIVE_CHANGE"
	case strings.Contains(r, "command") && strings.Contains(r, "failed"):
		return "COMMAND_FAILED"
	case strings.Contains(r, "build") && strings.Contains(r, "fail"):
		return "BUILD_FAILED"
	case strings.Contains(r, "test") && strings.Contains(r, "fail"):
		return "TEST_FAILED"
	case strings.Contains(r, "rollback") && strings.Contains(r, "fail"):
		return "ROLLBACK_FAILED"
	default:
		return "UNKNOWN_FAILED"
	}
}

// tailLines 返回字符串的最后 N 行
func (g *Generator) tailLines(s string, n int) []string {
	if n <= 0 || s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// 去除末尾空行
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	// 截断每行
	for i, line := range lines {
		if len(line) > 500 {
			lines[i] = line[:500] + "..."
		}
	}
	return lines
}

// GenerateMarkdown 生成完整 Markdown 格式报告（用于 full 模式，向后兼容）
func (g *Generator) GenerateMarkdown(data *ReportData) (string, error) {
	var sb strings.Builder

	sb.WriteString("# coding-bridge 任务报告\n\n")

	// 基本信息
	sb.WriteString("## 任务信息\n\n")
	sb.WriteString(fmt.Sprintf("| 字段 | 值 |\n"))
	sb.WriteString(fmt.Sprintf("|------|----|\n"))
	sb.WriteString(fmt.Sprintf("| Task ID | `%s` |\n", data.TaskID))
	sb.WriteString(fmt.Sprintf("| 状态 | **%s** |\n", data.Status))
	if data.Provider != "" {
		sb.WriteString(fmt.Sprintf("| Executor | `%s` |\n", data.Provider))
	}
	if data.Model != "" {
		sb.WriteString(fmt.Sprintf("| 模型 | `%s` |\n", data.Model))
	}
	sb.WriteString(fmt.Sprintf("| 上下文 | %d 文件 / %d bytes |\n", data.ContextFiles, data.ContextBytes))
	if data.TotalTokens > 0 {
		sb.WriteString(fmt.Sprintf("| Token | prompt %d / completion %d / total %d |\n",
			data.PromptTokens, data.CompletionTokens, data.TotalTokens))
	}
	if data.EstimatedDirectTokens > 0 {
		sb.WriteString(fmt.Sprintf("| Direct Codex baseline (estimated) | %d tokens |\n", data.EstimatedDirectTokens))
		if data.ControllerTokens > 0 {
			sb.WriteString(fmt.Sprintf("| Controller tokens (observed) | %d |\n", data.ControllerTokens))
			sb.WriteString(fmt.Sprintf("| Estimated net token savings | %d |\n", data.EstimatedNetSavings))
		} else {
			sb.WriteString(fmt.Sprintf("| Estimated gross token savings | %d |\n", data.EstimatedGrossSavings))
			sb.WriteString("| Savings note | Controller/Codex session tokens were not reported, so net savings are unknown. |\n")
		}
	}
	sb.WriteString(fmt.Sprintf("| Generation attempts | %d |\n", data.GenerationAttempts))
	sb.WriteString(fmt.Sprintf("| Max repair attempts | %d |\n", data.MaxRepairAttempts))
	sb.WriteString(fmt.Sprintf("| Truncated output | %t |\n", data.TruncatedOutput))
	sb.WriteString(fmt.Sprintf("| Patch effect verified | %t |\n", data.PatchEffectVerified))
	sb.WriteString(fmt.Sprintf("| Effective changed files | %d |\n", data.EffectiveChangedFiles))
	sb.WriteString(fmt.Sprintf("| Patch changed lines | %d |\n", data.PatchChangedLines))
	sb.WriteString(fmt.Sprintf("| Technical verification | `%s` |\n", data.TechnicalVerification))
	sb.WriteString(fmt.Sprintf("| Business acceptance | `%s` |\n", data.BusinessAcceptance))
	if data.TotalTokens > 0 {
		sb.WriteString(fmt.Sprintf("| Executor effective tokens | %d |\n", data.ExecutorEffectiveTokens))
		sb.WriteString(fmt.Sprintf("| Executor wasted tokens | %d |\n", data.ExecutorWastedTokens))
		sb.WriteString(fmt.Sprintf("| Executor waste rate | %.2f%% |\n", data.ExecutorWasteRate*100))
	}
	sb.WriteString(fmt.Sprintf("| 开始时间 | %s |\n", data.StartedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("| 结束时间 | %s |\n", data.FinishedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("| 耗时 | %s |\n", data.FinishedAt.Sub(data.StartedAt).Round(time.Millisecond)))
	sb.WriteString("\n")

	// 修改文件
	if len(data.ModifiedFiles) > 0 {
		sb.WriteString("## 修改文件\n\n")
		for _, f := range data.ModifiedFiles {
			sb.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
		sb.WriteString("\n")
	}

	if len(data.FileHashChanges) > 0 {
		sb.WriteString("## File hash verification\n\n")
		sb.WriteString("| File | Before SHA-256 | After SHA-256 |\n")
		sb.WriteString("|------|---------------|--------------|\n")
		for _, change := range data.FileHashChanges {
			sb.WriteString(fmt.Sprintf(
				"| `%s` | `%s` | `%s` |\n",
				change.File,
				change.BeforeSHA256,
				change.AfterSHA256,
			))
		}
		sb.WriteString("\n")
	}

	// Git Diff (仅在 include_diff 为 true 时)
	if data.GitDiff != "" && g.cfg.IncludeDiff {
		sb.WriteString("## Git Diff\n\n")
		sb.WriteString("```diff\n")
		sb.WriteString(data.GitDiff)
		sb.WriteString("\n```\n\n")
	}

	// Patch content (仅在 include_patch 为 true 时)
	if data.PatchContent != "" && g.cfg.IncludePatch {
		sb.WriteString("## Patch\n\n")
		sb.WriteString("```diff\n")
		sb.WriteString(data.PatchContent)
		sb.WriteString("\n```\n\n")
	}

	// 命令执行结果 (仅在 include 模式时显示输出)
	if len(data.CommandsRun) > 0 {
		sb.WriteString("## 命令执行\n\n")
		for _, cmd := range data.CommandsRun {
			sb.WriteString(fmt.Sprintf("### `%s`\n\n", cmd.Command))
			sb.WriteString(fmt.Sprintf("- **Exit Code**: %d\n", cmd.ExitCode))
			sb.WriteString(fmt.Sprintf("- **耗时**: %s\n", cmd.Duration))
			if cmd.TimedOut {
				sb.WriteString("- **⚠️ 超时**\n")
			}
			showFull := cmd.ExitCode != 0 && g.cfg.SaveFullCommandOutput
			if showFull {
				if cmd.Stdout != "" {
					sb.WriteString("\n**stdout:**\n\n```\n")
					sb.WriteString(truncate(cmd.Stdout, 5000))
					sb.WriteString("\n```\n")
				}
				if cmd.Stderr != "" {
					sb.WriteString("\n**stderr:**\n\n```\n")
					sb.WriteString(truncate(cmd.Stderr, 5000))
					sb.WriteString("\n```\n")
				}
			}
		}
		sb.WriteString("\n")
	}

	// 构建结果
	if data.BuildResult != nil {
		sb.WriteString("## 构建结果\n\n")
		status := "✅ 成功"
		if !data.BuildResult.Success {
			status = "❌ 失败"
		}
		sb.WriteString(fmt.Sprintf("**%s** (Exit Code: %d)\n\n", status, data.BuildResult.ExitCode))
		if data.BuildResult.Output != "" && g.cfg.SaveFullCommandOutput {
			sb.WriteString("```\n")
			sb.WriteString(truncate(data.BuildResult.Output, 5000))
			sb.WriteString("\n```\n\n")
		}
	}

	// 测试结果
	if data.TestResult != nil {
		sb.WriteString("## 测试结果\n\n")
		status := "✅ 通过"
		if !data.TestResult.Success {
			status = "❌ 失败"
		}
		sb.WriteString(fmt.Sprintf("**%s** (Exit Code: %d)\n\n", status, data.TestResult.ExitCode))
		if data.TestResult.Output != "" && g.cfg.SaveFullCommandOutput {
			sb.WriteString("```\n")
			sb.WriteString(truncate(data.TestResult.Output, 5000))
			sb.WriteString("\n```\n\n")
		}
	}

	// 安全检查结果
	if data.SecurityCheck != nil {
		sb.WriteString("## 安全检查\n\n")
		status := "✅ 通过"
		if !data.SecurityCheck.Passed {
			status = "❌ 未通过"
		}
		sb.WriteString(fmt.Sprintf("**%s**\n\n", status))
		if data.SecurityCheck.SecretFound {
			sb.WriteString("- ⚠️ 检测到敏感信息\n")
		}
		if data.SecurityCheck.ForbiddenFilesAccessed {
			sb.WriteString("- ⚠️ 访问了禁止文件\n")
		}
		for _, issue := range data.SecurityCheck.Issues {
			sb.WriteString(fmt.Sprintf("- %s\n", issue))
		}
		sb.WriteString("\n")
	}

	// 失败原因
	if data.FailureReason != "" {
		sb.WriteString("## 失败原因\n\n")
		sb.WriteString(data.FailureReason)
		sb.WriteString("\n\n")
	}

	// 回滚信息
	if data.RollbackInfo != "" {
		sb.WriteString("## 回滚信息\n\n")
		sb.WriteString(data.RollbackInfo)
		sb.WriteString("\n\n")
	}

	return sb.String(), nil
}

// truncate 截断字符串到最大长度
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}

func boolToYesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func passFail(passed bool) string {
	if passed {
		return "passed"
	}
	return "failed"
}
