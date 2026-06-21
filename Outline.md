# coding-bridge 工程化新版大纲

## 1. 项目定位

`coding-bridge` 是一个面向 AI Coding Agent 的安全执行桥接系统。

**技术栈：Go 1.26+**

它不是简单的模型调用脚本，而是一个具备：

```text id="fp10qc"
任务事务
模型调度
Patch 校验
Git / Bak 保护
命令执行
测试验证
报告回传
失败恢复
配置管理
审计追踪
Web 配置界面
交互式初始化向导
自动浏览器打开
```

能力的本地自动化工具。

核心目标：

```text id="scgkk9"
高质量模型负责分析、规划、审查；
低成本模型负责执行、生成 patch；
coding-bridge 负责安全执行、状态管理、失败恢复；
Git / Bak 负责回滚和保护；
Report 负责把真实执行结果同步给 Codex 或其他控制模型。
```

---

## 2. 核心角色划分

```text id="ogt4dj"
Controller Model
  高质量模型，例如 Codex / GPT-5.5
  负责分析、拆任务、审查结果、决定下一步

Executor Model
  低成本或本地模型，例如 DeepSeek / Qwen / Ollama
  负责根据 task.json 生成 patch

coding-bridge Runner
  负责调度 provider、校验 patch、创建快照、应用修改、执行测试、生成报告

Git / Bak Layer
  负责执行前保护、执行后回滚

User
  负责确认高危操作、确认最终合并
```

---

## 3. 总体流程

```text id="f0btym"
User
  ↓
Controller Model
  ↓
生成 task.json
  ↓
coding-bridge Task Manager
  ↓
Provider Health Check
  ↓
Provider Selector
  ↓
Context Collector
  ↓
Executor Model
  ↓
Patch Generator
  ↓
Patch Validator
  ↓
Risk Guard
  ↓
Snapshot Manager
  ↓
Patch Applier
  ↓
Command Runner
  ↓
Build / Test / Lint
  ↓
Report Generator
  ↓
Controller Model Review
  ↓
Approve / Retry / Rollback / Finish
```

---

## 4. 核心设计原则

## 4.1 任务必须小

每次任务只允许处理一个明确问题。

不允许：

```text id="rzh9ns"
顺便重构
顺便优化
顺便改命名
顺便修复其他 bug
顺便升级依赖
```

---

## 4.2 执行模型只允许生成 patch

Executor Model 只能返回：

```text id="q63vv4"
unified diff
NEED_MORE_CONTEXT
REFUSE
FAILED
```

不允许执行模型：

```text id="ny4xjy"
直接执行命令
直接修改主分支
自由扩展需求
修改未授权文件
输出大段解释
```

---

## 4.3 所有修改必须可回滚

任何写入操作前必须创建保护点。

优先级：

```text id="txy8zk"
1. Git worktree
2. Git branch
3. Git stash + branch
4. Bak 文件级快照
```

---

## 4.4 所有执行必须生成报告

没有 report，不允许 Controller Model 继续下一步。

Report 必须包含：

```text id="otnq6u"
修改文件
git diff
执行命令
stdout
stderr
build 结果
test 结果
安全检查结果
失败原因
回滚信息
```

---

## 4.5 高危操作默认禁止

高危操作只有在配置允许，并且用户确认后才能执行。

---

## 5. 项目目录结构

```text id="szagk7"
coding-bridge/
  cmd/
    coding-bridge/
      main.go                    # 程序入口

  internal/
    cli/
      root.go                    # CLI 入口 (cobra)
      init.go                    # coding-bridge init (Web 引导)
      run.go                     # coding-bridge run
      status.go                  # coding-bridge status / report / rollback
      providers.go               # coding-bridge providers (list/check/benchmark)
      config.go                  # coding-bridge config (validate/reload/rollback)
      web.go                     # coding-bridge web (启动配置页面)

    providers/
      provider.go                # Provider 统一接口 + 类型定义
      registry.go                # 注册中心（可插拔架构）
      openai_compatible.go       # OpenAI 兼容通用基座
      codex.go                   # Codex Provider
      deepseek.go                # DeepSeek Provider
      health.go                  # 健康检测 + 自动选择 + 基准测试

    core/
      task.go                    # Task 模型 + 状态机 + 任务事务
      runner.go                  # 核心执行引擎（串联全流程）
      errors.go                  # 错误码体系

    config/
      schema.go                  # 完整配置 Schema + 默认值
      loader.go                  # 配置加载/保存/环境变量解析

    patch/
      patch.go                   # Unified Diff 解析/校验/应用

    sandbox/
      sandbox.go                 # Git worktree/branch/stash + Bak 快照

    commands/
      runner.go                  # 命令白名单/黑名单 + 安全执行

    report/
      generator.go               # Markdown 报告生成

    context/
      collector.go               # 上下文收集 + 脱敏

    security/                    # 安全模块（待实现）
    web/
      server.go                  # Web 配置页面（内嵌 HTML + API）

  config.example.yaml            # 配置示例
  task.example.json              # 任务示例
  go.mod
  go.sum
  README.md
  LICENSE
  NOTICE
```

