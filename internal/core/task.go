package core

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/coding-bridge/internal/commands"
)

var taskIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// TaskState 任务状态
type TaskState string

const (
	StateCreated          TaskState = "created"
	StateValidated        TaskState = "validated"
	StateContextCollected TaskState = "context_collected"
	StateProviderSelected TaskState = "provider_selected"
	StatePatchRequested   TaskState = "patch_requested"
	StatePatchGenerated   TaskState = "patch_generated"
	StatePatchValidated   TaskState = "patch_validated"
	StateRiskChecked      TaskState = "risk_checked"
	StateSnapshotCreated  TaskState = "snapshot_created"
	StatePatchApplied     TaskState = "patch_applied"
	StateCommandsExecuted TaskState = "commands_executed"
	StateReportGenerated  TaskState = "report_generated"
	StateReviewRequired   TaskState = "review_required"
	StateCompleted        TaskState = "completed"
	StateFailed           TaskState = "failed"
	StateRolledBack       TaskState = "rolled_back"
	StateCancelled        TaskState = "cancelled"
)

// Task 表示一个代码修改任务
type Task struct {
	// 基本信息
	TaskID      string `json:"task_id" yaml:"task_id"`
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description" yaml:"description"`

	// Controller 模型配置
	Controller ControllerConfig `json:"controller" yaml:"controller"`

	// Executor 模型配置
	Executor ExecutorConfig `json:"executor" yaml:"executor"`

	// 文件范围控制
	AllowedFiles   []string `json:"allowed_files" yaml:"allowed_files"`
	ForbiddenFiles []string `json:"forbidden_files" yaml:"forbidden_files"`

	// 允许执行的命令
	AllowedCommands []string `json:"allowed_commands" yaml:"allowed_commands"`

	// 需求约束
	Requirements []string `json:"requirements" yaml:"requirements"`

	// 验收标准
	AcceptanceCriteria []string `json:"acceptance_criteria" yaml:"acceptance_criteria"`

	// 风险控制
	Risk RiskConfig `json:"risk" yaml:"risk"`

	// 输出格式
	OutputFormat string `json:"output_format" yaml:"output_format"`
}

// ControllerConfig Controller 模型配置
type ControllerConfig struct {
	Provider       string `json:"provider" yaml:"provider"`
	Model          string `json:"model" yaml:"model"`
	ObservedTokens int    `json:"observed_tokens,omitempty" yaml:"observed_tokens,omitempty"`
}

// ExecutorConfig Executor 模型配置
type ExecutorConfig struct {
	Selection         string `json:"selection" yaml:"selection"`                   // auto, manual
	PreferredProvider string `json:"preferred_provider" yaml:"preferred_provider"` // 首选 Provider
	PreferredModel    string `json:"preferred_model" yaml:"preferred_model"`       // 首选模型
}

// RiskConfig 任务级风险配置
type RiskConfig struct {
	AllowHighRisk      bool `json:"allow_high_risk" yaml:"allow_high_risk"`
	AllowForbiddenRead bool `json:"allow_forbidden_read" yaml:"allow_forbidden_read"`
}

