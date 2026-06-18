package providers

import (
	"fmt"
	"sync"
)

// Registry 是 Provider 的注册中心，管理所有已注册的 Provider 实例。
// 通过注册中心实现可插拔架构：新增 Provider 只需 Register，无需修改核心代码。
type Registry struct {
	mu        sync.RWMutex
	providers map[ProviderType]Provider
	aliases   map[string]ProviderType // 别名映射，如 "codex" -> "openai_compatible"
}

// NewRegistry 创建新的注册中心
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[ProviderType]Provider),
		aliases:   make(map[string]ProviderType),
	}
}

// Register 注册一个 Provider。
// 如果同类型已存在，会被覆盖。
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Type()] = p
}

// RegisterAlias 注册别名，如 "codex" -> ProviderOpenAICompat
func (r *Registry) RegisterAlias(alias string, pt ProviderType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aliases[alias] = pt
}

// Get 根据类型获取 Provider
func (r *Registry) Get(pt ProviderType) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[pt]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", pt)
	}
	return p, nil
}

// GetByAlias 根据别名获取 Provider（如 "codex"、"deepseek"）
func (r *Registry) GetByAlias(alias string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	pt, ok := r.aliases[alias]
	if !ok {
		// 尝试直接按类型查找
		pt = ProviderType(alias)
	}
	p, ok := r.providers[pt]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", alias)
	}
	return p, nil
}

// List 返回所有已注册的 Provider 类型
func (r *Registry) List() []ProviderType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]ProviderType, 0, len(r.providers))
	for t := range r.providers {
		types = append(types, t)
	}
	return types
}

// ListAll 返回所有已注册的 Provider 实例
func (r *Registry) ListAll() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	providers := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	return providers
}

// Unregister 注销一个 Provider
func (r *Registry) Unregister(pt ProviderType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, pt)
}

// DefaultRegistry 全局默认注册中心
var DefaultRegistry = NewRegistry()

// Register 向全局注册中心注册
func Register(p Provider) {
	DefaultRegistry.Register(p)
}

// GetFromRegistry 从全局注册中心获取
func GetFromRegistry(pt ProviderType) (Provider, error) {
	return DefaultRegistry.Get(pt)
}
