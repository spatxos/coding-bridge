package cli

import "testing"

func TestResolveProviderAPIKeyUsesLiteralValue(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")

	got := resolveProviderAPIKey("deepseek", "sk-literal")
	if got != "sk-literal" {
		t.Fatalf("resolveProviderAPIKey() = %q, want literal key", got)
	}
}

func TestResolveProviderAPIKeyExpandsConfiguredEnvironmentVariable(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("CUSTOM_DEEPSEEK_KEY", "sk-from-custom-env")

	got := resolveProviderAPIKey("deepseek", "${CUSTOM_DEEPSEEK_KEY}")
	if got != "sk-from-custom-env" {
		t.Fatalf("resolveProviderAPIKey() = %q, want configured environment key", got)
	}
}

func TestResolveProviderAPIKeyPrefersConventionalEnvironmentVariable(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-from-standard-env")
	t.Setenv("CUSTOM_DEEPSEEK_KEY", "sk-from-custom-env")

	got := resolveProviderAPIKey("deepseek", "${CUSTOM_DEEPSEEK_KEY}")
	if got != "sk-from-standard-env" {
		t.Fatalf("resolveProviderAPIKey() = %q, want standard environment key", got)
	}
}
