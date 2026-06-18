package providers

import (
	"context"
	"time"
)

// DeepSeekProvider DeepSeek 专用 Provider。
// DeepSeek API 完全兼容 OpenAI 格式，基于 OpenAICompatibleProvider 实现。
// 默认 BaseURL: https://api.deepseek.com
// 支持模型：deepseek-chat, deepseek-reasoner (deepseek-v4-pro 等)
type DeepSeekProvider struct {
	*OpenAICompatibleProvider
}

// NewDeepSeekProvider 创建 DeepSeek Provider
func NewDeepSeekProvider(apiKey string, model string) *DeepSeekProvider {
	if model == "" {
		model = "deepseek-chat" // 默认模型
	}

	cfg := ProviderConfig{
		Type:     ProviderDeepSeek,
		Name:     "DeepSeek",
		BaseURL:  "https://api.deepseek.com",
		APIKey:   apiKey,
		Model:    model,
		Timeout:  120 * time.Second,
		MaxRetry: 2,
	}

	base := NewOpenAICompatible(cfg)
	// DeepSeek 额外能力
	base.capabilities = append(base.capabilities, CapabilityLongContext)

	return &DeepSeekProvider{OpenAICompatibleProvider: base}
}

// NewDeepSeekProviderWithConfig 使用自定义配置创建 DeepSeek Provider
// 允许自定义 BaseURL（如代理地址）、超时等
func NewDeepSeekProviderWithConfig(cfg ProviderConfig) *DeepSeekProvider {
	cfg.Type = ProviderDeepSeek
	if cfg.Name == "" {
		cfg.Name = "DeepSeek"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepseek.com"
	}
	if cfg.Model == "" {
		cfg.Model = "deepseek-chat"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 120 * time.Second
	}
	if cfg.MaxRetry == 0 {
		cfg.MaxRetry = 2
	}

	base := NewOpenAICompatible(cfg)
	base.capabilities = append(base.capabilities, CapabilityLongContext)

	return &DeepSeekProvider{OpenAICompatibleProvider: base}
}

// Type 返回 Provider 类型
func (p *DeepSeekProvider) Type() ProviderType {
	return ProviderDeepSeek
}

// HealthCheck DeepSeek 专用健康检测
// DeepSeek 的 /models 端点可能返回所有模型，"deepseek-chat" 和 "deepseek-reasoner"
func (p *DeepSeekProvider) HealthCheck(ctx context.Context) (*HealthCheckResult, error) {
	result, err := p.OpenAICompatibleProvider.HealthCheck(ctx)
	if err != nil {
		return result, err
	}
	result.Provider = ProviderDeepSeek
	// DeepSeek 的成本估算（参考价格，可能会变）
	result.CostPer1K = 0.14 / 1000 // deepseek-chat: $0.14/1M input tokens ≈ $0.00014/1K
	result.Capabilities = p.capabilities
	return result, nil
}

// Generate 使用 DeepSeek 优化的参数生成补全
func (p *DeepSeekProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	// DeepSeek 不需要 special_announcement 等 OpenAI 特有字段
	// 直接使用 OpenAI 兼容接口
	return p.OpenAICompatibleProvider.Generate(ctx, req)
}
