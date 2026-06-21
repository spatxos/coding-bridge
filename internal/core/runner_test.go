package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coding-bridge/internal/config"
	"github.com/coding-bridge/internal/patch"
	"github.com/coding-bridge/internal/providers"
)

type fakeProvider struct {
	response     *providers.GenerateResponse
	request      *providers.GenerateRequest
	providerType providers.ProviderType
	healthStatus providers.HealthStatus
}

func (p *fakeProvider) Type() providers.ProviderType {
	if p.providerType != "" {
		return p.providerType
	}
	return providers.ProviderDeepSeek
}
func (p *fakeProvider) Name() string { return "fake" }
func (p *fakeProvider) Generate(_ context.Context, req *providers.GenerateRequest) (*providers.GenerateResponse, error) {
	p.request = req
	return p.response, nil
}
func (p *fakeProvider) HealthCheck(context.Context) (*providers.HealthCheckResult, error) {
	status := p.healthStatus
	if status == "" {
		status = providers.HealthHealthy
	}
	return &providers.HealthCheckResult{Status: status}, nil
}
func (p *fakeProvider) ListModels(context.Context) ([]string, error) {
	return []string{"cheap-model"}, nil
}
func (p *fakeProvider) SupportsCapability(providers.ModelCapability) bool { return true }
func (p *fakeProvider) IsAvailable(context.Context) bool                  { return true }

func TestRunnerSendsContextUsesRequestedModelAndWritesReport(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "main.txt")
	if err := os.WriteFile(sourcePath, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}

	provider := &fakeProvider{response: &providers.GenerateResponse{
		Content: strings.Join([]string{
			"diff --git a/main.txt b/main.txt",
			"--- a/main.txt",
			"+++ b/main.txt",
			"@@ -1 +1 @@",
			"-old",
			"+new",
			patch.EndMarker,
		}, "\n"),
		Model: "cheap-model",
		Usage: providers.UsageInfo{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
	}}

	result := newTestRunner(root, provider).Run(context.Background(), validTestTask())
	if result.Err != nil {
		t.Fatalf("Run() error = %v", result.Err)
	}
	if result.TaskResult.Status != StateCompleted {
		t.Fatalf("status = %s, want completed", result.TaskResult.Status)
	}
	if provider.request == nil {
		t.Fatal("provider did not receive a request")
	}
	if provider.request.Model != "cheap-model" {
		t.Fatalf("requested model = %q, want cheap-model", provider.request.Model)
	}
	if !strings.Contains(provider.request.Messages[0].Content, "--- main.txt ---\nold") {
		t.Fatal("executor request does not contain collected source context")
	}
	if result.TaskResult.TotalTokens != 120 {
		t.Fatalf("TotalTokens = %d, want 120", result.TaskResult.TotalTokens)
	}
	if !result.TaskResult.PatchEffectVerified ||
		result.TaskResult.EffectiveChangedFiles != 1 ||
		len(result.TaskResult.FileHashChanges) != 1 {
		t.Fatalf("patch verification = %#v", result.TaskResult)
	}
	if result.TaskResult.ExecutorEffectiveTokens != 120 ||
		result.TaskResult.ExecutorWastedTokens != 0 {
		t.Fatalf("token disposition = %#v", result.TaskResult)
	}
	if _, err := os.Stat(result.ReportPath); err != nil {
		t.Fatalf("report was not written: %v", err)
	}
}

func TestRunnerWritesFailureReport(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{Content: "not a diff\n" + patch.EndMarker}}

	result := newTestRunner(root, provider).Run(context.Background(), validTestTask())
	if result.Err == nil {
		t.Fatal("Run() error = nil, want patch parse error")
	}
	if result.TaskResult.Status != StateFailed {
		t.Fatalf("status = %s, want failed", result.TaskResult.Status)
	}
	if _, err := os.Stat(result.ReportPath); err != nil {
		t.Fatalf("failure report was not written: %v", err)
	}
}

func TestRunnerRejectsTruncatedExecutorOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{
		Content:      "diff --git a/main.txt b/main.txt\n--- a/main.txt\n",
		FinishReason: "length",
		Usage: providers.UsageInfo{
			CompletionTokens: 4096,
			TotalTokens:      4200,
		},
	}}

	result := newTestRunner(root, provider).Run(context.Background(), validTestTask())
	if result.Err == nil {
		t.Fatal("Run() error = nil, want truncation error")
	}
	if !strings.Contains(result.TaskResult.FailureReason, "truncated") {
		t.Fatalf("failure reason = %q", result.TaskResult.FailureReason)
	}
	if !result.TaskResult.TruncatedOutput {
		t.Fatal("TruncatedOutput = false, want true")
	}
}

