package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectorSanitizesAndSkipsForbiddenFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\napi_key = super-secret\n")
	writeTestFile(t, filepath.Join(root, ".env"), "TOKEN=must-not-leak\n")

	collector := NewCollector(root, []string{"main.go", ".env"}, nil)
	collected, err := collector.Collect()
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if collected.TotalFiles != 1 {
		t.Fatalf("TotalFiles = %d, want 1", collected.TotalFiles)
	}
	if collected.SkippedFiles != 1 {
		t.Fatalf("SkippedFiles = %d, want 1", collected.SkippedFiles)
	}
	if strings.Contains(collected.Files[0].Content, "super-secret") {
		t.Fatal("collected context contains an unmasked API key")
	}
	if !strings.Contains(collected.Files[0].Content, "[REDACTED]") {
		t.Fatal("collected context does not contain the redaction marker")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
