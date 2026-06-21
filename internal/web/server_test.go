package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/coding-bridge/internal/config"
)

func TestChooseStoredAPIKeyUsesSubmittedKey(t *testing.T) {
	got := chooseStoredAPIKey("sk-new", "sk-old", "DEEPSEEK_API_KEY")
	if got != "sk-new" {
		t.Fatalf("chooseStoredAPIKey() = %q, want submitted key", got)
	}
}

func TestChooseStoredAPIKeyPreservesExistingKey(t *testing.T) {
	for _, submitted := range []string{"", "sk-a****xyz"} {
		got := chooseStoredAPIKey(submitted, "sk-existing", "DEEPSEEK_API_KEY")
		if got != "sk-existing" {
			t.Fatalf("chooseStoredAPIKey(%q) = %q, want existing key", submitted, got)
		}
	}
}

func TestChooseStoredAPIKeyFallsBackToEnvironmentReference(t *testing.T) {
	got := chooseStoredAPIKey("", "", "DEEPSEEK_API_KEY")
	if got != "${DEEPSEEK_API_KEY}" {
		t.Fatalf("chooseStoredAPIKey() = %q, want environment reference", got)
	}
}

func TestConfigPageExposesExecutionCodexAndTokenSettings(t *testing.T) {
	for _, id := range []string{
		`id="patch-max-tokens"`,
		`id="temperature"`,
		`id="max-repair-attempts"`,
		`id="enforce-task-budgets"`,
		`id="max-task-files"`,
		`id="max-context-bytes"`,
		`id="max-patch-lines"`,
		`id="cli-enabled"`,
		`id="default-cli"`,
		`id="codex-fallback"`,
		`id="sharing-approved"`,
		`id="token-accounting"`,
		`id="baseline-ratio"`,
		`id="advanced-config"`,
		`id="active-config-path"`,
	} {
		if !strings.Contains(configPageHTML, id) {
			t.Fatalf("config page missing %s", id)
		}
	}
}

func TestHandleContextAPIReportsActiveProjectAndConfigPath(t *testing.T) {
	root := t.TempDir()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/context", nil)
	NewServer(root, 0).handleContextAPI(rec, req)
	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["project_root"] != root ||
		!strings.Contains(response["config_path"], `.coding-bridge`) ||
		!strings.Contains(response["config_path"], `config.yaml`) {
		t.Fatalf("context response = %#v", response)
	}
}

func TestHandleFullConfigSavePreservesMaskedAPIKey(t *testing.T) {
	root := t.TempDir()
	loader := config.NewLoader(root)
	cfg := config.DefaultConfig()
	ds := cfg.Providers.Configs["deepseek"]
	ds.APIKey = "sk-existing"
	cfg.Providers.Configs["deepseek"] = ds
	if err := loader.Save(cfg); err != nil {
		t.Fatal(err)
	}
	submitted, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	ds = submitted.Providers.Configs["deepseek"]
	ds.APIKey = "sk-e****ting"
	submitted.Providers.Configs["deepseek"] = ds
	submitted.Execution.PatchMaxTokens = 65536
	body, _ := json.Marshal(submitted)
	req := httptest.NewRequest(http.MethodPost, "/api/config/full-save", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	NewServer(root, 0).handleFullConfigSave(rec, req)
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("response = %s", rec.Body.String())
	}
	saved, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Providers.Configs["deepseek"].APIKey != "sk-existing" ||
		saved.Execution.PatchMaxTokens != 65536 {
		t.Fatalf("saved = %#v", saved)
	}
}

func TestHandleConfigSavePreservesUneditedConfigAndWritesCodexPolicy(t *testing.T) {
	root := t.TempDir()
	loader := config.NewLoader(root)
	cfg := config.DefaultConfig()
	cfg.Providers.Configs["deepseek"] = config.ProviderItemConfig{
		Type: "deepseek", BaseURL: "https://old.example", APIKey: "sk-existing",
		Models: []string{"old-model"}, Timeout: 90, MaxRetry: 1, Enabled: true,
	}
	cfg.Encoding.DefaultEncoding = "gbk"
	cfg.Security.ForbiddenFileReadPolicy.AuditLog = false
	if err := loader.Save(cfg); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(saveRequest{
		DeepSeekKey: "", DeepSeekModels: []string{"new-model"}, DeepSeekEnabled: true,
		DeepSeekBaseURL: "https://new.example", DeepSeekTimeout: 180, DeepSeekMaxRetry: 4,
		DefaultExecutor: "deepseek", SelectionMode: "auto", FallbackOrder: "deepseek,openai",
		AllowedCommands: "go test ./...", ForbiddenCmds: "rm -rf",
		CmdTimeout: 600, ReqTimeout: 240, PatchMaxTokens: 32768, Temperature: 0,
		MaxRepairAttempts:  2,
		EnforceTaskBudgets: true, MaxTaskFiles: 3, MaxContextBytes: 49152, MaxPatchLines: 200,
		CLIEnabled: true, DefaultCLI: true, CodexFallback: true, SharingApproved: true,
		TokenAccounting: true, BaselineRatio: 4, ReportMaxHistory: 250,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/config/save", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	NewServer(root, 0).handleConfigSave(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}

	saved, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Encoding.DefaultEncoding != "gbk" ||
		saved.Security.ForbiddenFileReadPolicy.AuditLog {
		t.Fatalf("unexposed settings were overwritten: %#v", saved)
	}
	ds := saved.Providers.Configs["deepseek"]
	if ds.APIKey != "sk-existing" || ds.BaseURL != "https://new.example" ||
		ds.Models[0] != "new-model" || ds.MaxRetry != 4 {
		t.Fatalf("deepseek config = %#v", ds)
	}
	if saved.Execution.PatchMaxTokens != 32768 || saved.Execution.Temperature != 0 ||
		saved.Execution.MaxRepairAttempts != 2 ||
		!saved.Execution.EnforceTaskBudgets ||
		saved.Execution.MaxTaskFiles != 3 ||
		saved.Execution.MaxContextBytes != 49152 ||
		saved.Execution.MaxPatchLines != 200 ||
		!saved.Codex.ExternalCodeSharingApproved ||
		saved.TokenAccounting.DirectCodexBaselineRatio != 4 {
		t.Fatalf("new settings were not saved: %#v", saved)
	}
	policy, err := os.ReadFile(root + `/.coding-bridge/codex-policy.json`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(policy), "sk-existing") ||
		!strings.Contains(string(policy), `"sharing_approved":true`) {
		t.Fatalf("policy = %s", policy)
	}
}