---

## 6. 任务模型

## 6.1 task.json

```json id="hrqcvw"
{
  "task_id": "fix-modbus-timeout-release-port",
  "title": "修复 Modbus RTU 超时后串口未释放的问题",
  "description": "超时后需要释放串口锁，但不得修改协议帧解析逻辑。",
  "controller": {
    "provider": "openai",
    "model": "gpt-5.5"
  },
  "executor": {
    "selection": "auto",
    "preferred_provider": "deepseek",
    "preferred_model": "deepseek-v4-pro"
  },
  "allowed_files": [
    "src/Protocols/VfdModbusRtuCollector.cs",
    "tests/Protocols/VfdModbusRtuCollectorTests.cs"
  ],
  "forbidden_files": [
    ".env",
    "appsettings.Production.json",
    "secrets.json",
    "*.pfx",
    "*.key"
  ],
  "allowed_commands": [
    "dotnet build",
    "dotnet test"
  ],
  "requirements": [
    "不得修改协议帧解析逻辑",
    "不得吞掉异常",
    "必须保留原有日志格式",
    "超时后必须释放串口锁"
  ],
  "acceptance_criteria": [
    "dotnet build 成功",
    "dotnet test 成功",
    "只修改 allowed_files",
    "没有新增敏感信息",
    "没有修改生产配置"
  ],
  "risk": {
    "allow_high_risk": false,
    "allow_forbidden_read": false
  },
  "output_format": "unified_diff_only"
}
```

---

## 6.2 任务状态机

每个任务必须进入状态机管理。

```text id="o0xbui"
created
  ↓
validated
  ↓
context_collected
  ↓
provider_selected
  ↓
patch_requested
  ↓
patch_generated
  ↓
patch_validated
  ↓
risk_checked
  ↓
snapshot_created
  ↓
patch_applied
  ↓
commands_executed
  ↓
report_generated
  ↓
review_required
  ↓
completed / failed / rolled_back / cancelled
```

---

## 6.3 任务事务

每个任务是一个事务。

事务必须记录：

```text id="b9hes2"
task_id
config_version
provider
model
context_hash
patch_hash
snapshot_id
worktree_path
branch_name
started_at
finished_at
status
failure_reason
rollback_method
```

---

## 7. Provider 管理

## 7.1 Provider 类型

支持：

```text id="w4ejl3"
OpenAI-compatible API（通用基座，覆盖 80% 场景）
DeepSeek
OpenAI / Codex
Qwen
Claude
Gemini
Ollama
LM Studio
GitHub Models
Copilot CLI
```

### 7.1.1 可插拔架构

新增 Provider 只需 3 步：

1. 创建 Provider 结构体（嵌入 `OpenAICompatibleProvider` 或实现 `Provider` 接口）
2. 在 CLI 初始化中 `registry.Register(...)` + `registry.RegisterAlias(...)`
3. 配置文件添加配置项

```go
// Provider 接口 —— 所有 Provider 必须实现
type Provider interface {
    Type() ProviderType
    Name() string
    Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)
    HealthCheck(ctx context.Context) (*HealthCheckResult, error)
    ListModels(ctx context.Context) ([]string, error)
    SupportsCapability(cap ModelCapability) bool
    IsAvailable(ctx context.Context) bool
}
```

### 7.1.2 OpenAICompatibleProvider 基座

所有兼容 OpenAI API 格式的 Provider（Codex、DeepSeek、Qwen、Ollama 等）
均基于 `OpenAICompatibleProvider` 实现，只需配置不同的 BaseURL 和模型名。

- Codex: `https://api.openai.com/v1`
- DeepSeek: `https://api.deepseek.com`
- Qwen: `https://dashscope.aliyuncs.com/compatible-mode/v1`

---

## 7.2 Provider 健康检测

