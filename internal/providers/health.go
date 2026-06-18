package providers

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// HealthChecker 对注册中心中的所有 Provider 执行批量健康检测
type HealthChecker struct {
	registry *Registry
	results  map[ProviderType]*HealthCheckResult
	mu       sync.RWMutex
}

// NewHealthChecker 创建健康检测器
func NewHealthChecker(registry *Registry) *HealthChecker {
	return &HealthChecker{
		registry: registry,
		results:  make(map[ProviderType]*HealthCheckResult),
	}
}

// CheckAll 对所有已注册 Provider 执行健康检测
func (hc *HealthChecker) CheckAll(ctx context.Context) map[ProviderType]*HealthCheckResult {
	providers := hc.registry.ListAll()
	results := make(map[ProviderType]*HealthCheckResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range providers {
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()
			result, err := p.HealthCheck(ctx)
			if err != nil {
				result = &HealthCheckResult{
					Provider: p.Type(),
					Model:    "",
					Status:   HealthUnhealthy,
					Error:    err.Error(),
				}
			}
			mu.Lock()
			results[p.Type()] = result
			mu.Unlock()
		}(p)
	}

	wg.Wait()

	hc.mu.Lock()
	hc.results = results
	hc.mu.Unlock()

	return results
}

// CheckOne 检测单个 Provider
func (hc *HealthChecker) CheckOne(ctx context.Context, pt ProviderType) (*HealthCheckResult, error) {
	p, err := hc.registry.Get(pt)
	if err != nil {
		return nil, err
	}

	result, err := p.HealthCheck(ctx)
	if err != nil {
		return nil, err
	}

	hc.mu.Lock()
	hc.results[pt] = result
	hc.mu.Unlock()

	return result, nil
}

// GetCached 获取缓存的检测结果
func (hc *HealthChecker) GetCached(pt ProviderType) *HealthCheckResult {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.results[pt]
}

// GetHealthy 返回所有健康的 Provider
func (hc *HealthChecker) GetHealthy() []*HealthCheckResult {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	var healthy []*HealthCheckResult
	for _, r := range hc.results {
		if r.Status == HealthHealthy {
			healthy = append(healthy, r)
		}
	}
	return healthy
}

// Selector 根据策略自动选择合适的 Provider
type Selector struct {
	registry *Registry
	checker  *HealthChecker
}

// SelectionStrategy Provider 选择策略
type SelectionStrategy struct {
	PreferAvailable     bool `yaml:"prefer_available"`
	PreferLowCost       bool `yaml:"prefer_low_cost"`
	PreferFastResponse  bool `yaml:"prefer_fast_response"`
	PreferPatchAccuracy bool `yaml:"prefer_patch_accuracy"`
}

// DefaultSelectionStrategy 默认选择策略
var DefaultSelectionStrategy = SelectionStrategy{
	PreferAvailable:     true,
	PreferLowCost:       true,
	PreferFastResponse:  true,
	PreferPatchAccuracy: true,
}

// NewSelector 创建 Provider 选择器
func NewSelector(registry *Registry, checker *HealthChecker) *Selector {
	return &Selector{
		registry: registry,
		checker:  checker,
	}
}

