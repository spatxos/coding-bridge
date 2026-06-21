package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/coding-bridge/internal/config"
	"github.com/coding-bridge/internal/providers"
)

type Server struct {
	projectRoot string
	loader      *config.Loader
	addr        string
	port        int
}

func NewServer(projectRoot string, port int) *Server {
	return &Server{
		projectRoot: projectRoot,
		loader:      config.NewLoader(projectRoot),
		port:        port,
	}
}

func (s *Server) Start() (string, error) {
	addr, err := s.findPort()
	if err != nil {
		return "", fmt.Errorf("find available port: %w", err)
	}
	s.addr = addr

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/config", s.handleConfigAPI)
	mux.HandleFunc("/api/context", s.handleContextAPI)
	mux.HandleFunc("/api/config/save", s.handleConfigSave)
	mux.HandleFunc("/api/config/full-save", s.handleFullConfigSave)
	mux.HandleFunc("/api/config/check", s.handleProviderCheck)
	mux.HandleFunc("/api/models", s.handleModels)

	go func() { http.ListenAndServe(addr, mux) }()

	return "http://" + addr, nil
}

func (s *Server) handleContextAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"project_root": s.projectRoot,
		"config_path":  s.loader.ConfigPath(),
	})
}

func (s *Server) findPort() (string, error) {
	port := s.port
	for i := 0; i < 10; i++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			return addr, nil
		}
		port++
	}
	return "", fmt.Errorf("no available port in range %d-%d", s.port, s.port+9)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(configPageHTML))
}

func (s *Server) handleConfigAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cfg, err := s.loader.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	masked := *cfg
	for name, pc := range masked.Providers.Configs {
		if pc.APIKey != "" {
			pc.APIKey = config.MaskAPIKey(pc.APIKey)
			masked.Providers.Configs[name] = pc
		}
	}
	json.NewEncoder(w).Encode(masked)
}

type saveRequest struct {
	DeepSeekKey        string   `json:"deepseek_key"`
	DeepSeekModels     []string `json:"deepseek_models"`
	DeepSeekEnabled    bool     `json:"deepseek_enabled"`
	DeepSeekBaseURL    string   `json:"deepseek_base_url"`
	DeepSeekTimeout    int      `json:"deepseek_timeout"`
	DeepSeekMaxRetry   int      `json:"deepseek_max_retry"`
	DefaultExecutor    string   `json:"default_executor"`
	SelectionMode      string   `json:"selection_mode"`
	FallbackOrder      string   `json:"fallback_order"`
	AllowedCommands    string   `json:"allowed_commands"`
	ForbiddenCmds      string   `json:"forbidden_commands"`
	CmdTimeout         int      `json:"cmd_timeout"`
	ReqTimeout         int      `json:"req_timeout"`
	PatchMaxTokens     int      `json:"patch_max_tokens"`
	Temperature        float64  `json:"temperature"`
	MaxRepairAttempts  int      `json:"max_repair_attempts"`
	EnforceTaskBudgets bool     `json:"enforce_task_budgets"`
	MaxTaskFiles       int      `json:"max_task_files"`
	MaxContextBytes    int      `json:"max_context_bytes"`
	MaxPatchLines      int      `json:"max_patch_lines"`
	CLIEnabled         bool     `json:"cli_enabled"`
	DefaultCLI         bool     `json:"default_cli"`
	CodexFallback      bool     `json:"codex_fallback"`
	SharingApproved    bool     `json:"sharing_approved"`
	TokenAccounting    bool     `json:"token_accounting"`
	BaselineRatio      float64  `json:"baseline_ratio"`
	ReportMaxHistory   int      `json:"report_max_history"`
}