Provider 检测分层：

```text id="lcagfb"
1. connectivity：网络是否可达
2. auth：密钥是否有效
3. model：模型是否存在
4. capability：是否支持 JSON / patch / 长上下文
5. latency：延迟
6. quota：额度或限流
7. quality：最近成功率
8. cost：成本
9. trust：是否允许处理敏感上下文
```

命令：

```bash id="gy3mbf"
coding-bridge providers list
coding-bridge providers check
coding-bridge providers check --provider deepseek
coding-bridge providers benchmark
```

---

## 7.3 Provider 自动选择

选择依据：

```text id="vh6654"
任务复杂度
任务风险等级
Provider 存活状态
Provider 成本
Provider 延迟
Provider 最近成功率
Provider 是否支持 patch-only
Provider 是否允许读取敏感上下文
```

配置：

```yaml id="enbrmx"
provider_selection:
  mode: "auto"

  strategy:
    prefer_available: true
    prefer_low_cost: true
    prefer_fast_response: true
    prefer_patch_accuracy: true

  fallback_order:
    - deepseek
    - qwen
    - openai
    - ollama
```

---

## 8. Context Collector

## 8.1 上下文收集范围

允许收集：

```text id="lmkytl"
allowed_files
相关测试文件
公开文档
编译错误
接口定义
类型定义
```

默认禁止：

```text id="ctwbhy"
.env
secrets.json
appsettings.Production.json
私钥
证书
token
cookie
数据库连接串
真实账号密码
```

---

## 8.2 禁止文件临时授权

禁止文件可以在用户明确要求时读取，但必须分级授权。

```text id="61yqtf"
Level 0：禁止读取
Level 1：本地读取，不发送给模型
Level 2：脱敏后发送给模型
Level 3：完整发送给模型，必须人工确认
```

配置：

```yaml id="6roftt"
security:
  forbidden_file_read_policy:
    default: "deny"
    allow_user_override: true
    require_reason: true
    allow_local_read: true
    allow_send_masked_to_provider: true
    allow_send_raw_to_provider: false
    audit_log: true
```

---

## 8.3 脱敏规则

必须脱敏：

```text id="ea2qw7"
API Key
token
cookie
password
private key
connection string
JWT
session id
证书内容
```

---

## 9. Patch 系统

## 9.1 Patch-only 原则

执行模型只允许返回 unified diff。

---

## 9.2 Patch Validator

校验内容：

```text id="n97v0a"
文件路径是否在 allowed_files
是否触碰 forbidden_files
是否删除文件
是否修改二进制文件
是否修改生产配置
是否新增敏感信息
是否新增危险 API
是否注释掉测试
是否吞异常
是否绕过校验
```

---

## 9.3 语义风险扫描

语言级扫描：

```text id="d2y6n2"
C#:
  Process.Start
  File.Delete
  Directory.Delete
  HttpClient 上传
  Environment.GetEnvironmentVariable
  Registry
  Assembly.Load
  catch {}

JavaScript:
  child_process
  eval
  fs.rm
  fetch 上传
  process.env

Python:
  os.system
  subprocess
  eval
  exec
  requests 上传
  shutil.rmtree
```

---

## 10. Snapshot 与回滚

## 10.1 Git 仓库策略

优先级：

```text id="dldf9t"
1. git worktree
2. git branch
3. git stash + branch
```

推荐：

```bash id="pkvt58"
git worktree add .coding-bridge/worktrees/task-id -b bridge/task-id
```

---

## 10.2 无 Git 仓库策略

创建文件级 bak 快照。

目录：

```text id="le7fs5"
.coding-bridge/backups/
  task-id/
    manifest.json
    files/
    restore.sh
    restore.ps1
```

manifest 记录：

```json id="efrvog"
{
  "task_id": "task-id",
  "created_at": "2026-06-15T12:00:00",
  "files": [
    {
      "path": "src/A.cs",
      "encoding": "utf-8",
      "line_ending": "crlf",
      "sha256_before": "xxx",
      "backup_path": ".coding-bridge/backups/task-id/files/src/A.cs"
    }
  ]
}
```

---

## 10.3 回滚命令

```bash id="hlyvqw"
coding-bridge rollback task-id
coding-bridge restore task-id
```

---

## 11. 高危操作控制

## 11.1 高危操作定义

```text id="0eropc"
删除文件
批量重命名
修改配置
修改数据库脚本
执行 shell
修改依赖文件
执行 git reset
执行 git clean
网络下载脚本
部署命令
数据库写入
```

