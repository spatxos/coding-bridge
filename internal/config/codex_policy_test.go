package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCodexPolicyWritesOnlyRoutingBooleans(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	provider := cfg.Providers.Configs["deepseek"]
	provider.APIKey = "secret-key"
	cfg.Providers.Configs["deepseek"] = provider
	cfg.Codex.ExternalCodeSharingApproved = true

	path, err := WriteCodexPolicy(root, cfg)
	if err != nil {
		t.Fatalf("WriteCodexPolicy() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "codex-policy.json" {
		t.Fatalf("path = %q", path)
	}
	if string(data) == "" || string(data) == "{}\n" {
		t.Fatalf("policy is empty: %q", data)
	}
	if string(data) != `{"cli_enabled":true,"default_cli":true,"fallback_to_codex":true,"sharing_approved":true,"default_provider":"deepseek","default_model":"deepseek-chat"}`+"\n" {
		t.Fatalf("policy = %q", data)
	}
	var policy CodexPolicyFile
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatal(err)
	}
}
