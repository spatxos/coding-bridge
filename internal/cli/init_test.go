package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCodingBridgeGitignoreCreatesFile(t *testing.T) {
	root := t.TempDir()

	added, err := ensureCodingBridgeGitignore(root)
	if err != nil {
		t.Fatalf("ensureCodingBridgeGitignore() error = %v", err)
	}
	if !added {
		t.Fatal("added = false, want true")
	}
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "/.coding-bridge/\n" {
		t.Fatalf(".gitignore = %q", data)
	}
}

func TestEnsureCodingBridgeGitignorePreservesContentAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(path, []byte("bin/\r\nobj/\r\n"), 0644); err != nil {
		t.Fatal(err)
	}

	added, err := ensureCodingBridgeGitignore(root)
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatal("first call did not add the rule")
	}
	added, err = ensureCodingBridgeGitignore(root)
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Fatal("second call added a duplicate rule")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "bin/\r\nobj/\r\n") {
		t.Fatalf("existing content was not preserved: %q", text)
	}
	if strings.Count(text, "/.coding-bridge/") != 1 {
		t.Fatalf("rule count = %d, want 1", strings.Count(text, "/.coding-bridge/"))
	}
	if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
		t.Fatalf("CRLF line endings were not preserved: %q", text)
	}
}

func TestEnsureCodingBridgeGitignoreAcceptsExistingEquivalentRule(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(path, []byte(".coding-bridge/\n"), 0644); err != nil {
		t.Fatal(err)
	}

	added, err := ensureCodingBridgeGitignore(root)
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Fatal("equivalent existing rule should not be duplicated")
	}
}