func TestRunnerRejectsMissingEndMarkerEvenWhenFinishReasonIsStop(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{
		Content: strings.Join([]string{
			"diff --git a/main.txt b/main.txt",
			"--- a/main.txt",
			"+++ b/main.txt",
			"@@ -1 +1 @@",
			"-old",
			"+new",
		}, "\n"),
		FinishReason: "stop",
	}}

	result := newTestRunner(root, provider).Run(context.Background(), validTestTask())
	if result.Err == nil || !result.TaskResult.TruncatedOutput {
		t.Fatalf("result = %#v, err = %v", result.TaskResult, result.Err)
	}
	if !strings.Contains(result.TaskResult.FailureReason, "TRUNCATED_OUTPUT") {
		t.Fatalf("failure reason = %q", result.TaskResult.FailureReason)
	}
}

func TestRunnerAcceptsMarkdownWrappedDiffWithEndMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{
		Content: strings.Join([]string{
			"```diff",
			"diff --git a/main.txt b/main.txt",
			"--- a/main.txt",
			"+++ b/main.txt",
			"@@ -1 +1 @@",
			"-old",
			"+new",
			"```",
			patch.EndMarker,
		}, "\n"),
	}}

	result := newTestRunner(root, provider).Run(context.Background(), validTestTask())
	if result.Err != nil {
		t.Fatalf("Run() error = %v", result.Err)
	}
	if !result.TaskResult.PatchEffectVerified ||
		result.TaskResult.EffectiveChangedFiles != 1 {
		t.Fatalf("result = %#v", result.TaskResult)
	}
}

func TestRunnerRejectsPatchWithNoEffectiveHashChange(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{
		Content: strings.Join([]string{
			"diff --git a/main.txt b/main.txt",
			"--- a/main.txt",
			"+++ b/main.txt",
			"@@ -1 +1 @@",
			"-old",
			"+old",
			patch.EndMarker,
		}, "\n"),
	}}

	result := newTestRunner(root, provider).Run(context.Background(), validTestTask())
	if result.Err == nil {
		t.Fatal("Run() error = nil, want no effective change failure")
	}
	if !strings.Contains(result.TaskResult.FailureReason, "NO_EFFECTIVE_CHANGE") {
		t.Fatalf("failure reason = %q", result.TaskResult.FailureReason)
	}
	data, err := os.ReadFile(filepath.Join(root, "main.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old\n" {
		t.Fatalf("file content = %q", data)
	}
}

func TestRunnerRequestsLargerPatchTokenBudget(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{
		Content: strings.Join([]string{
			"diff --git a/main.txt b/main.txt",
			"--- a/main.txt",
			"+++ b/main.txt",
			"@@ -1 +1 @@",
			"-old",
			"+new",
			patch.EndMarker,
		}, "\n"),
	}}

	result := newTestRunner(root, provider).Run(context.Background(), validTestTask())
	if result.Err != nil {
		t.Fatalf("Run() error = %v", result.Err)
	}
	if provider.request == nil || provider.request.MaxTokens != 16384 {
		t.Fatalf("MaxTokens = %v, want 16384", provider.request)
	}
}

func TestRunnerUsesConfiguredPatchBudgetAndCalculatesObservedNetSavings(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{
		Content: strings.Join([]string{
			"diff --git a/main.txt b/main.txt",
			"--- a/main.txt",
			"+++ b/main.txt",
			"@@ -1 +1 @@",
			"-old",
			"+new",
			patch.EndMarker,
		}, "\n"),
		Usage: providers.UsageInfo{PromptTokens: 600, CompletionTokens: 400, TotalTokens: 1000},
	}}
	runner := newTestRunner(root, provider)
	runner.cfg.Execution.PatchMaxTokens = 8192
	runner.cfg.Execution.Temperature = 0
	runner.cfg.TokenAccounting.DirectCodexBaselineRatio = 3
	task := validTestTask()
	task.Controller.ObservedTokens = 500

	result := runner.Run(context.Background(), task)
	if result.Err != nil {
		t.Fatalf("Run() error = %v", result.Err)
	}
	if provider.request.MaxTokens != 8192 || provider.request.Temperature != 0 {
		t.Fatalf("request = %#v", provider.request)
	}
	if result.TaskResult.EstimatedDirectTokens != 3000 ||
		result.TaskResult.EstimatedGrossSavings != 2000 ||
		result.TaskResult.EstimatedNetSavings != 1500 {
		t.Fatalf("token estimates = %#v", result.TaskResult)
	}
}

