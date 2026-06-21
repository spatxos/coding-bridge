package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/coding-bridge/internal/config"
	"github.com/coding-bridge/internal/providers"
	"github.com/spf13/cobra"
)

var (
	providerCheckTarget string
)

var providersCmd = &cobra.Command{
	Use:   "providers [list|check|benchmark]",
	Short: "管理 AI Provider",
	Long: `列出、检测或基准测试已配置的 AI Provider。

示例:
  coding-bridge providers list
  coding-bridge providers check
  coding-bridge providers check --provider deepseek
  coding-bridge providers benchmark`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		action := "list"
		if len(args) > 0 {
			action = args[0]
		}

		// 加载配置
		projectRoot, _ := os.Getwd()
		loader := config.NewLoader(projectRoot)
		cfg, err := loader.Load()
		if err != nil {
			return fmt.Errorf("加载配置失败: %w", err)
		}

		registry := providers.NewRegistry()
		registerProvidersFromConfig(registry, cfg)

		switch action {
		case "list":
			return listProviders(registry, cfg)
		case "check":
			return checkProviders(registry, cfg)
		case "benchmark":
			return benchmarkProviders(registry, cfg)
		default:
			return fmt.Errorf("未知操作: %s (可用: list, check, benchmark)", action)
		}
	},
}

func init() {
	providersCmd.Flags().StringVar(&providerCheckTarget, "provider", "", "指定要检测的 Provider")
}

func listProviders(registry *providers.Registry, cfg *config.AppConfig) error {
	fmt.Println("📋 已注册的 Provider:")
	fmt.Println()

	all := registry.ListAll()
	if len(all) == 0 {
		fmt.Println("  ⚠️  没有已配置的 Provider。")
		fmt.Println()
		fmt.Println("  运行交互式初始化向导来配置：")
		fmt.Println("    coding-bridge init")
		fmt.Println()
		fmt.Println("  或手动编辑配置文件：")
		fmt.Println("    .coding-bridge/config.yaml")
		return nil
	}

	for _, p := range all {
		fmt.Printf("  🔹 %s (%s)\n", p.Name(), p.Type())
	}

	fmt.Println()
	fmt.Println("使用 'coding-bridge providers check' 检测健康状态")
	return nil
}

func checkProviders(registry *providers.Registry, cfg *config.AppConfig) error {
	checker := providers.NewHealthChecker(registry)
	ctx := context.Background()

	if providerCheckTarget != "" {
		// 检测单个 Provider
		pt := providers.ProviderType(providerCheckTarget)
		fmt.Printf("🔍 检测 Provider: %s\n\n", pt)

		result, err := checker.CheckOne(ctx, pt)
		if err != nil {
			return fmt.Errorf("检测失败: %w", err)
		}

		printHealthResult(result)
		return nil
	}

	// 检测所有 Provider
	fmt.Println("🔍 检测所有 Provider...")

	results := checker.CheckAll(ctx)
	for pt, result := range results {
		fmt.Printf("  🔹 %s:\n", pt)
		printHealthResult(result)
	}

	return nil
}

func printHealthResult(result *providers.HealthCheckResult) {
	statusIcon := "✅"
	switch result.Status {
	case providers.HealthHealthy:
		statusIcon = "✅"
	case providers.HealthDegraded:
		statusIcon = "⚠️"
	case providers.HealthUnhealthy:
		statusIcon = "❌"
	default:
		statusIcon = "❓"
	}

	fmt.Printf("    状态: %s %s\n", statusIcon, result.Status)
	fmt.Printf("    模型: %s\n", result.Model)
	fmt.Printf("    连通性: %v\n", result.Connectivity)
	fmt.Printf("    认证: %v\n", result.Auth)
	fmt.Printf("    模型存在: %v\n", result.ModelExists)
	fmt.Printf("    延迟: %v\n", result.Latency.Round(0))
	if result.Error != "" {
		fmt.Printf("    错误: %s\n", result.Error)
	}
	fmt.Println()
}

func benchmarkProviders(registry *providers.Registry, cfg *config.AppConfig) error {
	fmt.Println("🏃 Provider 基准测试")
	fmt.Println("  (基准测试功能开发中)")
	return nil
}

// registerProvidersFromConfig 从配置注册 Provider（供 providers check / init 使用）
func registerProvidersFromConfig(registry *providers.Registry, cfg *config.AppConfig) {
	for name, pc := range cfg.Providers.Configs {
		if !pc.Enabled {
			continue
		}
		apiKey := resolveProviderAPIKey(name, pc.APIKey)
		// 取第一个模型
		model := ""
		if models := pc.GetModels(); len(models) > 0 {
			model = models[0]
		}

		switch pc.Type {
		case "deepseek":
			registry.Register(providers.NewDeepSeekProviderWithConfig(providers.ProviderConfig{
				Type:     providers.ProviderDeepSeek,
				Name:     "DeepSeek",
				BaseURL:  pc.BaseURL,
				APIKey:   apiKey,
				Model:    model,
				MaxRetry: pc.MaxRetry,
			}))
			registry.RegisterAlias("deepseek", providers.ProviderDeepSeek)
		}
	}
}

func resolveProviderAPIKey(name, configured string) string {
	if key := os.Getenv(strings.ToUpper(name) + "_API_KEY"); key != "" {
		return key
	}
	configured = strings.TrimSpace(configured)
	if strings.HasPrefix(configured, "${") && strings.HasSuffix(configured, "}") {
		envName := strings.TrimSuffix(strings.TrimPrefix(configured, "${"), "}")
		return os.Getenv(envName)
	}
	return configured
}
