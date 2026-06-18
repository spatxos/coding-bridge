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
	mux.HandleFunc("/api/config/save", s.handleConfigSave)
	mux.HandleFunc("/api/config/check", s.handleProviderCheck)
	mux.HandleFunc("/api/models", s.handleModels)

	go func() { http.ListenAndServe(addr, mux) }()

	return "http://" + addr, nil
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
	DeepSeekKey     string   `json:"deepseek_key"`
	DeepSeekModels  []string `json:"deepseek_models"`
	DeepSeekEnabled bool     `json:"deepseek_enabled"`
	AllowedCommands string   `json:"allowed_commands"`
	ForbiddenCmds   string   `json:"forbidden_commands"`
	CmdTimeout      int      `json:"cmd_timeout"`
	ReqTimeout      int      `json:"req_timeout"`
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

	cfg := config.DefaultConfig()
	cfg.Providers.Configs = make(map[string]config.ProviderItemConfig)
	cfg.Providers.DefaultExecutor = "deepseek"

	if req.DeepSeekEnabled {
		dsKey := req.DeepSeekKey
		if dsKey == "" || strings.HasPrefix(dsKey, "****") {
			dsKey = "${DEEPSEEK_API_KEY}"
		}
		models := req.DeepSeekModels
		if len(models) == 0 {
			models = []string{"deepseek-chat"}
		}
		cfg.Providers.Configs["deepseek"] = config.ProviderItemConfig{
			Type:     "deepseek",
			BaseURL:  "https://api.deepseek.com",
			APIKey:   dsKey,
			Models:   models,
			Timeout:  120,
			MaxRetry: 2,
			Enabled:  true,
		}
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

	// 自动检测项目类型
	if pt := detectProjectTypeWeb(s.projectRoot); pt != "" {
		if len(cfg.Commands.Allowed) == 0 {
			cfg.Commands.Allowed = suggestedCommandsWeb(pt)
		}
	}

	if err := s.loader.Save(cfg); err != nil {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": cfg.Version})
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

<!-- DeepSeek -->
<div class="card">
<div class="card-title"><span class="icon">🚀</span> DeepSeek Executor</div>
<label class="toggle">
  <input type="checkbox" id="ds-enabled" checked onchange="toggleSection('ds')">
  <div class="switch"></div> 启用
</label>
<div id="ds-section">
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
    const r=await fetch('/api/config');
    const cfg=await r.json();
    if(cfg.providers&&cfg.providers.configs){
      const ds=cfg.providers.configs.deepseek;
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
      }
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
  }catch(e){}
}

function showStatus(msg,type){
  const s=document.getElementById('status');
  s.textContent=msg;s.className='status-bar '+type;
  setTimeout(()=>{s.style.display='none';s.className='status-bar'},5000);
}

async function saveConfig(){
  const dsEnabled=document.getElementById('ds-enabled').checked;
  if(!dsEnabled){showStatus('请启用 DeepSeek','error');return}
  const models=getSelectedModels();
  if(models.length===0){showStatus('请至少选择一个模型','error');return}

  const body={
    deepseek_key:document.getElementById('ds-key').value,
    deepseek_models:models,
    deepseek_enabled:true,
    allowed_commands:document.getElementById('allowed-cmds').value,
    forbidden_commands:document.getElementById('forbidden-cmds').value,
    cmd_timeout:parseInt(document.getElementById('cmd-timeout').value)||300,
    req_timeout:parseInt(document.getElementById('req-timeout').value)||120
  };
  try{
    const r=await fetch('/api/config/save',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
    const d=await r.json();
    if(d.ok)showStatus('✅ 配置已保存！版本 '+d.version,'success');
    else showStatus('❌ 保存失败: '+d.error,'error');
  }catch(e){showStatus('❌ 请求失败: '+e.message,'error')}
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