func TestDryRunFallsBackToHealthyConfiguredProviderInAutoMode(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.ProviderSelection.FallbackOrder = []string{"deepseek", "qwen"}
	registry := providers.NewRegistry()
	registry.Register(&fakeProvider{
		providerType: providers.ProviderDeepSeek,
		healthStatus: providers.HealthUnhealthy,
	})
	registry.Register(&fakeProvider{
		providerType: providers.ProviderQwen,
		healthStatus: providers.HealthHealthy,
	})
	runner := NewRunner(root, cfg, registry)
	task := validTestTask()
	task.Executor.Selection = "auto"

	result, err := runner.DryRun(context.Background(), task)
	if err != nil {
		t.Fatalf("DryRun() error = %v", err)
	}
	if result.TaskResult.Status != StateCompleted {
		t.Fatalf("status = %s", result.TaskResult.Status)
	}
}

func TestRunnerRejectsOversizedTaskBeforeCallingExecutor(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt", "d.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("old\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{}}
	runner := newTestRunner(root, provider)
	task := validTestTask()
	task.AllowedFiles = []string{"a.txt", "b.txt", "c.txt", "d.txt"}

	result := runner.Run(context.Background(), task)
	if result.Err == nil || !strings.Contains(result.TaskResult.FailureReason, "TASK_TOO_BROAD") {
		t.Fatalf("result = %#v, err = %v", result.TaskResult, result.Err)
	}
	if provider.request != nil || result.TaskResult.TotalTokens != 0 {
		t.Fatalf("Executor was called for oversized task: %#v", provider.request)
	}
}

func TestRunnerRejectsOversizedPatchAndMarksTokensWasted(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{
		Content: strings.Join([]string{
			"diff --git a/main.txt b/main.txt",
			"--- a/main.txt",
			"+++ b/main.txt",
			"@@ -1 +1 @@",
			"-old",
			"+new",
			patch.EndMarker,
		}, "\n"),
		Usage: providers.UsageInfo{TotalTokens: 900},
	}}
	runner := newTestRunner(root, provider)
	runner.cfg.Execution.MaxPatchLines = 1

	result := runner.Run(context.Background(), validTestTask())
	if result.Err == nil || !strings.Contains(result.TaskResult.FailureReason, "PATCH_TOO_LARGE") {
		t.Fatalf("result = %#v, err = %v", result.TaskResult, result.Err)
	}
	if result.TaskResult.ExecutorWastedTokens != 900 ||
		result.TaskResult.ExecutorWasteRate != 1 {
		t.Fatalf("token disposition = %#v", result.TaskResult)
	}
}

func newTestRunner(root string, provider providers.Provider) *Runner {
	cfg := config.DefaultConfig()
	cfg.Commands.Allowed = []string{"go version"}
	cfg.Timeouts.Command = 30
	cfg.Report.OutputDir = filepath.Join(root, ".coding-bridge", "reports")

	registry := providers.NewRegistry()
	registry.Register(provider)
	return NewRunner(root, cfg, registry)
}

func validTestTask() *Task {
	return &Task{
		TaskID:      "test-task-" + time.Now().Format("150405.000000"),
		Title:       "replace text",
		Description: "replace old with new",
		Executor: ExecutorConfig{
			Selection:         "manual",
			PreferredProvider: "deepseek",
			PreferredModel:    "cheap-model",
		},
		AllowedFiles:       []string{"main.txt"},
		ForbiddenFiles:     []string{".env"},
		AllowedCommands:    []string{"go version"},
		Requirements:       []string{"minimal change"},
		AcceptanceCriteria: []string{"go version succeeds"},
		OutputFormat:       "unified_diff_only",
	}
}

func TestRunnerRejectsTaskWithTooLongDescription(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{}}
	runner := newTestRunner(root, provider)
	task := validTestTask()
	task.Description = strings.Repeat("x", 3000) // exceeds default 2000

	result := runner.Run(context.Background(), task)
	if result.Err == nil {
		t.Fatal("Run() error = nil, want TASK_TEXT_TOO_LARGE")
	}
	if !strings.Contains(result.TaskResult.FailureReason, "TASK_TEXT_TOO_LARGE") {
		t.Fatalf("failure reason = %q", result.TaskResult.FailureReason)
	}
}

func TestRunnerRejectsTaskWithTooManyRequirements(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{}}
	runner := newTestRunner(root, provider)
	task := validTestTask()
	reqs := make([]string, 60)
	for i := range reqs {
		reqs[i] = "x"
	}
	task.Requirements = reqs

	result := runner.Run(context.Background(), task)
	if result.Err == nil {
		t.Fatal("Run() error = nil, want TASK_TOO_BROAD")
	}
	if !strings.Contains(result.TaskResult.FailureReason, "TASK_TOO_BROAD") {
		t.Fatalf("failure reason = %q", result.TaskResult.FailureReason)
	}
}

