package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectRootUsesExplicitDirectory(t *testing.T) {
	root := t.TempDir()
	got, err := resolveProjectRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(root)
	if got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}

func TestResolveProjectRootDetectsParentCodingBridgeDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".coding-bridge"), 0755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "src", "child")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatal(err)
	}
	previous, _ := os.Getwd()
	defer os.Chdir(previous)
	if err := os.Chdir(child); err != nil {
		t.Fatal(err)
	}
	got, err := resolveProjectRoot("")
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("root = %q, want %q", got, root)
	}
}