// TaskTransaction 任务事务记录
type TaskTransaction struct {
	TaskID         string     `json:"task_id" yaml:"task_id"`
	ConfigVersion  int        `json:"config_version" yaml:"config_version"`
	Provider       string     `json:"provider" yaml:"provider"`
	Model          string     `json:"model" yaml:"model"`
	ContextHash    string     `json:"context_hash" yaml:"context_hash"`
	PatchHash      string     `json:"patch_hash" yaml:"patch_hash"`
	SnapshotID     string     `json:"snapshot_id" yaml:"snapshot_id"`
	WorktreePath   string     `json:"worktree_path,omitempty" yaml:"worktree_path,omitempty"`
	BranchName     string     `json:"branch_name,omitempty" yaml:"branch_name,omitempty"`
	StartedAt      time.Time  `json:"started_at" yaml:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
	Status         TaskState  `json:"status" yaml:"status"`
	FailureReason  string     `json:"failure_reason,omitempty" yaml:"failure_reason,omitempty"`
	RollbackMethod string     `json:"rollback_method,omitempty" yaml:"rollback_method,omitempty"`
}

// TaskResult 任务执行结果
type TaskResult struct {
	TaskID                  string                    `json:"task_id"`
	Status                  TaskState                 `json:"status"`
	Provider                string                    `json:"provider,omitempty"`
	Model                   string                    `json:"model,omitempty"`
	ContextFiles            int                       `json:"context_files"`
	ContextBytes            int                       `json:"context_bytes"`
	PromptTokens            int                       `json:"prompt_tokens"`
	CompletionTokens        int                       `json:"completion_tokens"`
	TotalTokens             int                       `json:"total_tokens"`
	ControllerTokens        int                       `json:"controller_tokens,omitempty"`
	EstimatedDirectTokens   int                       `json:"estimated_direct_tokens,omitempty"`
	EstimatedGrossSavings   int                       `json:"estimated_gross_savings,omitempty"`
	EstimatedNetSavings     int                       `json:"estimated_net_savings,omitempty"`
	TruncatedOutput         bool                      `json:"truncated_output"`
	PatchEffectVerified     bool                      `json:"patch_effect_verified"`
	EffectiveChangedFiles   int                       `json:"effective_changed_files"`
	GenerationAttempts      int                       `json:"generation_attempts"`
	MaxRepairAttempts       int                       `json:"max_repair_attempts"`
	PatchChangedLines       int                       `json:"patch_changed_lines"`
	ExecutorEffectiveTokens int                       `json:"executor_effective_tokens"`
	ExecutorWastedTokens    int                       `json:"executor_wasted_tokens"`
	ExecutorWasteRate       float64                   `json:"executor_waste_rate"`
	FileHashChanges         []FileHashChange          `json:"file_hash_changes,omitempty"`
	TechnicalVerification   string                    `json:"technical_verification"`
	BusinessAcceptance      string                    `json:"business_acceptance"`
	ModifiedFiles           []string                  `json:"modified_files"`
	GitDiff                 string                    `json:"git_diff"`
	CommandsRun             []commands.CommandResult  `json:"commands_run"`
	BuildResult             *commands.BuildTestResult `json:"build_result,omitempty"`
	TestResult              *commands.BuildTestResult `json:"test_result,omitempty"`
	SecurityCheck           *SecurityCheckResult      `json:"security_check,omitempty"`
	FailureReason           string                    `json:"failure_reason,omitempty"`
	RollbackInfo            string                    `json:"rollback_info,omitempty"`
	StartedAt               time.Time                 `json:"started_at"`
	FinishedAt              time.Time                 `json:"finished_at"`
}

type FileHashChange struct {
	File         string `json:"file"`
	BeforeSHA256 string `json:"before_sha256"`
	AfterSHA256  string `json:"after_sha256"`
}

// SecurityCheckResult 安全检查结果
type SecurityCheckResult struct {
	Passed                 bool     `json:"passed"`
	Issues                 []string `json:"issues,omitempty"`
	SecretFound            bool     `json:"secret_found"`
	ForbiddenFilesAccessed bool     `json:"forbidden_files_accessed"`
}

// LoadTask 从 JSON 文件加载任务
func LoadTask(path string) (*Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read task file: %w", err)
	}

	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("parse task file: %w", err)
	}

	return &task, nil
}

// SaveTask 保存任务到文件
func SaveTask(task *Task, path string) error {
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// Validate 校验任务有效性
func (t *Task) Validate() []error {
	var errs []error

	if t.TaskID == "" {
		errs = append(errs, fmt.Errorf("task_id is required"))
	} else if !taskIDPattern.MatchString(t.TaskID) {
		errs = append(errs, fmt.Errorf("task_id may only contain letters, numbers, dot, underscore, and hyphen"))
	}

	if t.Title == "" {
		errs = append(errs, fmt.Errorf("title is required"))
	}

	if len(t.AllowedFiles) == 0 {
		errs = append(errs, fmt.Errorf("allowed_files must not be empty"))
	}

	if len(t.AllowedCommands) == 0 {
		errs = append(errs, fmt.Errorf("allowed_commands must not be empty"))
	}

	if t.Executor.PreferredProvider == "" && t.Executor.Selection != "auto" {
		errs = append(errs, fmt.Errorf("executor.preferred_provider is required when selection is not auto"))
	}

	if t.OutputFormat == "" {
		t.OutputFormat = "unified_diff_only"
	}

	return errs
}

// --- 状态机 ---

// StateMachine 任务状态机
type StateMachine struct {
	current TaskState
}

// NewStateMachine 创建状态机
func NewStateMachine(initial TaskState) *StateMachine {
	return &StateMachine{current: initial}
}

// Current 返回当前状态
func (sm *StateMachine) Current() TaskState {
	return sm.current
}

// validTransitions 定义合法的状态转换
var validTransitions = map[TaskState][]TaskState{
	StateCreated:          {StateValidated, StateFailed, StateCancelled},
	StateValidated:        {StateContextCollected, StateFailed, StateCancelled},
	StateContextCollected: {StateProviderSelected, StateFailed, StateCancelled},
	StateProviderSelected: {StatePatchRequested, StateFailed, StateCancelled},
	StatePatchRequested:   {StatePatchGenerated, StateFailed, StateCancelled},
	StatePatchGenerated:   {StatePatchValidated, StateFailed, StateCancelled},
	StatePatchValidated:   {StateRiskChecked, StateFailed, StateCancelled},
	StateRiskChecked:      {StateSnapshotCreated, StateFailed, StateCancelled},
	StateSnapshotCreated:  {StatePatchApplied, StateFailed, StateCancelled},
	StatePatchApplied:     {StateCommandsExecuted, StateFailed, StateCancelled},
	StateCommandsExecuted: {StateReportGenerated, StateFailed, StateCancelled},
	StateReportGenerated:  {StateReviewRequired, StateFailed, StateCancelled},
	StateReviewRequired:   {StateCompleted, StateFailed, StateRolledBack, StateCancelled},
	// 终态
	StateCompleted:  {},
	StateFailed:     {StateRolledBack},
	StateRolledBack: {},
	StateCancelled:  {},
}

// CanTransition 检查是否可以从当前状态转换到目标状态
func (sm *StateMachine) CanTransition(to TaskState) bool {
	allowed, ok := validTransitions[sm.current]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// Transition 尝试转换状态
func (sm *StateMachine) Transition(to TaskState) error {
	if !sm.CanTransition(to) {
		return fmt.Errorf("invalid state transition: %s -> %s", sm.current, to)
	}
	sm.current = to
	return nil
}

// IsTerminal 检查是否是终态
func (sm *StateMachine) IsTerminal() bool {
	terminalStates := map[TaskState]bool{
		StateCompleted:  true,
		StateFailed:     true,
		StateRolledBack: true,
		StateCancelled:  true,
	}
	return terminalStates[sm.current]
}

// IsFinal 检查是否是最终状态（终态中真正的终态）
func (sm *StateMachine) IsFinal() bool {
	finalStates := map[TaskState]bool{
		StateCompleted:  true,
		StateRolledBack: true,
		StateCancelled:  true,
	}
	return finalStates[sm.current]
}
