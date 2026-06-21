package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coding-bridge/internal/config"
)

func TestInstallCodexInstructionsCreatesAgentsFile(t *testing.T) {
	root := t.TempDir()

	agentsPath, created, err := installCodexInstructions(root, config.DefaultConfig())
	if err != nil {
		t.Fatalf("installCodexInstructions() error = %v", err)
	}
	if !created {
		t.Fatal("created = false, want true")
	}
	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), codexBlockStart) {
		t.Fatal("AGENTS.md does not contain the coding-bridge block")
	}
	if !strings.Contains(string(content), "not a Codex plugin") {
		t.Fatal("AGENTS.md does not explicitly identify coding-bridge as a local CLI")
	}
	if !strings.Contains(string(content), "Never use tool discovery or plugin installation") {
		t.Fatal("AGENTS.md does not prohibit plugin discovery")
	}
	if _, err := os.Stat(filepath.Join(root, ".coding-bridge", "tasks")); err != nil {
		t.Fatalf("tasks directory was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".coding-bridge", "codex-policy.json")); err != nil {
		t.Fatalf("Codex policy was not created: %v", err)
	}
}

func TestInstallCodexInstructionsPreservesAndUpdatesExistingFile(t *testing.T) {
	root := t.TempDir()
	agentsPath := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("# Existing rules\n\nKeep this.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, created, err := installCodexInstructions(root, config.DefaultConfig()); err != nil {
		t.Fatal(err)
	} else if created {
		t.Fatal("created = true, want false")
	}
	if _, _, err := installCodexInstructions(root, config.DefaultConfig()); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "Keep this.") {
		t.Fatal("existing AGENTS.md content was lost")
	}
	if strings.Count(text, codexBlockStart) != 1 {
		t.Fatalf("managed block count = %d, want 1", strings.Count(text, codexBlockStart))
	}
}

func TestUpsertCodexBlockRejectsBrokenMarkers(t *testing.T) {
	_, err := upsertCodexBlock(codexBlockStart+"\nbroken", config.DefaultConfig())
	if err == nil {
		t.Fatal("upsertCodexBlock() accepted incomplete markers")
	}
}

func TestCodexInstructionsUseSmallNonSecretPolicyFile(t *testing.T) {
	block := codexInstructionsBlock(config.DefaultConfig())
	if !strings.Contains(block, ".coding-bridge/codex-policy.json") {
		t.Fatal("managed instructions do not use the compact policy file")
	}
	if strings.Contains(block, "model in `.coding-bridge/config.yaml`") {
		t.Fatal("managed instructions use the credential-bearing config for model routing")
	}
}
