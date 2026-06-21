package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type CodexPolicyFile struct {
	CLIEnabled      bool   `json:"cli_enabled"`
	DefaultCLI      bool   `json:"default_cli"`
	FallbackToCodex bool   `json:"fallback_to_codex"`
	SharingApproved bool   `json:"sharing_approved"`
	DefaultProvider string `json:"default_provider"`
	DefaultModel    string `json:"default_model"`
}

func WriteCodexPolicy(projectRoot string, cfg *AppConfig) (string, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	path := filepath.Join(projectRoot, ".coding-bridge", "codex-policy.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("create coding-bridge directory: %w", err)
	}
	defaultProvider := cfg.Providers.DefaultExecutor
	defaultModel := ""
	if provider, ok := cfg.Providers.Configs[defaultProvider]; ok {
		models := provider.GetModels()
		if len(models) > 0 {
			defaultModel = models[0]
		}
	}
	data, err := json.Marshal(CodexPolicyFile{
		CLIEnabled:      cfg.Codex.CLIEnabled,
		DefaultCLI:      cfg.Codex.DefaultCLIForCodingTasks,
		FallbackToCodex: cfg.Codex.FallbackToCodexOnUnavailable,
		SharingApproved: cfg.Codex.ExternalCodeSharingApproved,
		DefaultProvider: defaultProvider,
		DefaultModel:    defaultModel,
	})
	if err != nil {
		return "", fmt.Errorf("marshal Codex policy: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return "", fmt.Errorf("write Codex policy: %w", err)
	}
	return path, nil
}
