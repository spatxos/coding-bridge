// Package providers 定义了 AI Provider 的统一接口与注册机制。
// 所有 Provider（Codex、DeepSeek、OpenAI、Qwen 等）必须实现 Provider 接口。
// 通过注册中心（Registry）实现可插拔的 Provider 管理。
package providers

import (
	"context"
	"time"
)

// ProviderType 标识 Provider 类型
type ProviderType string

const (
	ProviderOpenAICompat ProviderType = "openai_compatible" // 通用 OpenAI 兼容 API
	ProviderDeepSeek     ProviderType = "deepseek"
	ProviderOpenAI       ProviderType = "openai"
	ProviderQwen         ProviderType = "qwen"
	ProviderClaude       ProviderType = "claude"
	ProviderGemini       ProviderType = "gemini"
	ProviderOllama       ProviderType = "ollama"
	ProviderLMStudio     ProviderType = "lmstudio"
	ProviderGitHubModels ProviderType = "github_models"
	ProviderCopilotCLI   ProviderType = "copilot_cli"
)

// ModelCapability 描述模型能力
type ModelCapability string

const (
	CapabilityChat           ModelCapability = "chat"
	CapabilityJSON           ModelCapability = "json_mode"
	CapabilityPatch          ModelCapability = "patch_only"
	CapabilityLongContext    ModelCapability = "long_context"
	CapabilityToolCall       ModelCapability = "tool_call"
	CapabilityVision         ModelCapability = "vision"
)

// HealthStatus 表示 Provider 健康状态
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
	HealthUnknown   HealthStatus = "unknown"
)

// HealthCheckResult Provider 健康检测结果
type HealthCheckResult struct {
	Provider     ProviderType `json:"provider" yaml:"provider"`
	Model        string       `json:"model" yaml:"model"`
	Status       HealthStatus `json:"status" yaml:"status"`
	Connectivity bool         `json:"connectivity" yaml:"connectivity"`
	Auth         bool         `json:"auth" yaml:"auth"`
	ModelExists  bool         `json:"model_exists" yaml:"model_exists"`
	Latency      time.Duration `json:"latency" yaml:"latency"`
	Quota        bool         `json:"quota" yaml:"quota"`
	RecentSuccess float64     `json:"recent_success_rate" yaml:"recent_success_rate"`
	CostPer1K    float64      `json:"cost_per_1k_tokens" yaml:"cost_per_1k_tokens"`
	Capabilities []ModelCapability `json:"capabilities" yaml:"capabilities"`
	Error        string       `json:"error,omitempty" yaml:"error,omitempty"`
	CheckedAt    time.Time    `json:"checked_at" yaml:"checked_at"`
}

// ChatMessage 表示一条对话消息
type ChatMessage struct {
	Role    string `json:"role"`    // system, user, assistant
	Content string `json:"content"` // 消息内容
}

// GenerateRequest 补全请求
type GenerateRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	SystemPrompt string       `json:"system_prompt,omitempty"`
	// 用于 Patch-only 模式：强制要求模型只返回 unified diff
	PatchOnly   bool          `json:"patch_only,omitempty"`
}

// GenerateResponse 补全响应
type GenerateResponse struct {
	Content      string        `json:"content"`
	FinishReason string        `json:"finish_reason"`
	Usage        UsageInfo     `json:"usage"`
	Model        string        `json:"model"`
	Latency      time.Duration `json:"latency"`
}

// UsageInfo token 用量
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Provider 是所有 AI Provider 必须实现的接口。
// 新增 Provider 只需实现此接口并注册到 Registry。
type Provider interface {
	// Type 返回 Provider 类型标识
	Type() ProviderType

	// Name 返回可读名称
	Name() string

	// Generate 发送补全请求并返回结果。
	// 这是核心方法，所有 Provider 必须实现。
	Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)

	// HealthCheck 执行健康检测，返回详细的健康状态。
	HealthCheck(ctx context.Context) (*HealthCheckResult, error)

	// ListModels 返回该 Provider 可用的模型列表
	ListModels(ctx context.Context) ([]string, error)

	// SupportsCapability 检查是否支持特定能力
	SupportsCapability(cap ModelCapability) bool

	// IsAvailable 快速检查 Provider 是否可用（轻量级）
	IsAvailable(ctx context.Context) bool
}

// ProviderConfig Provider 通用配置
type ProviderConfig struct {
	Type     ProviderType `json:"type" yaml:"type"`
	Name     string       `json:"name" yaml:"name"`
	BaseURL  string       `json:"base_url" yaml:"base_url"`
	APIKey   string       `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	Model    string       `json:"model" yaml:"model"`
	Timeout  time.Duration `json:"timeout" yaml:"timeout"`
	MaxRetry int          `json:"max_retry" yaml:"max_retry"`
	// 扩展字段：不同 Provider 的自定义配置
	Extra map[string]any `json:"extra,omitempty" yaml:"extra,omitempty"`
}
