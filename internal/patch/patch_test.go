package patch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatorRejectsPathTraversal(t *testing.T) {
	result := &ParseResult{
		Files: []DiffFile{{
			OrigPath: "../outside.txt",
			NewPath:  "../outside.txt",
		}},
	}

	errs := NewValidator([]string{"../outside.txt"}, nil, nil).Validate(result)
	if len(errs) == 0 {
		t.Fatal("Validate() accepted a path outside the project")
	}
}

func TestApplierRejectsContextMismatchWithoutWriting(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.txt")
	original := "first\nsecond\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewParser().Parse(strings.Join([]string{
		"diff --git a/main.txt b/main.txt",
		"--- a/main.txt",
		"+++ b/main.txt",
		"@@ -1,2 +1,2 @@",
		" first",
		"-wrong",
		"+changed",
	}, "\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if _, err := NewApplier(root).Apply(result); err == nil {
		t.Fatal("Apply() accepted a mismatched hunk")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("file changed after rejected patch: %q", data)
	}
}

func TestApplierPreservesCRLF(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.txt")
	if err := os.WriteFile(path, []byte("first\r\nsecond\r\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewParser().Parse(strings.Join([]string{
		"diff --git a/main.txt b/main.txt",
		"--- a/main.txt",
		"+++ b/main.txt",
		"@@ -1,2 +1,2 @@",
		" first",
		"-second",
		"+changed",
	}, "\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if _, err := NewApplier(root).Apply(result); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first\r\nchanged\r\n" {
		t.Fatalf("content = %q, want CRLF content", data)
	}
}
