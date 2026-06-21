package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Loader 负责加载和保存配置文件
type Loader struct {
	projectRoot string
	configDir   string
	configPath  string
}

// NewLoader 创建配置加载器
// projectRoot: 项目根目录
func NewLoader(projectRoot string) *Loader {
	configDir := filepath.Join(projectRoot, ".coding-bridge")
	return &Loader{
		projectRoot: projectRoot,
		configDir:   configDir,
		configPath:  filepath.Join(configDir, "config.yaml"),
	}
}

// Load 加载配置文件。如果文件不存在，返回默认配置。
func (l *Loader) Load() (*AppConfig, error) {
	data, err := os.ReadFile(l.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 返回默认配置
			cfg := DefaultConfig()
			return cfg, nil
		}
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	// 合并默认值（未设置的字段使用默认值）
	cfg = mergeDefaults(cfg)

	return &cfg, nil
}

// Save 保存配置到文件
func (l *Loader) Save(cfg *AppConfig) error {
	// 确保目录存在
	if err := os.MkdirAll(l.configDir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// 递增版本号
	cfg.Version++

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(l.configPath, data, 0644)
}

// Exists 检查配置文件是否存在
func (l *Loader) Exists() bool {
	_, err := os.Stat(l.configPath)
	return err == nil
}

func (l *Loader) ProjectRoot() string {
	return l.projectRoot
}

func (l *Loader) ConfigPath() string {
	return l.configPath
}

// mergeDefaults 将默认值合并到配置中（对于零值字段）
func mergeDefaults(cfg AppConfig) AppConfig {
	defaults := DefaultConfig()
	loadedVersion := cfg.Version

	if cfg.Version == 0 {
		cfg.Version = defaults.Version
	}
	if loadedVersion < 3 {
		cfg.Codex = defaults.Codex
		cfg.TokenAccounting = defaults.TokenAccounting
	}
	if cfg.Providers.DefaultController == "" {
		cfg.Providers.DefaultController = defaults.Providers.DefaultController
	}
	if cfg.Providers.DefaultExecutor == "" {
		cfg.Providers.DefaultExecutor = defaults.Providers.DefaultExecutor
	}
	if cfg.ProviderSelection.Mode == "" {
		cfg.ProviderSelection.Mode = defaults.ProviderSelection.Mode
	}
	if len(cfg.ProviderSelection.FallbackOrder) == 0 {
		cfg.ProviderSelection.FallbackOrder = defaults.ProviderSelection.FallbackOrder
	}
	if cfg.Security.ForbiddenFileReadPolicy.Default == "" {
		cfg.Security.ForbiddenFileReadPolicy = defaults.Security.ForbiddenFileReadPolicy
	}
	if cfg.Risk.HighRiskPolicy.GitSnapshotMode == "" {
		cfg.Risk.HighRiskPolicy.GitSnapshotMode = defaults.Risk.HighRiskPolicy.GitSnapshotMode
	}
	if cfg.Risk.HighRiskPolicy.NonGitSnapshotMode == "" {
		cfg.Risk.HighRiskPolicy.NonGitSnapshotMode = defaults.Risk.HighRiskPolicy.NonGitSnapshotMode
	}
	if cfg.Encoding.DefaultEncoding == "" {
		cfg.Encoding.DefaultEncoding = defaults.Encoding.DefaultEncoding
	}
	if len(cfg.Encoding.FallbackEncoding) == 0 {
		cfg.Encoding.FallbackEncoding = defaults.Encoding.FallbackEncoding
	}
	if cfg.Report.OutputDir == "" {
		cfg.Report.OutputDir = defaults.Report.OutputDir
	}
	if cfg.Report.Format == "" {
		cfg.Report.Format = defaults.Report.Format
	}
	if cfg.Report.MaxHistory == 0 {
		cfg.Report.MaxHistory = defaults.Report.MaxHistory
	}
	if cfg.Web.Host == "" {
		cfg.Web.Host = defaults.Web.Host
	}
	if cfg.Web.Port == 0 {
		cfg.Web.Port = defaults.Web.Port
	}
	if cfg.Execution.PatchMaxTokens == 0 {
		cfg.Execution.PatchMaxTokens = defaults.Execution.PatchMaxTokens
	}
	if loadedVersion < 3 {
		cfg.Execution.Temperature = defaults.Execution.Temperature
	}
	if loadedVersion < 4 {
		cfg.Execution.MaxRepairAttempts = defaults.Execution.MaxRepairAttempts
	}
	if loadedVersion < 5 {
		cfg.Execution.EnforceTaskBudgets = defaults.Execution.EnforceTaskBudgets
		cfg.Execution.MaxTaskFiles = defaults.Execution.MaxTaskFiles
		cfg.Execution.MaxContextBytes = defaults.Execution.MaxContextBytes
		cfg.Execution.MaxPatchLines = defaults.Execution.MaxPatchLines
	}
	if cfg.TokenAccounting.DirectCodexBaselineRatio == 0 {
		cfg.TokenAccounting.DirectCodexBaselineRatio = defaults.TokenAccounting.DirectCodexBaselineRatio
	}

	if cfg.Timeouts.ProviderRequest == 0 {
		cfg.Timeouts.ProviderRequest = defaults.Timeouts.ProviderRequest
	}
	if cfg.Timeouts.ProviderHealth == 0 {
		cfg.Timeouts.ProviderHealth = defaults.Timeouts.ProviderHealth
	}
	if cfg.Timeouts.PatchApply == 0 {
		cfg.Timeouts.PatchApply = defaults.Timeouts.PatchApply
	}
	if cfg.Timeouts.Command == 0 {
		cfg.Timeouts.Command = defaults.Timeouts.Command
	}
	if cfg.Timeouts.WebRequest == 0 {
		cfg.Timeouts.WebRequest = defaults.Timeouts.WebRequest
	}

	return cfg
}

// LoadOrInit 加载配置，如果不存在则创建默认配置
func (l *Loader) LoadOrInit() (*AppConfig, error) {
	cfg, err := l.Load()
	if err != nil {
		return nil, err
	}

	// 如果配置文件不存在，保存默认配置
	if !l.Exists() {
		if err := l.Save(cfg); err != nil {
			return nil, fmt.Errorf("save default config: %w", err)
		}
	}

	return cfg, nil
}

// DetectProjectRoot 从当前目录向上查找项目根目录
// 通过查找 .coding-bridge 目录或 .git 目录来确定
func DetectProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}

	for {
		// 检查 .coding-bridge 目录
		bridgeDir := filepath.Join(dir, ".coding-bridge")
		if info, err := os.Stat(bridgeDir); err == nil && info.IsDir() {
			return dir, nil
		}

		// 检查 .git 目录
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// 到达根目录，返回当前目录
			return os.Getwd()
		}
		dir = parent
	}
}

// ResolveEnvVars 解析环境变量引用 ${VAR_NAME}
func ResolveEnvVars(s string) string {
	return os.ExpandEnv(s)
}

// MaskAPIKey 脱敏 API Key（用于日志输出）
func MaskAPIKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}