func (s *Server) handleConfigSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	var req saveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	existingCfg, _ := s.loader.Load()
	existingDeepSeekKey := ""
	if existingCfg != nil {
		if existing, ok := existingCfg.Providers.Configs["deepseek"]; ok {
			existingDeepSeekKey = existing.APIKey
		}
	}

	cfg := existingCfg
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if cfg.Providers.Configs == nil {
		cfg.Providers.Configs = make(map[string]config.ProviderItemConfig)
	}
	if strings.TrimSpace(req.DefaultExecutor) != "" {
		cfg.Providers.DefaultExecutor = strings.TrimSpace(req.DefaultExecutor)
	}
	if req.SelectionMode == "auto" || req.SelectionMode == "manual" {
		cfg.ProviderSelection.Mode = req.SelectionMode
	}
	if strings.TrimSpace(req.FallbackOrder) != "" {
		cfg.ProviderSelection.FallbackOrder = parseCommaOrLines(req.FallbackOrder)
	}

	if req.DeepSeekEnabled {
		dsKey := chooseStoredAPIKey(req.DeepSeekKey, existingDeepSeekKey, "DEEPSEEK_API_KEY")
		models := req.DeepSeekModels
		if len(models) == 0 {
			models = []string{"deepseek-chat"}
		}
		cfg.Providers.Configs["deepseek"] = config.ProviderItemConfig{
			Type:     "deepseek",
			BaseURL:  valueOrDefault(req.DeepSeekBaseURL, "https://api.deepseek.com"),
			APIKey:   dsKey,
			Models:   models,
			Timeout:  positiveOrDefault(req.DeepSeekTimeout, 120),
			MaxRetry: nonNegativeOrDefault(req.DeepSeekMaxRetry, 2),
			Enabled:  true,
		}
	} else if existing, ok := cfg.Providers.Configs["deepseek"]; ok {
		existing.Enabled = false
		cfg.Providers.Configs["deepseek"] = existing
	}

	// 命令白名单
	if req.AllowedCommands != "" {
		cfg.Commands.Allowed = parseLines(req.AllowedCommands)
	}
	// 命令黑名单
	if req.ForbiddenCmds != "" {
		cfg.Commands.Forbidden = parseLines(req.ForbiddenCmds)
	}

	// 超时
	if req.CmdTimeout > 0 {
		cfg.Timeouts.Command = req.CmdTimeout
	}
	if req.ReqTimeout > 0 {
		cfg.Timeouts.ProviderRequest = req.ReqTimeout
	}
	if req.PatchMaxTokens > 0 {
		cfg.Execution.PatchMaxTokens = req.PatchMaxTokens
	}
	if req.Temperature >= 0 && req.Temperature <= 2 {
		cfg.Execution.Temperature = req.Temperature
	}
	if req.MaxRepairAttempts >= 0 && req.MaxRepairAttempts <= 5 {
		cfg.Execution.MaxRepairAttempts = req.MaxRepairAttempts
	}
	cfg.Execution.EnforceTaskBudgets = req.EnforceTaskBudgets
	if req.MaxTaskFiles > 0 {
		cfg.Execution.MaxTaskFiles = req.MaxTaskFiles
	}
	if req.MaxContextBytes >= 1024 {
		cfg.Execution.MaxContextBytes = req.MaxContextBytes
	}
	if req.MaxPatchLines > 0 {
		cfg.Execution.MaxPatchLines = req.MaxPatchLines
	}
	cfg.Codex.CLIEnabled = req.CLIEnabled
	cfg.Codex.DefaultCLIForCodingTasks = req.DefaultCLI
	cfg.Codex.FallbackToCodexOnUnavailable = req.CodexFallback
	cfg.Codex.ExternalCodeSharingApproved = req.SharingApproved
	cfg.TokenAccounting.Enabled = req.TokenAccounting
	if req.BaselineRatio >= 1 {
		cfg.TokenAccounting.DirectCodexBaselineRatio = req.BaselineRatio
	}
	if req.ReportMaxHistory > 0 {
		cfg.Report.MaxHistory = req.ReportMaxHistory
	}

	// 自动检测项目类型
	if pt := detectProjectTypeWeb(s.projectRoot); pt != "" {
		if len(cfg.Commands.Allowed) == 0 {
			cfg.Commands.Allowed = suggestedCommandsWeb(pt)
		}
	}
	if errs := cfg.Validate(); len(errs) > 0 {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": fmt.Sprintf("invalid config: %v", errs)})
		return
	}

	if err := s.loader.Save(cfg); err != nil {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err := config.WriteCodexPolicy(s.projectRoot, cfg); err != nil {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": cfg.Version})
}

func (s *Server) handleFullConfigSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var submitted config.AppConfig
	if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	existing, _ := s.loader.Load()
	if existing != nil {
		for name, provider := range submitted.Providers.Configs {
			old, ok := existing.Providers.Configs[name]
			if ok && (provider.APIKey == "" || strings.Contains(provider.APIKey, "*")) {
				provider.APIKey = old.APIKey
				submitted.Providers.Configs[name] = provider
			}
		}
	}
	if errs := submitted.Validate(); len(errs) > 0 {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": fmt.Sprintf("invalid config: %v", errs)})
		return
	}
	if err := s.loader.Save(&submitted); err != nil {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err := config.WriteCodexPolicy(s.projectRoot, &submitted); err != nil {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": submitted.Version})
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func positiveOrDefault(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func nonNegativeOrDefault(value, fallback int) int {
	if value >= 0 {
		return value
	}
	return fallback
}

func parseCommaOrLines(s string) []string {
	return parseLines(strings.ReplaceAll(s, ",", "\n"))
}

func chooseStoredAPIKey(submitted, existing, envName string) string {
	submitted = strings.TrimSpace(submitted)
	if submitted != "" && !strings.Contains(submitted, "*") {
		return submitted
	}
	if strings.TrimSpace(existing) != "" {
		return existing
	}
	return "${" + envName + "}"
}

func parseLines(s string) []string {
	var result []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	apiKey := r.URL.Query().Get("key")
	baseURL := r.URL.Query().Get("base")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	if apiKey == "" || strings.HasPrefix(apiKey, "****") {
		apiKey = os.Getenv("DEEPSEEK_API_KEY")
	}

	// 创建临时 provider 拉取模型列表
	tmp := providers.NewDeepSeekProviderWithConfig(providers.ProviderConfig{
		Type:    providers.ProviderDeepSeek,
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   "deepseek-chat",
		Timeout: 10 * time.Second,
	})

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	models, err := tmp.ListModels(ctx)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"ok": true, "models": models})
}

func (s *Server) handleProviderCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cfg, _ := s.loader.Load()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	registry := providers.NewRegistry()
	registerFromConfig(registry, cfg)

	checker := providers.NewHealthChecker(registry)
	results := checker.CheckAll(r.Context())

	type checkEntry struct {
		Provider string `json:"provider"`
		Status   string `json:"status"`
		Error    string `json:"error,omitempty"`
		Latency  string `json:"latency"`
	}
	var entries []checkEntry
	for pt, result := range results {
		entry := checkEntry{
			Provider: string(pt),
			Status:   string(result.Status),
			Latency:  result.Latency.Round(0).String(),
		}
		if result.Error != "" {
			entry.Error = result.Error
		}
		entries = append(entries, entry)
	}
	json.NewEncoder(w).Encode(map[string]any{"results": entries})
}

func registerFromConfig(registry *providers.Registry, cfg *config.AppConfig) {
	for _, pc := range cfg.Providers.Configs {
		if !pc.Enabled {
			continue
		}
		apiKey := pc.APIKey
		if strings.HasPrefix(apiKey, "${") {
			envName := strings.TrimSuffix(strings.TrimPrefix(apiKey, "${"), "}")
			apiKey = os.Getenv(envName)
		}
		model := pc.Model
		if model == "" && len(pc.Models) > 0 {
			model = pc.Models[0]
		}

		switch pc.Type {
		case "deepseek":
			registry.Register(providers.NewDeepSeekProviderWithConfig(providers.ProviderConfig{
				Type:     providers.ProviderDeepSeek,
				BaseURL:  pc.BaseURL,
				APIKey:   apiKey,
				Model:    model,
				MaxRetry: pc.MaxRetry,
			}))
		}
	}
}

func OpenBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}

