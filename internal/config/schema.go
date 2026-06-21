// Package config 提供配置的 schema 定义、加载、校验功能。
// 所有配置必须经过 schema 校验后才能使用。
package config

import (
	"fmt"
	"time"
)

// AppConfig 是 coding-bridge 的完整配置结构
type AppConfig struct {
	// 配置版本号，每次变化递增
	Version int `yaml:"config_version" json:"config_version"`

	// Provider 配置
	Providers ProviderSection `yaml:"providers" json:"providers"`

	// Provider 选择策略
	ProviderSelection ProviderSelectionSection `yaml:"provider_selection" json:"provider_selection"`

	// 安全配置
	Security SecuritySection `yaml:"security" json:"security"`

	// 风险控制
	Risk RiskSection `yaml:"risk" json:"risk"`

	// 命令白名单/黑名单
	Commands CommandsSection `yaml:"commands" json:"commands"`

	// 超时控制
	Timeouts TimeoutsSection `yaml:"timeouts" json:"timeouts"`

	// 编码策略
	Encoding EncodingSection `yaml:"encoding" json:"encoding"`

	// 报告配置
	Report ReportSection `yaml:"report" json:"report"`

	// Web 服务配置
	Web WebSection `yaml:"web" json:"web"`

	// Executor 执行参数
	Execution ExecutionSection `yaml:"execution" json:"execution"`

	// Codex 接入策略
	Codex CodexSection `yaml:"codex" json:"codex"`

	// Token 统计与节省估算
	TokenAccounting TokenAccountingSection `yaml:"token_accounting" json:"token_accounting"`
}

// ProviderSection Provider 相关配置
type ProviderSection struct {
	// 默认 Controller Provider
	DefaultController string `yaml:"default_controller" json:"default_controller"`
	// 默认 Executor Provider
	DefaultExecutor string `yaml:"default_executor" json:"default_executor"`
	// 各 Provider 的具体配置
	Configs map[string]ProviderItemConfig `yaml:"configs" json:"configs"`
}

// ProviderItemConfig 单个 Provider 的配置项
type ProviderItemConfig struct {
	Type     string   `yaml:"type" json:"type"`
	BaseURL  string   `yaml:"base_url" json:"base_url"`
	APIKey   string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Model    string   `yaml:"model,omitempty" json:"model,omitempty"`   // 单模型（向后兼容）
	Models   []string `yaml:"models,omitempty" json:"models,omitempty"` // 多模型
	Timeout  int      `yaml:"timeout_seconds" json:"timeout_seconds"`
	MaxRetry int      `yaml:"max_retry" json:"max_retry"`
	Enabled  bool     `yaml:"enabled" json:"enabled"`
}

// GetModels 返回模型列表（兼容单/多模型配置）
func (p *ProviderItemConfig) GetModels() []string {
	if len(p.Models) > 0 {
		return p.Models
	}
	if p.Model != "" {
		return []string{p.Model}
	}
	return nil
}

// ProviderSelectionSection Provider 选择策略
type ProviderSelectionSection struct {
	Mode     string   `yaml:"mode" json:"mode"` // auto, manual
	Strategy Strategy `yaml:"strategy" json:"strategy"`
	// 回退顺序
	FallbackOrder []string `yaml:"fallback_order" json:"fallback_order"`
}

// Strategy 选择策略
type Strategy struct {
	PreferAvailable     bool `yaml:"prefer_available" json:"prefer_available"`
	PreferLowCost       bool `yaml:"prefer_low_cost" json:"prefer_low_cost"`
	PreferFastResponse  bool `yaml:"prefer_fast_response" json:"prefer_fast_response"`
	PreferPatchAccuracy bool `yaml:"prefer_patch_accuracy" json:"prefer_patch_accuracy"`
}

// SecuritySection 安全配置
type SecuritySection struct {
	// 禁止文件读取策略
	ForbiddenFileReadPolicy ForbiddenFilePolicy `yaml:"forbidden_file_read_policy" json:"forbidden_file_read_policy"`
}

