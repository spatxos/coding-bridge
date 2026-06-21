// Package report 提供任务执行报告的生成功能。
// 每个任务完成后必须生成报告，Controller Model 依赖报告进行下一步决策。
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CmdResult 命令执行结果（不依赖 core 包）
type CmdResult struct {
	Command  string
	Stdout   string
	Stderr   string
	ExitCode int
	Duration string
	TimedOut bool
}

// BuildResult 构建/测试结果（不依赖 core 包）
type BuildResult struct {
	Success  bool
	Output   string
	ExitCode int
}

// SecurityResult 安全检查结果（不依赖 core 包）
type SecurityResult struct {
	Passed                 bool
	Issues                 []string
	SecretFound            bool
	ForbiddenFilesAccessed bool
}

type FileHashChange struct {
	File         string
	BeforeSHA256 string
	AfterSHA256  string
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
	CommandsRun             []CmdResult
	BuildResult             *BuildResult
	TestResult              *BuildResult
	SecurityCheck           *SecurityResult
	FailureReason           string
	RollbackInfo            string
	StartedAt               time.Time
	FinishedAt              time.Time
}

// Generator 报告生成器
type Generator struct {
	outputDir string
}

// NewGenerator 创建报告生成器
func NewGenerator(outputDir string) *Generator {
	return &Generator{outputDir: outputDir}
}

// GenerateMarkdown 生成 Markdown 格式报告
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

	// Git Diff
	if data.GitDiff != "" {
		sb.WriteString("## Git Diff\n\n")
		sb.WriteString("```diff\n")
		sb.WriteString(data.GitDiff)
		sb.WriteString("\n```\n\n")
	}

	// 命令执行结果
	if len(data.CommandsRun) > 0 {
		sb.WriteString("## 命令执行\n\n")
		for _, cmd := range data.CommandsRun {
			sb.WriteString(fmt.Sprintf("### `%s`\n\n", cmd.Command))
			sb.WriteString(fmt.Sprintf("- **Exit Code**: %d\n", cmd.ExitCode))
			sb.WriteString(fmt.Sprintf("- **耗时**: %s\n", cmd.Duration))
			if cmd.TimedOut {
				sb.WriteString("- **⚠️ 超时**\n")
			}
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
		if data.BuildResult.Output != "" {
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
		if data.TestResult.Output != "" {
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

// SaveReport 保存报告到文件
func (g *Generator) SaveReport(data *ReportData) (string, error) {
	content, err := g.GenerateMarkdown(data)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(g.outputDir, 0755); err != nil {
		return "", fmt.Errorf("create report dir: %w", err)
	}

	filename := fmt.Sprintf("%s-%s-report.md", data.TaskID, time.Now().Format("20060102-150405"))
	reportPath := filepath.Join(g.outputDir, filename)

	if err := os.WriteFile(reportPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}

	return reportPath, nil
}

// truncate 截断字符串到最大长度
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}
