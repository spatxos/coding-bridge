package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/coding-bridge/internal/config"
	"github.com/coding-bridge/internal/web"
	"github.com/spf13/cobra"
)

var (
	initQuick bool
	initPort  int
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化 coding-bridge 项目配置（Web 引导）",
	Long: `启动本地 Web 配置页面，在浏览器中配置 coding-bridge。

引导你配置 AI Provider（Executor 模型），包括 API Key、模型名等。

使用 --quick 跳过 Web 页面，直接生成默认配置文件。`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVar(&initQuick, "quick", false, "快速模式：跳过 Web，直接生成默认配置")
	initCmd.Flags().IntVar(&initPort, "port", 8765, "Web 配置页面端口")
}

func runInit(cmd *cobra.Command, args []string) error {
	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取当前目录失败: %w", err)
	}

	added, err := ensureCodingBridgeGitignore(projectRoot)
	if err != nil {
		return err
	}
	if added {
		fmt.Println("✅ 已将 /.coding-bridge/ 添加到 .gitignore")
		fmt.Println()
	}

	loader := config.NewLoader(projectRoot)

	// 快速模式
	if initQuick {
		return quickInit(loader)
	}

	// 已有配置时提示
	if loader.Exists() {
		fmt.Println("⚙️  配置文件已存在: .coding-bridge/config.yaml")
		fmt.Println()
		fmt.Println("你可以：")
		fmt.Println("  coding-bridge web                  # 打开 Web 配置页面编辑")
		fmt.Println("  coding-bridge config validate      # 校验现有配置")
		fmt.Println("  coding-bridge init --quick         # 重置为默认配置")
		fmt.Println()
		fmt.Print("是否打开 Web 页面编辑现有配置？[Y/n] ")

		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(answer) == "n" || strings.ToLower(answer) == "no" {
			return nil
		}
	}

	return webInit(loader, projectRoot)
}

func quickInit(loader *config.Loader) error {
	cfg, err := loader.LoadOrInit()
	if err != nil {
		return fmt.Errorf("初始化失败: %w", err)
	}
	if _, _, err := installCodexInstructions(loader.ProjectRoot(), cfg); err != nil {
		return fmt.Errorf("install Codex workflow: %w", err)
	}
	fmt.Printf("✅ 已创建默认配置 (版本 %d)\n", cfg.Version)
	fmt.Println()
	fmt.Println("下一步:")
	fmt.Println("  coding-bridge providers check    # 检测 Provider 状态")
	fmt.Println("  coding-bridge web                # 打开 Web 页面配置")
	fmt.Println("  coding-bridge run task.json      # 执行任务")
	return nil
}

func webInit(loader *config.Loader, projectRoot string) error {
	// 确保有默认配置
	if !loader.Exists() {
		cfg := config.DefaultConfig()
		if err := loader.Save(cfg); err != nil {
			return fmt.Errorf("创建默认配置失败: %w", err)
		}
		fmt.Printf("✅ 已创建默认配置 (版本 %d)\n", cfg.Version)
		fmt.Println()
	}
	cfg, err := loader.Load()
	if err != nil {
		return err
	}
	if _, _, err := installCodexInstructions(projectRoot, cfg); err != nil {
		return fmt.Errorf("install Codex workflow: %w", err)
	}

	// 启动 Web 服务器
	srv := web.NewServer(projectRoot, initPort)
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

	// 尝试自动打开浏览器
	web.OpenBrowser(url)
	fmt.Println("📂 正在尝试自动打开浏览器...")
	fmt.Println("   如未自动打开，请手动复制上方 URL 到浏览器")
	fmt.Println()
	fmt.Println("配置完成后，运行以下命令验证：")
	fmt.Println("   coding-bridge providers check")
	fmt.Println()

	// 阻塞等待
	select {}
}

func ensureCodingBridgeGitignore(projectRoot string) (bool, error) {
	gitignorePath := filepath.Join(projectRoot, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("读取 .gitignore 失败: %w", err)
	}

	content := string(data)
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		switch strings.TrimSpace(line) {
		case ".coding-bridge", ".coding-bridge/", "/.coding-bridge", "/.coding-bridge/":
			return false, nil
		}
	}

	lineEnding := "\n"
	if strings.Contains(content, "\r\n") {
		lineEnding = "\r\n"
	}
	trimmed := strings.TrimRight(content, "\r\n")
	if trimmed != "" {
		trimmed += lineEnding
	}
	updated := trimmed + "/.coding-bridge/" + lineEnding
	if err := os.WriteFile(gitignorePath, []byte(updated), 0644); err != nil {
		return false, fmt.Errorf("写入 .gitignore 失败: %w", err)
	}
	return true, nil
}
