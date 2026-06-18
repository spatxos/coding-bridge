package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatibleProvider 实现 OpenAI 兼容 API 的通用 Provider。
// Codex、DeepSeek、Qwen、Ollama 等所有兼容 OpenAI API 格式的服务均可通过此 Provider 接入，
// 只需配置不同的 BaseURL 和模型名即可。
type OpenAICompatibleProvider struct {
	config       ProviderConfig
	capabilities []ModelCapability
	httpClient   *http.Client
}

// NewOpenAICompatible 创建 OpenAI 兼容 Provider
func NewOpenAICompatible(cfg ProviderConfig) *OpenAICompatibleProvider {
	if cfg.Timeout == 0 {
		cfg.Timeout = 120 * time.Second
	}
	if cfg.MaxRetry == 0 {
		cfg.MaxRetry = 2
	}
	return &OpenAICompatibleProvider{
		config: cfg,
		capabilities: []ModelCapability{
			CapabilityChat,
			CapabilityJSON,
			CapabilityPatch,
		},
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Type 返回 Provider 类型
func (p *OpenAICompatibleProvider) Type() ProviderType {
	return p.config.Type
}

// Name 返回可读名称
func (p *OpenAICompatibleProvider) Name() string {
	if p.config.Name != "" {
		return p.config.Name
	}
	return string(p.config.Type)
}

// openAIChatRequest OpenAI 兼容的 chat completions 请求体
type openAIChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream"`
}

// openAIChatResponse OpenAI 兼容的 chat completions 响应体
type openAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Generate 发送补全请求
func (p *OpenAICompatibleProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = p.config.Model
	}

	messages := req.Messages
	if req.SystemPrompt != "" {
		messages = append([]ChatMessage{{Role: "system", Content: req.SystemPrompt}}, messages...)
	}

	chatReq := openAIChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      false,
	}

	startTime := time.Now()

	bodyBytes, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	apiURL := strings.TrimRight(p.config.BaseURL, "/") + "/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}

	// 重试逻辑
	var lastErr error
	var chatResp *openAIChatResponse
	for attempt := 0; attempt <= p.config.MaxRetry; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}

		chatResp, lastErr = p.doRequest(httpReq)
		if lastErr == nil {
			break
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}

	resp := &GenerateResponse{
		Model:   chatResp.Model,
		Latency: time.Since(startTime),
		Usage: UsageInfo{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
	}

	if len(chatResp.Choices) > 0 {
		resp.Content = chatResp.Choices[0].Message.Content
		resp.FinishReason = chatResp.Choices[0].FinishReason
	}

	return resp, nil
}

func (p *OpenAICompatibleProvider) doRequest(req *http.Request) (*openAIChatResponse, error) {
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	// 处理非 200 状态码，读取响应体获取详细错误信息
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("API error [%s]: %s", chatResp.Error.Code, chatResp.Error.Message)
	}

	return &chatResp, nil
}

// listModelsResponse OpenAI 兼容的模型列表响应
type listModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// ListModels 返回可用模型列表
func (p *OpenAICompatibleProvider) ListModels(ctx context.Context) ([]string, error) {
	apiURL := strings.TrimRight(p.config.BaseURL, "/") + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if p.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var listResp listModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	models := make([]string, 0, len(listResp.Data))
	for _, m := range listResp.Data {
		models = append(models, m.ID)
	}
	return models, nil
}

// HealthCheck 健康检测
func (p *OpenAICompatibleProvider) HealthCheck(ctx context.Context) (*HealthCheckResult, error) {
	result := &HealthCheckResult{
		Provider: p.config.Type,
		Model:    p.config.Model,
		Status:   HealthUnknown,
	}

	// 1. 连通性检测：列出模型
	startTime := time.Now()
	models, err := p.ListModels(ctx)
	result.Latency = time.Since(startTime)

	if err != nil {
		result.Status = HealthUnhealthy
		result.Connectivity = false
		result.Error = err.Error()
		return result, nil
	}
	result.Connectivity = true

	// 2. 模型存在性
	result.ModelExists = false
	for _, m := range models {
		if strings.EqualFold(m, p.config.Model) {
			result.ModelExists = true
			break
		}
	}

	// 3. 认证检测：发送最小测试请求
	testReq := &GenerateRequest{
		Model:     p.config.Model,
		MaxTokens: 1,
		Messages: []ChatMessage{
			{Role: "user", Content: "hi"},
		},
	}
	_, authErr := p.Generate(ctx, testReq)
	result.Auth = authErr == nil

	// 4. 综合判断状态
	switch {
	case !result.Connectivity:
		result.Status = HealthUnhealthy
	case !result.Auth:
		result.Status = HealthUnhealthy
	case !result.ModelExists:
		result.Status = HealthDegraded
	default:
		result.Status = HealthHealthy
	}

	result.CheckedAt = time.Now()
	return result, nil
}

// SupportsCapability 检查能力
func (p *OpenAICompatibleProvider) SupportsCapability(cap ModelCapability) bool {
	for _, c := range p.capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// IsAvailable 快速可用性检查
func (p *OpenAICompatibleProvider) IsAvailable(ctx context.Context) bool {
	apiURL := strings.TrimRight(p.config.BaseURL, "/") + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false
	}
	if p.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// Ensure io and bufio are used (prevent import removal by some IDEs)
var _ = bufio.ScanLines
var _ io.Reader