// ForbiddenFilePolicy 禁止文件读取策略
type ForbiddenFilePolicy struct {
	Default                   string `yaml:"default" json:"default"`                                             // deny, ask, allow
	AllowUserOverride         bool   `yaml:"allow_user_override" json:"allow_user_override"`                     // 是否允许用户覆盖
	RequireReason             bool   `yaml:"require_reason" json:"require_reason"`                               // 是否需要说明原因
	AllowLocalRead            bool   `yaml:"allow_local_read" json:"allow_local_read"`                           // 是否允许本地读取
	AllowSendMaskedToProvider bool   `yaml:"allow_send_masked_to_provider" json:"allow_send_masked_to_provider"` // 是否允许脱敏后发送
	AllowSendRawToProvider    bool   `yaml:"allow_send_raw_to_provider" json:"allow_send_raw_to_provider"`       // 是否允许原文发送
	AuditLog                  bool   `yaml:"audit_log" json:"audit_log"`                                         // 是否写入审计日志
}

// RiskSection 风险控制配置
type RiskSection struct {
	AllowHighRiskOperations bool           `yaml:"allow_high_risk_operations" json:"allow_high_risk_operations"`
	HighRiskPolicy          HighRiskPolicy `yaml:"high_risk_policy" json:"high_risk_policy"`
}

// HighRiskPolicy 高危操作策略
type HighRiskPolicy struct {
	RequireUserConfirm           bool   `yaml:"require_user_confirm" json:"require_user_confirm"`
	RequireSnapshotBeforeExecute bool   `yaml:"require_snapshot_before_execute" json:"require_snapshot_before_execute"`
	GitSnapshotMode              string `yaml:"git_snapshot_mode" json:"git_snapshot_mode"`         // worktree, branch, stash
	NonGitSnapshotMode           string `yaml:"non_git_snapshot_mode" json:"non_git_snapshot_mode"` // bak
	AllowDeleteFiles             bool   `yaml:"allow_delete_files" json:"allow_delete_files"`
	AllowModifyConfig            bool   `yaml:"allow_modify_config" json:"allow_modify_config"`
	AllowRunShell                bool   `yaml:"allow_run_shell" json:"allow_run_shell"`
	AllowDatabaseWrite           bool   `yaml:"allow_database_write" json:"allow_database_write"`
}

// CommandsSection 命令策略
type CommandsSection struct {
	Allowed   []string `yaml:"allowed" json:"allowed"`
	Forbidden []string `yaml:"forbidden" json:"forbidden"`
}

// TimeoutsSection 超时配置（单位：秒）
type TimeoutsSection struct {
	ProviderRequest int `yaml:"provider_request_seconds" json:"provider_request_seconds"`
	ProviderHealth  int `yaml:"provider_health_seconds" json:"provider_health_seconds"`
	PatchApply      int `yaml:"patch_apply_seconds" json:"patch_apply_seconds"`
	Command         int `yaml:"command_seconds" json:"command_seconds"`
	WebRequest      int `yaml:"web_request_seconds" json:"web_request_seconds"`
}

// EncodingSection 编码策略
type EncodingSection struct {
	DetectBeforeWrite            bool     `yaml:"detect_before_write" json:"detect_before_write"`
	PreserveOriginalEncoding     bool     `yaml:"preserve_original_encoding" json:"preserve_original_encoding"`
	PreserveLineEndings          bool     `yaml:"preserve_line_endings" json:"preserve_line_endings"`
	RejectOnDecodeError          bool     `yaml:"reject_on_decode_error" json:"reject_on_decode_error"`
	RejectIfReplacementCharFound bool     `yaml:"reject_if_replacement_char_found" json:"reject_if_replacement_char_found"`
	DefaultEncoding              string   `yaml:"default_encoding" json:"default_encoding"`
	FallbackEncoding             []string `yaml:"fallback_encoding" json:"fallback_encoding"`
}

// ReportSection 报告配置
type ReportSection struct {
	OutputDir  string `yaml:"output_dir" json:"output_dir"`
	Format     string `yaml:"format" json:"format"` // markdown, json
	MaxHistory int    `yaml:"max_history" json:"max_history"`
}

// WebSection Web 服务配置
type WebSection struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Host    string `yaml:"host" json:"host"`
	Port    int    `yaml:"port" json:"port"`
	Token   string `yaml:"token,omitempty" json:"token,omitempty"`
}

type ExecutionSection struct {
	PatchMaxTokens     int     `yaml:"patch_max_tokens" json:"patch_max_tokens"`
	Temperature        float64 `yaml:"temperature" json:"temperature"`
	MaxRepairAttempts  int     `yaml:"max_repair_attempts" json:"max_repair_attempts"`
	EnforceTaskBudgets bool    `yaml:"enforce_task_budgets" json:"enforce_task_budgets"`
	MaxTaskFiles       int     `yaml:"max_task_files" json:"max_task_files"`
	MaxContextBytes    int     `yaml:"max_context_bytes" json:"max_context_bytes"`
	MaxPatchLines      int     `yaml:"max_patch_lines" json:"max_patch_lines"`
}

