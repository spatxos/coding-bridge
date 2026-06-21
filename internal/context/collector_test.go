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

func TestCollectorRejectsCodingBridgeInternalPaths(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
	}{
		{"reports path", ".coding-bridge/reports/task/xxx.md"},
		{"backups path", ".coding-bridge/backups/a.cs"},
		{"snapshots path", ".coding-bridge/snapshots/snapshot.json"},
		{"tasks path", ".coding-bridge/tasks/task.json"},
		{"top-level .coding-bridge", ".coding-bridge"},
		{"node_modules", "node_modules/pkg/file.js"},
		{".git path", ".git/config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			collector := NewCollector(root, []string{tt.pattern}, nil)
			_, err := collector.Collect()
			if err == nil {
				t.Fatalf("Collect() error = nil, want FORBIDDEN_INTERNAL_CONTEXT for %q", tt.pattern)
			}
			if !strings.Contains(err.Error(), ErrForbiddenInternalContext) {
				t.Fatalf("error = %v, want FORBIDDEN_INTERNAL_CONTEXT", err)
			}
		})
	}
}

func TestIsInternalStatePath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{".coding-bridge/reports/x.md", true},
		{".coding-bridge/backups/a.cs", true},
		{".coding-bridge/snapshots/s.json", true},
		{".coding-bridge/tasks/t.json", true},
		{".coding-bridge/config.yaml", true},
		{"src/main.cs", false},
		{"README.md", false},
		{"main.go", false},
		{".git/config", true},
		{"node_modules/pkg/index.js", true},
		{"bin/output.exe", true},
		{"dist/bundle.js", true},
		{"build/output", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := IsInternalStatePath(tt.path)
			if result != tt.expected {
				t.Fatalf("IsInternalStatePath(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}
