package cli

import (
	"fmt"
	"os"

	"github.com/coding-bridge/internal/config"
	"github.com/coding-bridge/internal/web"
	"github.com/spf13/cobra"
)

var (
	webPort int
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "启动 Web 配置页面",
	Long: `启动本地 Web 服务器，在浏览器中编辑 coding-bridge 配置。

默认地址: http://127.0.0.1:8765`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("获取当前目录失败: %w", err)
		}

		loader := config.NewLoader(projectRoot)
		if !loader.Exists() {
			cfg := config.DefaultConfig()
			if err := loader.Save(cfg); err != nil {
				return fmt.Errorf("创建默认配置失败: %w", err)
			}
			fmt.Printf("✅ 已创建默认配置 (版本 %d)\n", cfg.Version)
			fmt.Println()
		}

		srv := web.NewServer(projectRoot, webPort)
		url, err := srv.Start()
		if err != nil {
			return fmt.Errorf("启动 Web 服务失败: %w", err)
		}

		fmt.Println("╔══════════════════════════════════════════════════╗")
		fmt.Println("║         coding-bridge 配置页面已启动             ║")
		fmt.Println("╠══════════════════════════════════════════════════╣")
		fmt.Println("║                                                  ║")
		fmt.Printf("║   🌐  %-42s ║\n", url)
		fmt.Println("║                                                  ║")
		fmt.Println("║   请在浏览器中打开上述地址进行配置                ║")
		fmt.Println("║   按 Ctrl+C 停止服务                              ║")
		fmt.Println("║                                                  ║")
		fmt.Println("╚══════════════════════════════════════════════════╝")
		fmt.Println()

		web.OpenBrowser(url)
		fmt.Println("📂 正在尝试自动打开浏览器...")
		fmt.Println()

		select {}
	},
}

func init() {
	webCmd.Flags().IntVar(&webPort, "port", 8765, "Web 服务器端口")
}