func detectProjectTypeWeb(root string) string {
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		switch e.Name() {
		case "go.mod":
			return "Go"
		case "package.json":
			return "Node.js"
		case "Cargo.toml":
			return "Rust"
		case "pyproject.toml", "setup.py":
			return "Python"
		}
		if strings.HasSuffix(e.Name(), ".csproj") {
			return ".NET"
		}
	}
	return ""
}

func suggestedCommandsWeb(pt string) []string {
	switch pt {
	case "Go":
		return []string{"go build ./...", "go test ./...", "go vet ./..."}
	case "Node.js":
		return []string{"npm run build", "npm test"}
	case "Rust":
		return []string{"cargo build", "cargo test"}
	case "Python":
		return []string{"pytest"}
	case ".NET":
		return []string{"dotnet build", "dotnet test"}
	}
	return nil
}

const configPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>coding-bridge 配置</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#0d1117;color:#c9d1d9;min-height:100vh}
.container{max-width:780px;margin:0 auto;padding:40px 20px}
h1{font-size:24px;color:#58a6ff;margin-bottom:8px}
.subtitle{color:#8b949e;margin-bottom:32px;font-size:14px}
.card{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:24px;margin-bottom:16px}
.card-title{font-size:16px;font-weight:600;color:#f0f6fc;margin-bottom:16px;display:flex;align-items:center;gap:8px}
.card-title .icon{font-size:20px}
.form-group{margin-bottom:14px}
.form-group:last-child{margin-bottom:0}
label{display:block;font-size:13px;color:#8b949e;margin-bottom:6px}
input[type="text"],input[type="password"],input[type="number"],textarea{width:100%;background:#0d1117;border:1px solid #30363d;border-radius:6px;padding:8px 12px;color:#c9d1d9;font-size:13px;outline:none;font-family:inherit}
input:focus,textarea:focus{border-color:#58a6ff;box-shadow:0 0 0 2px rgba(88,166,255,0.15)}
textarea{resize:vertical;min-height:80px}
.row{display:flex;gap:12px}
.row>*{flex:1}
.toggle{display:flex;align-items:center;gap:8px;cursor:pointer;margin-bottom:14px}
.toggle input{display:none}
.toggle .switch{width:40px;height:22px;background:#30363d;border-radius:11px;position:relative;transition:background .2s;flex-shrink:0}
.toggle .switch::after{content:'';position:absolute;top:2px;left:2px;width:18px;height:18px;background:#c9d1d9;border-radius:50%;transition:transform .2s}
.toggle input:checked+.switch{background:#238636}
.toggle input:checked+.switch::after{transform:translateX(18px)}
.btn{display:inline-flex;align-items:center;gap:6px;padding:10px 20px;border-radius:6px;font-size:14px;font-weight:500;cursor:pointer;border:none;transition:opacity .2s}
.btn:disabled{opacity:0.5;cursor:not-allowed}
.btn-primary{background:#238636;color:#fff}
.btn-primary:hover:not(:disabled){background:#2ea043}
.btn-secondary{background:#21262d;color:#c9d1d9;border:1px solid #30363d}
.btn-secondary:hover:not(:disabled){background:#30363d}
.btn-sm{padding:6px 12px;font-size:12px}
.actions{display:flex;gap:12px;margin-top:24px;flex-wrap:wrap}
.status-bar{margin-top:16px;padding:12px;border-radius:6px;font-size:13px;display:none}
.status-bar.success{background:rgba(35,134,54,0.15);border:1px solid rgba(35,134,54,0.4);color:#3fb950;display:block}
.status-bar.error{background:rgba(218,54,51,0.15);border:1px solid rgba(218,54,51,0.4);color:#f85149;display:block}
.status-bar.info{background:rgba(88,166,255,0.1);border:1px solid rgba(88,166,255,0.3);color:#58a6ff;display:block}
.check-result{margin-top:12px;font-size:13px}
.check-result .provider{margin-bottom:8px;padding:8px;background:#0d1117;border-radius:4px;display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.check-result .provider .dot{width:8px;height:8px;border-radius:50%;flex-shrink:0}
.check-result .provider .dot.healthy{background:#3fb950}
.check-result .provider .dot.degraded{background:#d29922}
.check-result .provider .dot.unhealthy{background:#f85149}
.env-hint{font-size:12px;color:#8b949e;margin-top:4px}
.model-checkboxes{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:8px}
.model-checkboxes label{display:flex;align-items:center;gap:6px;background:#0d1117;border:1px solid #30363d;border-radius:4px;padding:6px 10px;cursor:pointer;font-size:13px;color:#c9d1d9;margin-bottom:0}
.model-checkboxes label:hover{border-color:#58a6ff}
.model-checkboxes input[type="checkbox"]{accent-color:#238636;width:14px;height:14px}
.model-loading{font-size:12px;color:#8b949e;margin-top:4px}
.badge{font-size:11px;padding:2px 6px;border-radius:3px;margin-left:4px}
.badge-new{background:rgba(88,166,255,0.15);color:#58a6ff}
.collapsible-header{cursor:pointer;user-select:none}
.collapsible-header:hover{color:#58a6ff}
</style>
</head>
<body>
<div class="container">
<h1>⚙️ coding-bridge 配置</h1>
<p class="subtitle">Executor 模型配置（生成 patch）。Controller（Codex）在你的 AI 工具中配置，不在此处。</p>
<div class="card" style="padding:14px">
  <div style="font-size:12px;color:#8b949e">Active project root</div>
  <div id="active-project-root" style="font-family:Consolas,monospace;margin:4px 0 10px"></div>
  <div style="font-size:12px;color:#8b949e">Active config file</div>
  <div id="active-config-path" style="font-family:Consolas,monospace;margin-top:4px;color:#58a6ff"></div>
</div>

<!-- DeepSeek -->
<div class="card">
<div class="card-title"><span class="icon">🚀</span> DeepSeek Executor</div>
<label class="toggle">
  <input type="checkbox" id="ds-enabled" checked onchange="toggleSection('ds')">
  <div class="switch"></div> 启用
</label>
<div id="ds-section">
  <div class="row">
    <div class="form-group"><label>Base URL</label><input type="text" id="ds-base-url" value="https://api.deepseek.com"></div>
    <div class="form-group"><label>Default Executor</label><input type="text" id="default-executor" value="deepseek"></div>
  </div>
  <div class="row">
    <div class="form-group"><label>Provider timeout (seconds)</label><input type="number" id="ds-timeout" value="120" min="1" max="3600"></div>
    <div class="form-group"><label>Provider retries</label><input type="number" id="ds-max-retry" value="2" min="0" max="10"></div>
  </div>
  <div class="form-group">
    <label>API Key</label>
    <div style="display:flex;gap:8px">
    <input type="password" id="ds-key" placeholder="sk-xxx（留空从环境变量读取）" style="flex:1">
    <button class="btn btn-secondary btn-sm" onclick="loadModels()" id="btn-load-models" style="flex-shrink:0">📡 加载模型列表</button>
    </div>
    <div class="env-hint">💡 也可设环境变量 DEEPSEEK_API_KEY，无需在此填写</div>
  </div>
  <div class="form-group">
    <label>模型 <span style="color:#f85149">*</span> <span style="font-weight:400;color:#8b949e">（最少选一个，输入 API Key 后点「加载模型列表」获取最新模型）</span></label>
    <div class="model-checkboxes" id="model-list">
      <label><input type="checkbox" value="deepseek-chat" checked onchange="checkModelMin()"><span>deepseek-chat</span></label>
      <label><input type="checkbox" value="deepseek-reasoner" onchange="checkModelMin()"><span>deepseek-reasoner</span></label>
    </div>
    <div class="model-loading" id="model-msg"></div>
    <div class="form-group" style="margin-top:8px">
      <label>或手动输入模型名</label>
      <input type="text" id="ds-model-manual" placeholder="多个模型用逗号分隔，如 deepseek-chat,deepseek-reasoner">
      <div class="env-hint">手动输入的模型会与上方选中的模型合并</div>
    </div>
  </div>
</div>
</div>

<div class="card">
<div class="card-title"><span class="icon">⚙️</span> Execution</div>
<div class="row">
  <div class="form-group"><label>Patch output token budget</label><input type="number" id="patch-max-tokens" value="16384" min="256" max="131072"></div>
  <div class="form-group"><label>Temperature</label><input type="number" id="temperature" value="0.1" min="0" max="2" step="0.1"></div>
</div>
<div class="form-group"><label>Maximum repair attempts</label><input type="number" id="max-repair-attempts" value="2" min="0" max="5"></div>
<label class="toggle"><input type="checkbox" id="enforce-task-budgets" checked><div class="switch"></div> Enforce task size budgets before spending Executor tokens</label>
<div class="row">
  <div class="form-group"><label>Maximum allowed files</label><input type="number" id="max-task-files" value="3" min="1" max="100"></div>
  <div class="form-group"><label>Maximum context bytes</label><input type="number" id="max-context-bytes" value="49152" min="1024"></div>
  <div class="form-group"><label>Maximum patch changed lines</label><input type="number" id="max-patch-lines" value="200" min="1"></div>
</div>
<div class="row">
  <div class="form-group"><label>Provider selection mode</label><input type="text" id="selection-mode" value="auto"></div>
  <div class="form-group"><label>Fallback order</label><input type="text" id="fallback-order" value="deepseek,openai"></div>
</div>
</div>

<div class="card">
<div class="card-title"><span class="icon">🤖</span> Codex workflow</div>
<label class="toggle"><input type="checkbox" id="cli-enabled" checked><div class="switch"></div> Enable coding-bridge</label>
<label class="toggle"><input type="checkbox" id="default-cli" checked><div class="switch"></div> Use CLI by default for coding tasks</label>
<label class="toggle"><input type="checkbox" id="codex-fallback" checked><div class="switch"></div> Fall back to Codex when all Executors are unavailable</label>
<label class="toggle"><input type="checkbox" id="sharing-approved"><div class="switch"></div> Persist external allowlisted-code sharing approval</label>
<div class="env-hint">Changes apply to the next Codex coding task.</div>
</div>

<div class="card">
<div class="card-title"><span class="icon">📊</span> Token accounting</div>
<label class="toggle"><input type="checkbox" id="token-accounting" checked><div class="switch"></div> Show token savings estimates</label>
<div class="row">
  <div class="form-group"><label>Direct Codex baseline ratio</label><input type="number" id="baseline-ratio" value="3" min="1" max="20" step="0.1"></div>
  <div class="form-group"><label>Report history limit</label><input type="number" id="report-max-history" value="100" min="1" max="10000"></div>
</div>
<div class="env-hint">Executor tokens are exact. Net savings need controller.observed_tokens in task JSON.</div>
</div>

<!-- 命令白名单 -->
<div class="card">
<div class="card-title collapsible-header" onclick="document.getElementById('cmd-section').style.display=document.getElementById('cmd-section').style.display==='none'?'block':'none'">
<span class="icon">💻</span> 命令白名单 <span style="font-size:11px;color:#8b949e;font-weight:400">▼ 点击展开</span>
</div>
<div id="cmd-section" style="display:none">
  <div class="form-group">
    <label>允许的命令（每行一个）</label>
    <textarea id="allowed-cmds" placeholder="go build ./...&#10;go test ./...&#10;dotnet build&#10;dotnet test&#10;npm test&#10;pytest"></textarea>
  </div>
  <div class="form-group">
    <label>禁止的命令（每行一个）</label>
    <textarea id="forbidden-cmds" placeholder="rm -rf&#10;git reset --hard&#10;git clean -fdx&#10;ssh&#10;curl&#10;wget"></textarea>
  </div>
</div>
</div>

<!-- 超时设置 -->
<div class="card">
<div class="card-title collapsible-header" onclick="document.getElementById('timeout-section').style.display=document.getElementById('timeout-section').style.display==='none'?'block':'none'">
<span class="icon">⏱️</span> 超时设置 <span style="font-size:11px;color:#8b949e;font-weight:400">▼ 点击展开</span>
</div>
<div id="timeout-section" style="display:none">
  <div class="row">
    <div class="form-group">
      <label>命令超时（秒）</label>
      <input type="number" id="cmd-timeout" value="300" min="10" max="3600">
    </div>
    <div class="form-group">
      <label>API 请求超时（秒）</label>
      <input type="number" id="req-timeout" value="120" min="10" max="600">
    </div>
  </div>
</div>
</div>

<div class="card">
<div class="card-title collapsible-header" onclick="document.getElementById('advanced-section').style.display=document.getElementById('advanced-section').style.display==='none'?'block':'none'">
<span class="icon">🧩</span> Advanced full configuration <span style="font-size:11px;color:#8b949e;font-weight:400">▼ click to expand</span>
</div>
<div id="advanced-section" style="display:none">
  <div class="form-group">
    <label>Complete configuration JSON (API keys are masked and preserved automatically)</label>
    <textarea id="advanced-config" style="min-height:360px;font-family:Consolas,monospace"></textarea>
  </div>
  <button class="btn btn-secondary" onclick="saveAdvancedConfig()">Save advanced configuration</button>
</div>
</div>

<div class="actions">
  <button class="btn btn-primary" onclick="saveConfig()">💾 保存配置</button>
  <button class="btn btn-secondary" onclick="checkProviders()">🔍 检测连通性</button>
</div>

<div id="status" class="status-bar"></div>
<div id="check-results" class="check-result"></div>
</div>

<script>
function toggleSection(id){
  document.getElementById(id+'-section').style.display=document.getElementById(id+'-enabled').checked?'block':'none';
}

function checkModelMin(){
  const checks=document.querySelectorAll('#model-list input[type="checkbox"]:checked');
  if(checks.length===0){
    document.getElementById('model-msg').innerHTML='<span style="color:#f85149">⚠️ 请至少选择一个模型</span>';
  }else{
    document.getElementById('model-msg').innerHTML='';
  }
}

function getSelectedModels(){
  const checks=document.querySelectorAll('#model-list input[type="checkbox"]:checked');
  const models=Array.from(checks).map(c=>c.value);
  const manual=document.getElementById('ds-model-manual').value.trim();
  if(manual){
    manual.split(',').forEach(m=>{m=m.trim();if(m&&!models.includes(m))models.push(m)});
  }
  return models;
}

async function loadModels(){
  const key=document.getElementById('ds-key').value.trim();
  if(!key){showStatus('请先输入 API Key','error');return}
  const btn=document.getElementById('btn-load-models');
  const msg=document.getElementById('model-msg');
  btn.disabled=true;btn.textContent='⏳ 加载中...';
  msg.innerHTML='<span style="color:#8b949e">⏳ 正在获取模型列表...</span>';
  try{
    const r=await fetch('/api/models?key='+encodeURIComponent(key));
    const d=await r.json();
    if(!d.ok){msg.innerHTML='<span style="color:#f85149">❌ '+d.error+'</span>';return}
    const list=document.getElementById('model-list');
    const existingModels=new Set(getSelectedModels());
    list.innerHTML='';
    d.models.forEach(m=>{
      const id='m-'+m.replace(/[^a-z0-9]/gi,'-');
      list.innerHTML+='<label><input type="checkbox" value="'+m+'" id="'+id+'" onchange="checkModelMin()" '+(existingModels.has(m)?'checked':'')+'><span>'+m+'</span></label>';
    });
    msg.innerHTML='<span style="color:#3fb950">✅ 已加载 '+d.models.length+' 个模型</span>';
    checkModelMin();
  }catch(e){
    msg.innerHTML='<span style="color:#f85149">❌ 网络错误: '+e.message+'</span>';
  }finally{
    btn.disabled=false;btn.textContent='📡 加载模型列表';
  }
}

async function loadConfig(){
  try{
    const contextResponse=await fetch('/api/context');
    const context=await contextResponse.json();
    document.getElementById('active-project-root').textContent=context.project_root||'';
    document.getElementById('active-config-path').textContent=context.config_path||'';
    const r=await fetch('/api/config');
    const cfg=await r.json();
    document.getElementById('advanced-config').value=JSON.stringify(cfg,null,2);
    if(cfg.providers&&cfg.providers.configs){
      const ds=cfg.providers.configs.deepseek;
      document.getElementById('ds-enabled').checked=!!(ds&&ds.enabled);
      document.getElementById('ds-section').style.display=(ds&&ds.enabled)?'block':'none';
      if(ds&&ds.enabled){
        document.getElementById('ds-enabled').checked=true;
        document.getElementById('ds-section').style.display='block';
        if(ds.models&&ds.models.length>0){
          document.querySelectorAll('#model-list input').forEach(cb=>{
            cb.checked=ds.models.includes(cb.value);
          });
        }else if(ds.model){
          document.getElementById('ds-model-manual').value=ds.model;
        }
        if(ds.base_url)document.getElementById('ds-base-url').value=ds.base_url;
        if(ds.timeout_seconds)document.getElementById('ds-timeout').value=ds.timeout_seconds;
        if(ds.max_retry!==undefined)document.getElementById('ds-max-retry').value=ds.max_retry;
      }
      if(cfg.providers.default_executor)document.getElementById('default-executor').value=cfg.providers.default_executor;
    }
    if(cfg.provider_selection){
      if(cfg.provider_selection.mode)document.getElementById('selection-mode').value=cfg.provider_selection.mode;
      if(cfg.provider_selection.fallback_order)document.getElementById('fallback-order').value=cfg.provider_selection.fallback_order.join(',');
    }
    // 命令
    if(cfg.commands){
      if(cfg.commands.allowed)document.getElementById('allowed-cmds').value=cfg.commands.allowed.join('\n');
      if(cfg.commands.forbidden)document.getElementById('forbidden-cmds').value=cfg.commands.forbidden.join('\n');
    }
    // 超时
    if(cfg.timeouts){
      if(cfg.timeouts.command_seconds)document.getElementById('cmd-timeout').value=cfg.timeouts.command_seconds;
      if(cfg.timeouts.provider_request_seconds)document.getElementById('req-timeout').value=cfg.timeouts.provider_request_seconds;
    }
    if(cfg.execution){
      if(cfg.execution.patch_max_tokens)document.getElementById('patch-max-tokens').value=cfg.execution.patch_max_tokens;
      if(cfg.execution.temperature!==undefined)document.getElementById('temperature').value=cfg.execution.temperature;
      if(cfg.execution.max_repair_attempts!==undefined)document.getElementById('max-repair-attempts').value=cfg.execution.max_repair_attempts;
      document.getElementById('enforce-task-budgets').checked=!!cfg.execution.enforce_task_budgets;
      if(cfg.execution.max_task_files)document.getElementById('max-task-files').value=cfg.execution.max_task_files;
      if(cfg.execution.max_context_bytes)document.getElementById('max-context-bytes').value=cfg.execution.max_context_bytes;
      if(cfg.execution.max_patch_lines)document.getElementById('max-patch-lines').value=cfg.execution.max_patch_lines;
    }
    if(cfg.codex){
      document.getElementById('cli-enabled').checked=!!cfg.codex.cli_enabled;
      document.getElementById('default-cli').checked=!!cfg.codex.default_cli_for_coding_tasks;
      document.getElementById('codex-fallback').checked=!!cfg.codex.fallback_to_codex_on_unavailable;
      document.getElementById('sharing-approved').checked=!!cfg.codex.external_code_sharing_approved;
    }
    if(cfg.token_accounting){
      document.getElementById('token-accounting').checked=!!cfg.token_accounting.enabled;
      if(cfg.token_accounting.direct_codex_baseline_ratio)document.getElementById('baseline-ratio').value=cfg.token_accounting.direct_codex_baseline_ratio;
    }
    if(cfg.report&&cfg.report.max_history)document.getElementById('report-max-history').value=cfg.report.max_history;
  }catch(e){}
}

function showStatus(msg,type){
  const s=document.getElementById('status');
  s.textContent=msg;s.className='status-bar '+type;
  setTimeout(()=>{s.style.display='none';s.className='status-bar'},5000);
}

async function saveConfig(){
  const dsEnabled=document.getElementById('ds-enabled').checked;
  const models=getSelectedModels();
  if(dsEnabled&&models.length===0){showStatus('请至少选择一个模型','error');return}

  const body={
    deepseek_key:document.getElementById('ds-key').value,
    deepseek_models:models,
    deepseek_enabled:dsEnabled,
    deepseek_base_url:document.getElementById('ds-base-url').value,
    deepseek_timeout:parseInt(document.getElementById('ds-timeout').value)||120,
    deepseek_max_retry:parseInt(document.getElementById('ds-max-retry').value)||0,
    default_executor:document.getElementById('default-executor').value,
    selection_mode:document.getElementById('selection-mode').value,
    fallback_order:document.getElementById('fallback-order').value,
    allowed_commands:document.getElementById('allowed-cmds').value,
    forbidden_commands:document.getElementById('forbidden-cmds').value,
    cmd_timeout:parseInt(document.getElementById('cmd-timeout').value)||300,
    req_timeout:parseInt(document.getElementById('req-timeout').value)||120,
    patch_max_tokens:parseInt(document.getElementById('patch-max-tokens').value)||16384,
    temperature:parseFloat(document.getElementById('temperature').value),
    max_repair_attempts:parseInt(document.getElementById('max-repair-attempts').value)||0,
    enforce_task_budgets:document.getElementById('enforce-task-budgets').checked,
    max_task_files:parseInt(document.getElementById('max-task-files').value)||3,
    max_context_bytes:parseInt(document.getElementById('max-context-bytes').value)||49152,
    max_patch_lines:parseInt(document.getElementById('max-patch-lines').value)||200,
    cli_enabled:document.getElementById('cli-enabled').checked,
    default_cli:document.getElementById('default-cli').checked,
    codex_fallback:document.getElementById('codex-fallback').checked,
    sharing_approved:document.getElementById('sharing-approved').checked,
    token_accounting:document.getElementById('token-accounting').checked,
    baseline_ratio:parseFloat(document.getElementById('baseline-ratio').value)||3,
    report_max_history:parseInt(document.getElementById('report-max-history').value)||100
  };
  try{
    const r=await fetch('/api/config/save',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
    const d=await r.json();
    if(d.ok)showStatus('✅ 配置已保存！版本 '+d.version,'success');
    else showStatus('❌ 保存失败: '+d.error,'error');
  }catch(e){showStatus('❌ 请求失败: '+e.message,'error')}
}

async function saveAdvancedConfig(){
  try{
    const cfg=JSON.parse(document.getElementById('advanced-config').value);
    const r=await fetch('/api/config/full-save',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(cfg)});
    const d=await r.json();
    if(d.ok)showStatus('Advanced configuration saved. New tasks use version '+d.version+'.','success');
    else showStatus('Advanced save failed: '+d.error,'error');
  }catch(e){showStatus('Invalid JSON or request failed: '+e.message,'error')}
}

async function checkProviders(){
  const results=document.getElementById('check-results');
  results.innerHTML='<div style="color:#8b949e;font-size:13px">⏳ 检测中...</div>';
  try{
    const r=await fetch('/api/config/check');
    const d=await r.json();
    if(!d.results||d.results.length===0){results.innerHTML='<div style="color:#8b949e;font-size:13px">请先保存配置再检测</div>';return}
    let html='';
    for(const p of d.results){
      const cls=p.status==='healthy'?'healthy':p.status==='degraded'?'degraded':'unhealthy';
      const icon=p.status==='healthy'?'✅':p.status==='degraded'?'⚠️':'❌';
      html+='<div class="provider"><span class="dot '+cls+'"></span><strong>'+p.provider+'</strong> — '+icon+' '+p.status+' ('+p.latency+')';
      if(p.error)html+='<br><span style="color:#f85149;font-size:12px">'+p.error+'</span>';
      html+='</div>';
    }
    results.innerHTML=html;
  }catch(e){results.innerHTML='<div style="color:#f85149;font-size:13px">检测失败: '+e.message+'</div>'}
}

loadConfig();
</script>
</body>
</html>`
