package providers

import (
	"context"
	"time"
)

// CodexProvider Codex (OpenAI GPT 系列) 专用 Provider。
// Codex 使用标准 OpenAI API，作为 Controller Model 角色。
// 默认 BaseURL: https://api.openai.com
// 支持模型：gpt-5, gpt-4o, gpt-4-turbo, gpt-4, gpt-3.5-turbo 等
type CodexProvider struct {
	*OpenAICompatibleProvider
}

// NewCodexProvider 创建 Codex Provider（使用默认 OpenAI API）
func NewCodexProvider(apiKey string, model string) *CodexProvider {
	if model == "" {
		model = "gpt-4o" // 默认模型
	}

	cfg := ProviderConfig{
		Type:     ProviderOpenAI,
		Name:     "Codex (OpenAI)",
		BaseURL:  "https://api.openai.com/v1",
		APIKey:   apiKey,
		Model:    model,
		Timeout:  120 * time.Second,
		MaxRetry: 2,
	}

	base := NewOpenAICompatible(cfg)
	// Codex/OpenAI 具有全部高级能力
	base.capabilities = append(base.capabilities,
		CapabilityLongContext,
		CapabilityToolCall,
		CapabilityVision,
	)

	return &CodexProvider{OpenAICompatibleProvider: base}
}

// NewCodexProviderWithConfig 使用自定义配置创建 Codex Provider
// 适用于代理、Azure OpenAI 等场景
func NewCodexProviderWithConfig(cfg ProviderConfig) *CodexProvider {
	cfg.Type = ProviderOpenAI
	if cfg.Name == "" {
		cfg.Name = "Codex (OpenAI)"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 120 * time.Second
	}
	if cfg.MaxRetry == 0 {
		cfg.MaxRetry = 2
	}

	base := NewOpenAICompatible(cfg)
	base.capabilities = append(base.capabilities,
		CapabilityLongContext,
		CapabilityToolCall,
		CapabilityVision,
	)

	return &CodexProvider{OpenAICompatibleProvider: base}
}

// Type 返回 Provider 类型
func (p *CodexProvider) Type() ProviderType {
	return ProviderOpenAI
}

// Name 返回名称
func (p *CodexProvider) Name() string {
	return "Codex (OpenAI)"
}

// HealthCheck Codex 专用健康检测
func (p *CodexProvider) HealthCheck(ctx context.Context) (*HealthCheckResult, error) {
	result, err := p.OpenAICompatibleProvider.HealthCheck(ctx)
	if err != nil {
		return result, err
	}
	result.Provider = ProviderOpenAI
	result.Capabilities = p.capabilities
	return result, nil
}