---

## 11.2 高危操作策略

```yaml id="k5v4p1"
risk:
  allow_high_risk_operations: false

  high_risk_policy:
    require_user_confirm: true
    require_snapshot_before_execute: true
    git_snapshot_mode: "worktree"
    non_git_snapshot_mode: "bak"
    allow_delete_files: false
    allow_modify_config: false
    allow_run_shell: false
    allow_database_write: false
```

---

## 12. 命令执行系统

## 12.1 命令白名单

```yaml id="p8src9"
commands:
  allowed:
    - "dotnet build"
    - "dotnet test"
    - "npm test"
    - "npm run build"
    - "pytest"
```

---

## 12.2 命令黑名单

```yaml id="kzjg0p"
commands:
  forbidden:
    - "rm -rf"
    - "git reset --hard"
    - "git clean -fdx"
    - "ssh"
    - "scp"
    - "curl"
    - "wget"
    - "powershell iwr"
    - "Invoke-WebRequest"
```

---

## 12.3 超时控制

```yaml id="u216yj"
timeouts:
  provider_request_seconds: 120
  provider_health_seconds: 10
  patch_apply_seconds: 30
  command_seconds: 300
  web_request_seconds: 30
```

---

## 13. 编码与写入安全

## 13.1 编码检测

支持：

```text id="xt6jal"
UTF-8
UTF-8 BOM
GBK
GB2312
UTF-16 LE
UTF-16 BE
ASCII
```

---

## 13.2 写入规则

```text id="yfug59"
1. 优先 patch，不整文件覆盖
2. 保持原编码
3. 保持原换行符
4. 写入前备份
5. 写入后 round-trip 校验
6. 检查替换字符 �
7. 失败自动回滚
```

---

## 13.3 配置

```yaml id="9hcawq"
encoding:
  detect_before_write: true
  preserve_original_encoding: true
  preserve_line_endings: true
  reject_on_decode_error: true
  reject_if_replacement_char_found: true
  default_encoding: "utf-8"
  fallback_encoding:
    - "utf-8-sig"
    - "gbk"
    - "gb2312"
```

---

## 14. 配置系统

## 14.1 配置文件

```text id="2v5aop"
.coding-bridge/config.yaml
```

---

## 14.2 配置 Schema 校验

命令：

```bash id="8avpuo"
coding-bridge config validate
```

所有配置必须经过 schema 校验。

---

## 14.3 配置版本

每次配置变化生成版本号。

```text id="zo0amk"
config_version: 12
```

每个任务记录使用的配置版本。

---

## 14.4 配置历史

```text id="trte7y"
.coding-bridge/config-history/
  2026-06-15-120000.config.yaml
  2026-06-15-121500.config.yaml
```

回滚：

```bash id="fzhx6b"
coding-bridge config rollback
```

---

## 15. Web 配置界面

### 15.1 启动

```bash id="ww4ik9"
coding-bridge web
coding-bridge init          # 首次初始化，自动启动 Web 并打开浏览器
coding-bridge init --quick  # 快速模式：跳过 Web，生成默认配置
coding-bridge init --port 9999
coding-bridge web --port 9999
```

默认：

```text id="c105e9"
http://127.0.0.1:8765
```

### 15.2 自动打开浏览器

启动 Web 服务后，自动调用系统默认浏览器打开配置页面。

- Windows: `rundll32 url.dll,FileProtocolHandler`
- macOS: `open`
- Linux: `xdg-open`

终端同步打印 URL，方便手动复制。

### 15.3 Web 页面功能（v0.1 已实现）

```text id="anwi9t-v2"
✅ Provider 开关（启用/禁用 DeepSeek、OpenAI）
✅ API Key 配置（支持直接填写或从环境变量读取）
✅ 模型选择
✅ 默认 Executor 选择（多 Provider 时）
✅ 一键保存到 config.yaml
✅ 实时连通性检测（Health Check）
✅ 暗色主题 UI（GitHub 风格）
✅ 单文件内嵌 HTML（零外部依赖）
```

### 15.4 Web API

| 端点 | 方法 | 功能 |
|------|------|------|
| `/` | GET | 配置页面 |
| `/api/config` | GET | 获取当前配置（API Key 脱敏） |
| `/api/config/save` | POST | 保存配置 |
| `/api/config/check` | POST | 检测 Provider 连通性 |

### 15.5 Web 安全

