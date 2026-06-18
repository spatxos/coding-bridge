package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/coding-bridge/internal/config"
	"github.com/coding-bridge/internal/core"
	"github.com/coding-bridge/internal/providers"
	"github.com/spf13/cobra"
)

var (
	runDryRun         bool
	runAllowHighRisk  bool
	runAllowForbidden bool
	runProvider       string
	runModel          string
)

var runCmd = &cobra.Command{
	Use:   "run [task.json]",
	Short: "执行一个任务",
	Long: `加载 task.json 并执行完整的任务流程：

1. 验证任务
2. 选择 Provider
3. 请求 Executor 生成 patch
4. 校验 patch
5. 创建快照
6. 应用 patch
7. 执行构建/测试
8. 生成报告

示例:
  coding-bridge run task.json
  coding-bridge run task.json --provider deepseek --model deepseek-v4-pro
  coding-bridge run task.json --dry-run
  coding-bridge run task.json --allow-high-risk`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskPath := args[0]

		// 加载配置
		projectRoot, _ := os.Getwd()
		loader := config.NewLoader(projectRoot)
		cfg, err := loader.Load()
		if err != nil {
			return fmt.Errorf("加载配置失败: %w", err)
		}

		// 加载任务
		task, err := core.LoadTask(taskPath)
		if err != nil {
			return fmt.Errorf("加载任务失败: %w", err)
		}

		// 命令行覆盖
		if runProvider != "" {
			task.Executor.PreferredProvider = runProvider
			task.Executor.Selection = "manual"
		}
		if runModel != "" {
			task.Executor.PreferredModel = runModel
		}
		if runAllowHighRisk {
			task.Risk.AllowHighRisk = true
		}
		if runAllowForbidden {
			task.Risk.AllowForbiddenRead = true
		}

		// 创建 Provider 注册中心
		registry := providers.NewRegistry()

		// 注册 DeepSeek
		if dsCfg, ok := cfg.Providers.Configs["deepseek"]; ok && dsCfg.Enabled {
			apiKey := os.Getenv("DEEPSEEK_API_KEY")
			if apiKey == "" && dsCfg.APIKey != "" {
				apiKey = config.ResolveEnvVars(dsCfg.APIKey)
			}
			model := dsCfg.Model
			if models := dsCfg.GetModels(); len(models) > 0 {
				model = models[0]
			}
			dsProvider := providers.NewDeepSeekProviderWithConfig(providers.ProviderConfig{
				Type:     providers.ProviderDeepSeek,
				Name:     "DeepSeek",
				BaseURL:  dsCfg.BaseURL,
				APIKey:   apiKey,
				Model:    model,
				MaxRetry: dsCfg.MaxRetry,
			})
			registry.Register(dsProvider)
			registry.RegisterAlias("deepseek", providers.ProviderDeepSeek)
		}

		// 创建 Runner
		runner := core.NewRunner(projectRoot, cfg, registry)

		ctx := context.Background()

		if runDryRun {
			fmt.Println("🔍 干运行模式...")
			result, err := runner.DryRun(ctx, task)
			if err != nil {
				return fmt.Errorf("干运行失败: %w", err)
			}
			fmt.Printf("✅ 干运行通过: 任务 '%s' 可以进行\n", task.TaskID)
			_ = result
			return nil
		}

		// 执行
		fmt.Printf("🚀 开始执行任务: %s\n", task.TaskID)
		fmt.Printf("   Provider: %s\n", task.Executor.PreferredProvider)
		fmt.Printf("   Model: %s\n", task.Executor.PreferredModel)

		runResult := runner.Run(ctx, task)

		if runResult.Err != nil {
			fmt.Printf("❌ 任务失败: %v\n", runResult.Err)
		}

		fmt.Println()
		fmt.Printf("📊 任务状态: %s\n", runResult.TaskResult.Status)
		if runResult.ReportPath != "" {
			fmt.Printf("📄 报告: %s\n", runResult.ReportPath)
		}
		if runResult.Snapshot != nil && runResult.Snapshot.WorktreePath != "" {
			fmt.Printf("🌿 审查工作区: %s\n", runResult.Snapshot.WorktreePath)
		}

		if runResult.TaskResult.Status == core.StateCompleted {
			fmt.Println("✅ 任务执行成功！")
			return nil
		}

		return fmt.Errorf("任务执行未完成: %s", runResult.TaskResult.Status)
	},
}

func init() {
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "干运行：检查但不实际执行")
	runCmd.Flags().BoolVar(&runAllowHighRisk, "allow-high-risk", false, "允许高危操作")
	runCmd.Flags().BoolVar(&runAllowForbidden, "allow-read-forbidden", false, "允许读取禁止文件")
	runCmd.Flags().StringVar(&runProvider, "provider", "", "指定 Executor Provider")
	runCmd.Flags().StringVar(&runModel, "model", "", "指定 Executor 模型")
}
