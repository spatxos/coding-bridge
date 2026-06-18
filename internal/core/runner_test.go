package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coding-bridge/internal/config"
	"github.com/coding-bridge/internal/providers"
)

type fakeProvider struct {
	response *providers.GenerateResponse
	request  *providers.GenerateRequest
}

func (p *fakeProvider) Type() providers.ProviderType { return providers.ProviderDeepSeek }
func (p *fakeProvider) Name() string                 { return "fake" }
func (p *fakeProvider) Generate(_ context.Context, req *providers.GenerateRequest) (*providers.GenerateResponse, error) {
	p.request = req
	return p.response, nil
}
func (p *fakeProvider) HealthCheck(context.Context) (*providers.HealthCheckResult, error) {
	return &providers.HealthCheckResult{Status: providers.HealthHealthy}, nil
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
	if _, err := os.Stat(result.ReportPath); err != nil {
		t.Fatalf("report was not written: %v", err)
	}
}

func TestRunnerWritesFailureReport(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{response: &providers.GenerateResponse{Content: "not a diff"}}

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