func TestRunnerRejectsTaskWithTooLongAcceptanceCriteria(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{}}
	runner := newTestRunner(root, provider)
	task := validTestTask()
	task.AcceptanceCriteria = []string{strings.Repeat("y", 5000)} // exceeds default 4000

	result := runner.Run(context.Background(), task)
	if result.Err == nil {
		t.Fatal("Run() error = nil, want TASK_TEXT_TOO_LARGE")
	}
	if !strings.Contains(result.TaskResult.FailureReason, "TASK_TEXT_TOO_LARGE") {
		t.Fatalf("failure reason = %q", result.TaskResult.FailureReason)
	}
}

func TestRunnerRejectsTaskWithTooManyAcceptanceCriteria(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{}}
	runner := newTestRunner(root, provider)
	task := validTestTask()
	acc := make([]string, 25)
	for i := range acc {
		acc[i] = "x"
	}
	task.AcceptanceCriteria = acc

	result := runner.Run(context.Background(), task)
	if result.Err == nil {
		t.Fatal("Run() error = nil, want TASK_TOO_BROAD")
	}
	if !strings.Contains(result.TaskResult.FailureReason, "TASK_TOO_BROAD") {
		t.Fatalf("failure reason = %q", result.TaskResult.FailureReason)
	}
}

func TestRunnerRejectsTaskWithWideGlobPattern(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{}}
	runner := newTestRunner(root, provider)
	task := validTestTask()
	task.AllowedFiles = []string{"src/**", "internal/**"}

	result := runner.Run(context.Background(), task)
	if result.Err == nil {
		t.Fatal("Run() error = nil, want TASK_TOO_BROAD")
	}
	if !strings.Contains(result.TaskResult.FailureReason, "TASK_TOO_BROAD") {
		t.Fatalf("failure reason = %q", result.TaskResult.FailureReason)
	}
}

func TestRunnerRejectsTaskWithBroadKeyword(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{}}
	runner := newTestRunner(root, provider)
	task := validTestTask()
	task.Description = "完整实现零漂测试流程和页面"

	result := runner.Run(context.Background(), task)
	if result.Err == nil {
		t.Fatal("Run() error = nil, want TASK_TOO_BROAD")
	}
	if !strings.Contains(result.TaskResult.FailureReason, "TASK_TOO_BROAD") {
		t.Fatalf("failure reason = %q", result.TaskResult.FailureReason)
	}
}

func TestRunnerRejectsMultiDomainTask(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{}}
	runner := newTestRunner(root, provider)
	task := validTestTask()
	task.Description = "实现零漂UI页面、MES上传和Modbus协议通讯"

	result := runner.Run(context.Background(), task)
	if result.Err == nil {
		t.Fatal("Run() error = nil, want TASK_TOO_BROAD")
	}
	if !strings.Contains(result.TaskResult.FailureReason, "TASK_TOO_BROAD") {
		t.Fatalf("failure reason = %q", result.TaskResult.FailureReason)
	}
}

func TestRunnerPhaseReflectsRealFailureStage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{Content: "not a diff\n" + patch.EndMarker}}

	result := newTestRunner(root, provider).Run(context.Background(), validTestTask())
	if result.Err == nil {
		t.Fatal("Run() error = nil, want parse error")
	}
	if result.TaskResult.Phase != "patch_parse" {
		t.Fatalf("Phase = %q, want patch_parse", result.TaskResult.Phase)
	}
	if result.TaskResult.FailureCode != "PATCH_PARSE_FAILED" {
		t.Fatalf("FailureCode = %q, want PATCH_PARSE_FAILED", result.TaskResult.FailureCode)
	}
}

func TestRunnerAcceptsSmallSingleDomainTask(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{
		Content: strings.Join([]string{
			"diff --git a/main.txt b/main.txt",
			"--- a/main.txt",
			"+++ b/main.txt",
			"@@ -1 +1 @@",
			"-old",
			"+new",
			patch.EndMarker,
		}, "\n"),
	}}

	result := newTestRunner(root, provider).Run(context.Background(), validTestTask())
	if result.Err != nil {
		t.Fatalf("Run() error = %v, want nil", result.Err)
	}
	if result.TaskResult.Status != StateCompleted {
		t.Fatalf("status = %s, want completed", result.TaskResult.Status)
	}
}