type CodexSection struct {
	CLIEnabled                   bool `yaml:"cli_enabled" json:"cli_enabled"`
	DefaultCLIForCodingTasks     bool `yaml:"default_cli_for_coding_tasks" json:"default_cli_for_coding_tasks"`
	FallbackToCodexOnUnavailable bool `yaml:"fallback_to_codex_on_unavailable" json:"fallback_to_codex_on_unavailable"`
	ExternalCodeSharingApproved  bool `yaml:"external_code_sharing_approved" json:"external_code_sharing_approved"`
}

type TokenAccountingSection struct {
	Enabled                  bool    `yaml:"enabled" json:"enabled"`
	DirectCodexBaselineRatio float64 `yaml:"direct_codex_baseline_ratio" json:"direct_codex_baseline_ratio"`
}

// DefaultConfig 返回带有合理默认值的配置
func DefaultConfig() *AppConfig {
	return &AppConfig{
		Version: 5,
		Providers: ProviderSection{
			DefaultController: "openai",
			DefaultExecutor:   "deepseek",
			Configs: map[string]ProviderItemConfig{
				"deepseek": {
					Type:     "deepseek",
					BaseURL:  "https://api.deepseek.com",
					Models:   []string{"deepseek-chat"},
					Timeout:  120,
					MaxRetry: 2,
					Enabled:  true,
				},
			},
		},
		ProviderSelection: ProviderSelectionSection{
			Mode: "auto",
			Strategy: Strategy{
				PreferAvailable:     true,
				PreferLowCost:       true,
				PreferFastResponse:  true,
				PreferPatchAccuracy: true,
			},
			FallbackOrder: []string{"deepseek", "openai"},
		},
		Security: SecuritySection{
			ForbiddenFileReadPolicy: ForbiddenFilePolicy{
				Default:                   "deny",
				AllowUserOverride:         true,
				RequireReason:             true,
				AllowLocalRead:            true,
				AllowSendMaskedToProvider: true,
				AllowSendRawToProvider:    false,
				AuditLog:                  true,
			},
		},
		Risk: RiskSection{
			AllowHighRiskOperations: false,
			HighRiskPolicy: HighRiskPolicy{
				RequireUserConfirm:           true,
				RequireSnapshotBeforeExecute: true,
				GitSnapshotMode:              "worktree",
				NonGitSnapshotMode:           "bak",
				AllowDeleteFiles:             false,
				AllowModifyConfig:            false,
				AllowRunShell:                false,
				AllowDatabaseWrite:           false,
			},
		},
		Commands: CommandsSection{
			Allowed: []string{
				"dotnet build",
				"dotnet test",
				"npm test",
				"npm run build",
				"pytest",
				"go build",
				"go test",
				"go vet",
			},
			Forbidden: []string{
				"rm -rf",
				"git reset --hard",
				"git clean -fdx",
				"ssh",
				"scp",
				"curl",
				"wget",
				"powershell iwr",
				"Invoke-WebRequest",
			},
		},
		Timeouts: TimeoutsSection{
			ProviderRequest: 120,
			ProviderHealth:  10,
			PatchApply:      30,
			Command:         300,
			WebRequest:      30,
		},
		Encoding: EncodingSection{
			DetectBeforeWrite:            true,
			PreserveOriginalEncoding:     true,
			PreserveLineEndings:          true,
			RejectOnDecodeError:          true,
			RejectIfReplacementCharFound: true,
			DefaultEncoding:              "utf-8",
			FallbackEncoding:             []string{"utf-8-sig", "gbk", "gb2312"},
		},
		Report: ReportSection{
			OutputDir:  ".coding-bridge/reports",
			Format:     "markdown",
			MaxHistory: 100,
		},
		Web: WebSection{
			Enabled: false,
			Host:    "127.0.0.1",
			Port:    8765,
		},
		Execution: ExecutionSection{
			PatchMaxTokens:     16384,
			Temperature:        0.1,
			MaxRepairAttempts:  2,
			EnforceTaskBudgets: true,
			MaxTaskFiles:       3,
			MaxContextBytes:    49152,
			MaxPatchLines:      200,
		},
		Codex: CodexSection{
			CLIEnabled:                   true,
			DefaultCLIForCodingTasks:     true,
			FallbackToCodexOnUnavailable: true,
			ExternalCodeSharingApproved:  true,
		},
		TokenAccounting: TokenAccountingSection{
			Enabled:                  true,
			DirectCodexBaselineRatio: 3,
		},
	}
}