```text id="mmf3q2-v2"
✅ 只监听 127.0.0.1（禁止远程访问）
⏳ 本地 token 认证（待实现）
⏳ 高危操作二次确认（待实现）
⏳ 修改配置写入审计日志（待实现）
```

```bash id="k4m9t1"
coding-bridge config reload
```

规则：

```text id="dzykit"
正在执行的任务继续使用旧配置
新任务使用新配置
配置变更写入 audit log
Provider health cache 清空
```

---

## 16. 日志与审计

## 16.1 日志类型

```text id="1znngj"
app.log
task-taskid.log
provider.log
audit.log
web.log
```

---

## 16.2 审计内容

```text id="twusre"
读取禁止文件
允许高危操作
修改配置
执行命令
回滚任务
Provider fallback
发送上下文给远程模型
```

---

## 17. 错误码体系

```text id="fu4skd"
CB_PROVIDER_UNAVAILABLE
CB_PROVIDER_AUTH_FAILED
CB_PROVIDER_MODEL_NOT_FOUND
CB_PATCH_INVALID
CB_PATCH_OUT_OF_SCOPE
CB_PATCH_APPLY_FAILED
CB_SNAPSHOT_FAILED
CB_BACKUP_FAILED
CB_COMMAND_TIMEOUT
CB_ENCODING_ERROR
CB_FORBIDDEN_FILE_ACCESS
CB_HIGH_RISK_BLOCKED
CB_CONFIG_INVALID
CB_TASK_ALREADY_RUNNING
CB_TASK_RECOVERY_REQUIRED
```

---

## 18. 并发与锁机制

## 18.1 锁文件

```text id="gbz3rc"
.coding-bridge/locks/
  project.lock
  config.lock
  task-taskid.lock
```

---

## 18.2 并发规则

```text id="j827e4"
同一项目默认只允许一个写任务
允许多个只读任务
不同 worktree 可以并发
配置修改不能影响正在执行的任务
```

---

## 19. 崩溃恢复

## 19.1 Doctor 命令

```bash id="a8oce4"
coding-bridge doctor
```

检查：

```text id="k36ho6"
未完成任务
残留锁
损坏 report
未清理 worktree
未完成 backup
配置错误
Provider 状态
```

---

## 19.2 Recovery

```bash id="spqqbo"
coding-bridge recover
```

恢复策略：

```text id="g19da3"
如果 patch 未应用，可以继续
如果 patch 已应用但命令未执行，可以继续执行命令
如果命令执行失败，可以生成失败报告
如果状态不确定，要求用户选择 rollback 或 continue
```

---

## 20. CLI 命令设计

```bash id="o8u4fh-v2"
coding-bridge init                 # 启动 Web 配置页面 + 自动打开浏览器
coding-bridge init --quick         # 快速模式：跳过 Web，生成默认配置
coding-bridge init --port 9999     # 指定 Web 端口

coding-bridge web                  # 启动 Web 配置页面（编辑现有配置）
coding-bridge web --port 9999      # 指定端口

coding-bridge providers list       # 列出已注册 Provider
coding-bridge providers check      # 检测所有 Provider 连通性
coding-bridge providers check --provider deepseek
coding-bridge providers benchmark

coding-bridge run task.json
coding-bridge run task.json --provider deepseek --model deepseek-v4-pro
coding-bridge run task.json --dry-run
coding-bridge run task.json --allow-high-risk
coding-bridge run task.json --allow-read-forbidden

coding-bridge status
coding-bridge report latest
coding-bridge report task-id

coding-bridge rollback task-id
coding-bridge restore task-id

coding-bridge config validate
coding-bridge config reload
coding-bridge config rollback

coding-bridge doctor
coding-bridge recover
coding-bridge codex install       # 安装/更新项目 AGENTS.md 对话接入规则
```

---

## 21. 开发工具接入

## 21.1 Codex 接入

通过：

```text id="qjbds6"
AGENTS.md
Codex Skill
MCP Server
CLI
```

流程：

```text id="4dsd4w"
Codex 生成 task.json
调用 coding-bridge run
读取 report
根据 report 继续分析
```

---

## 21.2 VS Code 接入

`.vscode/tasks.json`

```json id="qbxsaa"
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "coding-bridge: run latest task",
      "type": "shell",
      "command": "coding-bridge run .coding-bridge/tasks/latest.json",
      "problemMatcher": []
    },
    {
      "label": "coding-bridge: provider check",
      "type": "shell",
      "command": "coding-bridge providers check",
      "problemMatcher": []
    },
    {
      "label": "coding-bridge: web",
      "type": "shell",
      "command": "coding-bridge web",
      "problemMatcher": []
    }
  ]
}
```

