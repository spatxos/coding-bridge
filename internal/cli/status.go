package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/coding-bridge/internal/config"
	"github.com/coding-bridge/internal/core"
	"github.com/coding-bridge/internal/providers"
	"github.com/spf13/cobra"
)

var reportProject string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看任务状态",
	Long:  "查看当前项目的任务执行状态。",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := resolveProjectRoot(reportProject)
		if err != nil {
			return fmt.Errorf("获取当前目录失败: %w", err)
		}
		snapshots, err := listFiles(filepath.Join(projectRoot, ".coding-bridge", "snapshots"), ".json")
		if err != nil {
			return err
		}
		reports, err := listFiles(filepath.Join(projectRoot, ".coding-bridge", "reports"), "-report.md")
		if err != nil {
			return err
		}

		fmt.Println("📊 任务状态:")
		if len(snapshots) == 0 {
			fmt.Println("  待审查/可回滚任务: 0")
		} else {
			fmt.Printf("  待审查/可回滚任务: %d\n", len(snapshots))
			for _, snapshot := range snapshots {
				fmt.Printf("    - %s\n", strings.TrimSuffix(filepath.Base(snapshot), ".json"))
			}
		}
		fmt.Printf("  历史报告: %d\n", len(reports))
		fmt.Println()
		fmt.Println("使用 'coding-bridge report latest' 查看最近报告")
		return nil
	},
}

var reportCmd = &cobra.Command{
	Use:   "report [latest|task-id]",
	Short: "查看任务报告",
	Long: `查看任务执行报告。

示例:
  coding-bridge report latest    # 查看最新报告
  coding-bridge report task-123  # 查看指定任务报告`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := "latest"
		if len(args) > 0 {
			target = args[0]
		}

		projectRoot, err := resolveProjectRoot(reportProject)
		if err != nil {
			return fmt.Errorf("确定项目根目录失败: %w", err)
		}
		reports, err := listFiles(filepath.Join(projectRoot, ".coding-bridge", "reports"), "-report.md")
		if err != nil {
			return err
		}
		if target == "stats" {
			return printReportStats(reports)
		}
		reportPath, err := selectReport(reports, target)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(reportPath)
		if err != nil {
			return fmt.Errorf("读取报告失败: %w", err)
		}

		fmt.Printf("📄 报告: %s\n\n", reportPath)
		fmt.Print(string(content))
		if len(content) == 0 || content[len(content)-1] != '\n' {
			fmt.Println()
		}
		return nil
	},
}

var tokenRowPattern = regexp.MustCompile(`\| Token \| prompt (\d+) / completion (\d+) / total (\d+) \|`)
var statusValuePattern = regexp.MustCompile(`\*\*(completed|failed|rolled_back|cancelled)\*\*`)

func printReportStats(reports []string) error {
	var totalTokens int64
	var effectiveTokens int64
	var wastedTokens int64
	var completed int
	var failed int
	var measured int
	for _, reportPath := range reports {
		content, err := os.ReadFile(reportPath)
		if err != nil {
			return fmt.Errorf("read report %s: %w", reportPath, err)
		}
		tokenMatch := tokenRowPattern.FindStringSubmatch(string(content))
		if tokenMatch == nil {
			continue
		}
		tokens, _ := strconv.ParseInt(tokenMatch[3], 10, 64)
		statusMatch := statusValuePattern.FindStringSubmatch(string(content))
		status := ""
		if statusMatch != nil {
			status = statusMatch[1]
		}
		totalTokens += tokens
		measured++
		if status == "completed" {
			completed++
			effectiveTokens += tokens
		} else {
			failed++
			wastedTokens += tokens
		}
	}
	wasteRate := 0.0
	if totalTokens > 0 {
		wasteRate = float64(wastedTokens) / float64(totalTokens) * 100
	}
	fmt.Println("coding-bridge Executor usage statistics")
	fmt.Printf("  Reports with provider usage: %d\n", measured)
	fmt.Printf("  Completed attempts: %d\n", completed)
	fmt.Printf("  Failed/non-completed attempts: %d\n", failed)
	fmt.Printf("  Total Executor tokens: %d\n", totalTokens)
	fmt.Printf("  Effective-attempt tokens: %d\n", effectiveTokens)
	fmt.Printf("  Wasted-attempt tokens: %d\n", wastedTokens)
	fmt.Printf("  Attempt waste rate: %.2f%%\n", wasteRate)
	fmt.Println("  Note: effective-attempt means the bridge completed its technical gates; business acceptance still requires Controller review.")
	return nil
}

var rollbackCmd = &cobra.Command{
	Use:   "rollback [task-id]",
	Short: "回滚指定任务",
	Long: `回滚指定任务的修改。

示例:
  coding-bridge rollback task-123`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]
		fmt.Printf("↩️  回滚任务: %s\n", taskID)
		projectRoot, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("获取当前目录失败: %w", err)
		}
		cfg, err := config.NewLoader(projectRoot).Load()
		if err != nil {
			return fmt.Errorf("加载配置失败: %w", err)
		}
		runner := core.NewRunner(projectRoot, cfg, providers.NewRegistry())
		if err := runner.RollbackTask(taskID); err != nil {
			return fmt.Errorf("回滚失败: %w", err)
		}
		fmt.Println("✅ 回滚完成")
		return nil
	},
}

func listFiles(dir, suffix string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取目录 %s 失败: %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Slice(files, func(i, j int) bool {
		left, leftErr := os.Stat(files[i])
		right, rightErr := os.Stat(files[j])
		if leftErr != nil || rightErr != nil {
			return files[i] > files[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	return files, nil
}

func selectReport(reports []string, target string) (string, error) {
	if target == "" || target == "latest" {
		if len(reports) == 0 {
			return "", fmt.Errorf("没有可用报告")
		}
		return reports[0], nil
	}
	if filepath.Base(target) != target {
		return "", fmt.Errorf("无效的 task-id: %s", target)
	}
	if len(reports) == 0 {
		return "", fmt.Errorf("没有可用报告")
	}
	prefix := target + "-"
	for _, reportPath := range reports {
		if strings.HasPrefix(filepath.Base(reportPath), prefix) {
			return reportPath, nil
		}
	}
	return "", fmt.Errorf("未找到任务 %s 的报告", target)
}

func init() {
	reportCmd.Flags().StringVar(&reportProject, "project", "", "项目根目录；默认从当前目录向上查找 .coding-bridge 或 .git")
}
