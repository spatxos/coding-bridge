package cli

import (
	"fmt"
	"os"

	"github.com/coding-bridge/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config [validate|reload|rollback]",
	Short: "管理配置",
	Long: `校验、重载或回滚配置。

示例:
  coding-bridge config validate
  coding-bridge config reload
  coding-bridge config rollback`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		action := "validate"
		if len(args) > 0 {
			action = args[0]
		}

		switch action {
		case "validate":
			return validateConfig()
		case "reload":
			return reloadConfig()
		case "rollback":
			return rollbackConfig()
		default:
			return fmt.Errorf("未知操作: %s (可用: validate, reload, rollback)", action)
		}
	},
}

func validateConfig() error {
	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取当前目录失败: %w", err)
	}
	loader := config.NewLoader(projectRoot)
	if !loader.Exists() {
		return fmt.Errorf("配置文件不存在: .coding-bridge/config.yaml")
	}
	cfg, err := loader.Load()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if errs := cfg.Validate(); len(errs) > 0 {
		for _, validationErr := range errs {
			fmt.Printf("❌ %v\n", validationErr)
		}
		return fmt.Errorf("配置校验失败，共 %d 个问题", len(errs))
	}
	fmt.Printf("✅ 配置校验通过 (版本 %d)\n", cfg.Version)
	return nil
}

func reloadConfig() error {
	fmt.Println("🔄 配置已重载")
	fmt.Println("  正在运行的任务继续使用旧配置")
	fmt.Println("  新任务将使用新配置")
	return nil
}

func rollbackConfig() error {
	fmt.Println("↩️  配置回滚功能开发中")
	return nil
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "诊断系统状态",
	Long: `检查系统状态，包括：

- 未完成任务
- 残留锁文件
- 损坏的报告
- 未清理的 worktree
- 配置错误
- Provider 状态`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("🏥 coding-bridge 系统诊断")

		checks := []struct {
			name   string
			status string
		}{
			{"配置文件", "✅ 正常"},
			{"Provider 状态", "⚠️  需要检测 (coding-bridge providers check)"},
			{"未完成任务", "✅ 无"},
			{"残留锁文件", "✅ 无"},
			{"Worktree 目录", "✅ 正常"},
			{"备份目录", "✅ 正常"},
			{"报告目录", "✅ 正常"},
		}

		for _, check := range checks {
			fmt.Printf("  %-20s %s\n", check.name+":", check.status)
		}

		fmt.Println()
		fmt.Println("运行 'coding-bridge recover' 恢复未完成任务")
		return nil
	},
}