---

## 21.3 MCP Server

工具：

```text id="ahg8tl"
bridge_create_task
bridge_run_task
bridge_get_status
bridge_get_report
bridge_rollback
bridge_provider_check
bridge_config_reload
```

---

## 22. 测试体系

## 22.1 测试类型

```text id="ffkskq"
Unit Tests
Integration Tests
E2E Tests
Snapshot Tests
Provider Mock Tests
Encoding Tests
Git Tests
Backup / Restore Tests
Web Config Tests
Security Tests
Crash Recovery Tests
```

---

## 22.2 重点测试场景

```text id="jewrd6"
Provider 不可用
Provider 返回非 diff
Provider 返回越界 diff
Patch apply 失败
Build 失败
Test 失败
Git 仓库脏状态
无 Git 项目
GBK 文件写入
CRLF 文件写入
高危操作前备份
高危操作失败后恢复
配置热刷新
任务中途崩溃
禁止文件授权读取
敏感信息脱敏
```

---

## 23. 版本规划

## 23.1 v0.1：可靠执行闭环 ✅ 已完成

```text id="tbmubo-v2"
✅ 1. Go 项目骨架 (cmd/internal 结构)
✅ 2. init (Web 引导 + 自动打开浏览器)
✅ 3. config.yaml (完整 Schema + 默认值 + 环境变量)
✅ 4. task.json (加载/校验/状态机/事务记录)
✅ 5. Codex Provider (OpenAI 兼容)
✅ 6. DeepSeek Provider (含 deepseek-v4-pro)
✅ 7. OpenAICompatibleProvider 通用基座
✅ 8. Provider Registry (可插拔架构 + 别名)
✅ 9. Provider Health Check (9 维度检测 + 自动选择 + 基准测试)
✅ 10. patch-only (Unified Diff 解析/校验/应用)
✅ 11. allowed_files / forbidden_files 校验
✅ 12. git worktree / branch / stash / bak 快照
✅ 13. 命令白名单/黑名单 + 安全执行
✅ 14. 自动构建/测试 (go build, dotnet build, npm 等)
✅ 15. report.md 生成
✅ 16. rollback (Git + Bak)
✅ 17. Web 配置界面 (内嵌 HTML + API + 实时连通性检测)
✅ 18. CLI (cobra, 11 个子命令)
✅ 19. 错误码体系 (15 个错误码)
✅ 20. 上下文收集 + 脱敏
✅ 21. Apache 2.0 许可证
✅ 22. README + 完整文档
```

---

## 23.2 v0.2：安全增强

```text id="f3mh5o"
1. Risk Guard
2. Git stash fallback
3. Bak snapshot
4. Secret scanner
5. Encoding guard
6. Forbidden file policy
7. Audit log
8. Error codes
```

---

## 23.3 v0.3：鲁棒性增强

```text id="dg877v"
1. Task State Machine
2. Transaction Manager
3. Lock Manager
4. Doctor
5. Recover
6. Timeout
7. Retry policy
8. Config schema
```

---

## 23.4 v0.4：开发体验增强

```text id="q1yj8x"
1. Web UI
2. Config hot reload
3. VS Code Task
4. Codex Skill
5. MCP Server
6. Provider benchmark
```

---

## 23.5 v1.0：完整版本

```text id="u00lhv"
1. 多 Provider 调度
2. 成本统计
3. 任务队列
4. Web Dashboard
5. 完整审计日志
6. CI 集成
7. PR 生成
8. VS Code Extension
9. 多项目管理
```

---

## 24. 最终原则

```text id="m3bssk"
1. 先保护，再执行；
2. 先校验，再写入；
3. 先报告，再继续；
4. 先小任务，再自动化；
5. 执行模型只能生成 patch；
6. 高危操作必须确认；
7. 禁止文件必须授权；
8. 配置修改必须审计；
9. Provider 必须可检测、可 fallback；
10. 所有任务必须可恢复、可回滚、可追踪。
```

---

## 25. 项目一句话总结

```text id="cf1efv"
coding-bridge 是一个面向 AI Coding Agent 的安全执行事务系统，
用于让多个大模型在受控、可验证、可回滚的工程环境中协同完成代码修改任务。
```
