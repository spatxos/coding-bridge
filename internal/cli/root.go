package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "coding-bridge",
	Short: "AI Coding Agent 安全执行桥接系统",
	Long: `coding-bridge 是一个面向 AI Coding Agent 的安全执行桥接系统。

它负责任务事务管理、模型调度、Patch 校验、Git/Bak 保护、
命令执行、测试验证、报告回传、失败恢复等能力。

核心目标：
  高质量模型（Codex/GPT-5.5）负责分析、规划、审查；
  低成本模型（DeepSeek/Qwen）负责执行、生成 patch；
  coding-bridge 负责安全执行、状态管理、失败恢复。`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute 执行 CLI
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(rollbackCmd)
	rootCmd.AddCommand(providersCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(webCmd)
	rootCmd.AddCommand(codexCmd)
}