// Select 根据策略选择最佳 Provider。
// 返回按优先级排序的 Provider 列表（第一个是最佳选择）。
func (s *Selector) Select(ctx context.Context, strategy SelectionStrategy) ([]Provider, error) {
	// 先确保健康检测数据是最新的
	allResults := s.checker.CheckAll(ctx)

	type scoredProvider struct {
		provider Provider
		result   *HealthCheckResult
		score    float64
	}

	var scored []scoredProvider
	for pt, result := range allResults {
		if result.Status == HealthUnhealthy {
			continue // 跳过不可用的 Provider
		}
		p, err := s.registry.Get(pt)
		if err != nil {
			continue
		}
		scored = append(scored, scoredProvider{
			provider: p,
			result:   result,
			score:    0,
		})
	}

	// 打分
	for i := range scored {
		score := 100.0

		if strategy.PreferAvailable && scored[i].result.Status == HealthHealthy {
			score += 50
		} else if scored[i].result.Status == HealthDegraded {
			score -= 30
		}

		if strategy.PreferLowCost {
			// 成本越低分数越高
			score -= scored[i].result.CostPer1K * 10000
		}

		if strategy.PreferFastResponse {
			// 延迟越低分数越高
			latencyScore := 100.0 - scored[i].result.Latency.Seconds()*10
			if latencyScore < 0 {
				latencyScore = 0
			}
			score += latencyScore
		}

		if strategy.PreferPatchAccuracy {
			for _, cap := range scored[i].result.Capabilities {
				if cap == CapabilityPatch {
					score += 30
				}
				if cap == CapabilityJSON {
					score += 20
				}
			}
		}

		// 成功率加权
		score += scored[i].result.RecentSuccess * 50

		scored[i].score = score
	}

	// 按分数降序排列
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if len(scored) == 0 {
		return nil, fmt.Errorf("no available providers")
	}

	providers := make([]Provider, len(scored))
	for i, sp := range scored {
		providers[i] = sp.provider
	}

	return providers, nil
}

// SelectByType 根据指定类型获取 Provider（带健康检查）
func (s *Selector) SelectByType(ctx context.Context, pt ProviderType) (Provider, error) {
	result, err := s.checker.CheckOne(ctx, pt)
	if err != nil {
		return nil, fmt.Errorf("health check failed for %q: %w", pt, err)
	}

	if result.Status == HealthUnhealthy {
		return nil, fmt.Errorf("provider %q is unhealthy: %s", pt, result.Error)
	}

	return s.registry.Get(pt)
}

// BenchmarkResult Provider 基准测试结果
type BenchmarkResult struct {
	Provider     ProviderType   `json:"provider"`
	Model        string         `json:"model"`
	LatencyAvg   time.Duration  `json:"latency_avg"`
	LatencyP50   time.Duration  `json:"latency_p50"`
	LatencyP95   time.Duration  `json:"latency_p95"`
	TokensPerSec float64        `json:"tokens_per_sec"`
	SuccessRate  float64        `json:"success_rate"`
	CostPerCall  float64        `json:"cost_per_call"`
	Samples      int            `json:"samples"`
}

// Benchmark 对 Provider 执行基准测试
func (s *Selector) Benchmark(ctx context.Context, pt ProviderType, samples int) (*BenchmarkResult, error) {
	p, err := s.registry.Get(pt)
	if err != nil {
		return nil, err
	}

	result := &BenchmarkResult{
		Provider: pt,
		Samples:  samples,
	}

	var latencies []time.Duration
	successCount := 0
	totalTokens := 0

	testReq := &GenerateRequest{
		Model:     "",
		MaxTokens: 50,
		Messages: []ChatMessage{
			{Role: "user", Content: "Say hello in exactly 5 words."},
		},
	}

	for i := 0; i < samples; i++ {
		resp, err := p.Generate(ctx, testReq)
		if err == nil {
			successCount++
			latencies = append(latencies, resp.Latency)
			totalTokens += resp.Usage.TotalTokens
			if result.Model == "" {
				result.Model = resp.Model
			}
		}
	}

	result.SuccessRate = float64(successCount) / float64(samples)

	if len(latencies) > 0 {
		var total time.Duration
		for _, l := range latencies {
			total += l
		}
		result.LatencyAvg = total / time.Duration(len(latencies))

		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})
		result.LatencyP50 = latencies[len(latencies)/2]
		result.LatencyP95 = latencies[int(float64(len(latencies))*0.95)]

		if result.LatencyAvg > 0 {
			result.TokensPerSec = float64(totalTokens) / result.LatencyAvg.Seconds()
		}
	}

	return result, nil
}
