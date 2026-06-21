package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/coding-bridge/internal/config"
	"github.com/spf13/cobra"
)

const (
	codexBlockStart = "<!-- coding-bridge:codex:start -->"
	codexBlockEnd   = "<!-- coding-bridge:codex:end -->"
)

var codexCmd = &cobra.Command{
	Use:   "codex [install]",
	Short: "配置 Codex 对话接入",
	Long: `把 coding-bridge 工作流写入当前项目的 AGENTS.md。

安装后，在 Codex 对话中明确说“使用 coding-bridge 完成这个修改”，
Codex 会通过本地终端 CLI 生成 task.json、调用 coding-bridge，并读取执行报告。

示例:
  coding-bridge codex install`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		action := "install"
		if len(args) > 0 {
			action = args[0]
		}
		if action != "install" {
			return fmt.Errorf("未知操作: %s (可用: install)", action)
		}

		projectRoot, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("获取当前目录失败: %w", err)
		}
		if _, err := ensureCodingBridgeGitignore(projectRoot); err != nil {
			return err
		}
		cfg, err := config.NewLoader(projectRoot).Load()
		if err != nil {
			return err
		}
		agentsPath, created, err := installCodexInstructions(projectRoot, cfg)
		if err != nil {
			return err
		}

		actionText := "已更新"
		if created {
			actionText = "已创建"
		}
		fmt.Printf("✅ %s Codex 项目指令: %s\n", actionText, agentsPath)
		fmt.Println("✅ 已创建任务目录: .coding-bridge/tasks")
		fmt.Println()
		fmt.Println("现在可以在 Codex 对话中说：")
		fmt.Println("  通过本地 CLI 使用 coding-bridge 修复这个问题，并运行相关测试。")
		return nil
	},
}

func installCodexInstructions(projectRoot string, cfg *config.AppConfig) (string, bool, error) {
	agentsPath := filepath.Join(projectRoot, "AGENTS.md")
	content, err := os.ReadFile(agentsPath)
	created := os.IsNotExist(err)
	if err != nil && !created {
		return "", false, fmt.Errorf("读取 AGENTS.md 失败: %w", err)
	}

	updated, err := upsertCodexBlock(string(content), cfg)
	if err != nil {
		return "", false, err
	}
	if err := os.WriteFile(agentsPath, []byte(updated), 0644); err != nil {
		return "", false, fmt.Errorf("写入 AGENTS.md 失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".coding-bridge", "tasks"), 0755); err != nil {
		return "", false, fmt.Errorf("创建任务目录失败: %w", err)
	}
	if _, err := config.WriteCodexPolicy(projectRoot, cfg); err != nil {
		return "", false, err
	}
	return agentsPath, created, nil
}

func upsertCodexBlock(existing string, cfg *config.AppConfig) (string, error) {
	block := codexInstructionsBlock(cfg)
	start := strings.Index(existing, codexBlockStart)
	end := strings.Index(existing, codexBlockEnd)

	switch {
	case start == -1 && end == -1:
		trimmed := strings.TrimRight(existing, "\r\n")
		if trimmed == "" {
			return block + "\n", nil
		}
		return trimmed + "\n\n" + block + "\n", nil
	case start == -1 || end == -1 || end < start:
		return "", fmt.Errorf("AGENTS.md 中的 coding-bridge 管理标记不完整，请先修复 %s / %s", codexBlockStart, codexBlockEnd)
	default:
		end += len(codexBlockEnd)
		return existing[:start] + block + existing[end:], nil
	}
}

func codexInstructionsBlock(cfg *config.AppConfig) string {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	_ = cfg
	lines := []string{
		codexBlockStart,
		"## coding-bridge Workflow",
		"",
		"At the start of each coding task, read the small non-secret `.coding-bridge/codex-policy.json` file and apply it. Never read `.coding-bridge/config.yaml` for workflow routing because it may contain provider credentials.",
		"",
		"- If `cli_enabled` is false, follow the user's request with Codex's normal local implementation workflow.",
		"- If `cli_enabled` and `default_cli` are true, use coding-bridge for ordinary coding changes without requiring the user to explicitly name it.",
		"- If `default_cli` is false, use coding-bridge only when the user explicitly requests it.",
		"",
		"1. `coding-bridge` is a local command-line executable, not a Codex plugin, Skill, connector, MCP tool, or installable tool. Never use tool discovery or plugin installation for it. Invoke it only through the shell/terminal.",
		"2. Act as the Controller. Inspect only enough repository context to define one focused task. Do not implement the requested code change yourself before the bridge runs.",
		"3. Verify the executable through the terminal with `coding-bridge --help`. On Windows, if it is not on `PATH` and `.\\coding-bridge.exe` exists, use that executable consistently. If neither command exists, report that the local CLI is unavailable; do not search for a plugin.",
		"4. Create `.coding-bridge/tasks/<task-id>.json`. Use a short safe task ID containing only letters, numbers, dots, underscores, and hyphens.",
		"5. Set `executor.preferred_provider` and `executor.preferred_model` from `default_provider` and `default_model` in the compact policy unless the user explicitly chooses another configured option.",
		"6. If the Codex client exposes token usage for the controller work in this task, put it in `controller.observed_tokens`; otherwise omit it. Never invent a controller token count.",
		"7. Keep `allowed_files` minimal. Never include secret or production credential files. Put only necessary build, test, or lint commands in `allowed_commands`.",
		"8. Run `coding-bridge run .coding-bridge/tasks/<task-id>.json --dry-run` in the terminal, then run the same command without `--dry-run`.",
		"9. If `sharing_approved` is true, project initialization records approval to send task allowlisted source files to configured external Executors; do not ask for the same approval again. If false, obtain explicit approval before the first external send.",
		"10. If dry-run shows all configured Executor providers are unavailable and `fallback_to_codex` is true, continue with Codex's normal local implementation workflow. Otherwise stop and report the provider failure.",
		"11. After running coding-bridge, first read: `coding-bridge status <task-id> --json` to get the state. Then read: `coding-bridge report <task-id>` for the summary.",
		"12. Do NOT read full patch, full diff, or full command logs unless failure diagnosis requires it. The summary and state.json contain everything needed for normal decision-making.",
		"13. Do NOT read .coding-bridge/backups/**, .coding-bridge/snapshots/**, or .coding-bridge/reports/** full artifacts by default.",
		"14. Do NOT include .coding-bridge/** in allowed_files. This includes .coding-bridge/config.yaml, .coding-bridge/backups/**, .coding-bridge/reports/**, .coding-bridge/snapshots/**, and .coding-bridge/tasks/**.",
		"15. Keep task.json short. Do NOT embed full source files, full reports, or long documents in task.json. Use allowed_files for source context.",
		"16. Each task should modify at most 3 files unless explicitly configured.",
		"17. If the result is in a Git worktree, inspect the worktree and report its path. Do not claim the main working tree was changed and do not merge automatically unless the user explicitly approves merging.",
		"18. Summarize modified files, verification results, Executor token usage, report path, and rollback command. Use `coding-bridge rollback <task-id>` when the user asks to discard the bridge result.",
		"19. Always split large requests into small tasks. Each task must have one clear goal. Do not create a task that mixes UI, business flow, MES, protocol, device I/O, and tests.",
		"20. Do not use broad allowed_files patterns like **/*.go, src/**, internal/**. Use explicit file paths.",
		"21. If the request is broad, first produce a task split plan instead of creating a single task.",
		"22. coding-bridge will reject oversized tasks (TASK_TOO_BROAD). Accept the rejection and split into smaller tasks.",
		codexBlockEnd,
	}
	return strings.Join(lines, "\n")
}