// Validate 校验配置
func (c *AppConfig) Validate() []error {
	var errs []error

	if c.Version < 1 {
		errs = append(errs, fmt.Errorf("config_version must be >= 1"))
	}

	if c.Providers.DefaultController == "" {
		errs = append(errs, fmt.Errorf("providers.default_controller is required"))
	}

	if c.Providers.DefaultExecutor == "" {
		errs = append(errs, fmt.Errorf("providers.default_executor is required"))
	}
	if len(c.Providers.Configs) == 0 {
		errs = append(errs, fmt.Errorf("providers.configs must contain at least one provider"))
	}
	defaultExecutor, ok := c.Providers.Configs[c.Providers.DefaultExecutor]
	if !ok {
		errs = append(errs, fmt.Errorf("providers.default_executor %q is not configured", c.Providers.DefaultExecutor))
	} else if !defaultExecutor.Enabled {
		errs = append(errs, fmt.Errorf("providers.default_executor %q is disabled", c.Providers.DefaultExecutor))
	}
	for name, provider := range c.Providers.Configs {
		if provider.Type == "" {
			errs = append(errs, fmt.Errorf("providers.configs.%s.type is required", name))
		}
		if provider.Enabled && provider.BaseURL == "" {
			errs = append(errs, fmt.Errorf("providers.configs.%s.base_url is required when enabled", name))
		}
		if provider.Enabled && len(provider.GetModels()) == 0 {
			errs = append(errs, fmt.Errorf("providers.configs.%s requires at least one model", name))
		}
	}

	if c.Timeouts.ProviderRequest < 1 {
		errs = append(errs, fmt.Errorf("timeouts.provider_request_seconds must be >= 1"))
	}

	if c.Timeouts.Command < 1 {
		errs = append(errs, fmt.Errorf("timeouts.command_seconds must be >= 1"))
	}

	if c.Report.OutputDir == "" {
		errs = append(errs, fmt.Errorf("report.output_dir is required"))
	}
	if len(c.Commands.Allowed) == 0 {
		errs = append(errs, fmt.Errorf("commands.allowed must not be empty"))
	}
	if c.ProviderSelection.Mode != "auto" && c.ProviderSelection.Mode != "manual" {
		errs = append(errs, fmt.Errorf("provider_selection.mode must be auto or manual"))
	}
	if c.Execution.PatchMaxTokens < 256 {
		errs = append(errs, fmt.Errorf("execution.patch_max_tokens must be >= 256"))
	}
	if c.Execution.Temperature < 0 || c.Execution.Temperature > 2 {
		errs = append(errs, fmt.Errorf("execution.temperature must be between 0 and 2"))
	}
	if c.Execution.MaxRepairAttempts < 0 || c.Execution.MaxRepairAttempts > 5 {
		errs = append(errs, fmt.Errorf("execution.max_repair_attempts must be between 0 and 5"))
	}
	if c.Execution.MaxTaskFiles < 1 {
		errs = append(errs, fmt.Errorf("execution.max_task_files must be >= 1"))
	}
	if c.Execution.MaxContextBytes < 1024 {
		errs = append(errs, fmt.Errorf("execution.max_context_bytes must be >= 1024"))
	}
	if c.Execution.MaxPatchLines < 1 {
		errs = append(errs, fmt.Errorf("execution.max_patch_lines must be >= 1"))
	}
	if c.TokenAccounting.DirectCodexBaselineRatio < 1 {
		errs = append(errs, fmt.Errorf("token_accounting.direct_codex_baseline_ratio must be >= 1"))
	}
	switch c.Security.ForbiddenFileReadPolicy.Default {
	case "deny", "ask", "allow":
	default:
		errs = append(errs, fmt.Errorf("security.forbidden_file_read_policy.default must be deny, ask, or allow"))
	}

	return errs
}

// GetTimeout 获取指定超时值并转为 time.Duration
func (t *TimeoutsSection) ProviderRequestTimeout() time.Duration {
	return time.Duration(t.ProviderRequest) * time.Second
}

func (t *TimeoutsSection) ProviderHealthTimeout() time.Duration {
	return time.Duration(t.ProviderHealth) * time.Second
}

func (t *TimeoutsSection) PatchApplyTimeout() time.Duration {
	return time.Duration(t.PatchApply) * time.Second
}

func (t *TimeoutsSection) CommandTimeout() time.Duration {
	return time.Duration(t.Command) * time.Second
}
