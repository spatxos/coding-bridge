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

func TestParserExtractsDiffFromMarkdownFenceAndCommentary(t *testing.T) {
	response := strings.Join([]string{
		"Here is the requested patch:",
		"```diff",
		"diff --git a/main.txt b/main.txt",
		"--- a/main.txt",
		"+++ b/main.txt",
		"@@ -1 +1 @@",
		"-old",
		"+new",
		"```",
		"Done.",
	}, "\n")

	result, err := NewParser().Parse(response)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(result.Files) != 1 || result.Files[0].OrigPath != "main.txt" {
		t.Fatalf("parsed files = %#v", result.Files)
	}
	if strings.Contains(result.RawDiff, "Here is") || strings.Contains(result.RawDiff, "```") {
		t.Fatalf("RawDiff still contains wrapper text: %q", result.RawDiff)
	}
}

func TestHasEndMarkerRecognizesPlainAndMarkdownWrappedOutput(t *testing.T) {
	for _, response := range []string{
		"diff --git a/a b/a\n" + EndMarker,
		"```diff\ndiff --git a/a b/a\n```\n" + EndMarker,
		"```diff\ndiff --git a/a b/a\n" + EndMarker + "\n```",
	} {
		if !HasEndMarker(response) {
			t.Fatalf("HasEndMarker(%q) = false", response)
		}
	}
	if HasEndMarker("diff --git a/a b/a") {
		t.Fatal("HasEndMarker accepted output without marker")
	}
}

func TestApplierRelocatesHunkWhenHeaderLineIsWrong(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.txt")
	original := "zero\nfirst\nsecond\nthird\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewParser().Parse(strings.Join([]string{
		"diff --git a/main.txt b/main.txt",
		"--- a/main.txt",
		"+++ b/main.txt",
		"@@ -20,2 +20,2 @@",
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
	if string(data) != "zero\nfirst\nchanged\nthird\n" {
		t.Fatalf("content = %q", data)
	}
}

func TestApplierRejectsAmbiguousRelocatedHunk(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.txt")
	original := "same\nold\nsame\nold\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewParser().Parse(strings.Join([]string{
		"diff --git a/main.txt b/main.txt",
		"--- a/main.txt",
		"+++ b/main.txt",
		"@@ -20,2 +20,2 @@",
		" same",
		"-old",
		"+new",
	}, "\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if _, err := NewApplier(root).Apply(result); err == nil ||
		!strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("Apply() error = %v, want ambiguous relocation error", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("file changed after rejected patch: %q", data)
	}
}

func TestParserNormalizesIncorrectHunkLineCounts(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.txt")
	if err := os.WriteFile(path, []byte("first\nsecond\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewParser().Parse(strings.Join([]string{
		"diff --git a/main.txt b/main.txt",
		"--- a/main.txt",
		"+++ b/main.txt",
		"@@ -1,99 +1,77 @@",
		" first",
		"-second",
		"+changed",
	}, "\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	hunk := result.Files[0].Hunks[0]
	if hunk.OrigLines != 2 || hunk.NewLines != 2 {
		t.Fatalf("normalized counts = -%d +%d, want -2 +2", hunk.OrigLines, hunk.NewLines)
	}
	if _, err := NewApplier(root).Apply(result); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first\nchanged\n" {
		t.Fatalf("content = %q", data)
	}
}

func TestApplierTreatsWhitespaceOnlyLinesAsEquivalent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.txt")
	if err := os.WriteFile(path, []byte("before\n\nafter\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewParser().Parse(strings.Join([]string{
		"diff --git a/main.txt b/main.txt",
		"--- a/main.txt",
		"+++ b/main.txt",
		"@@ -1,3 +1,3 @@",
		" before",
		"-        ",
		"+changed",
		" after",
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
	if string(data) != "before\nchanged\nafter\n" {
		t.Fatalf("content = %q", data)
	}
}

func TestParserRejectsMalformedHunkHeaderInsteadOfSucceedingWithNoChanges(t *testing.T) {
	_, err := NewParser().Parse(strings.Join([]string{
		"diff --git a/main.txt b/main.txt",
		"--- a/main.txt",
		"+++ b/main.txt",
		"@@ -xx,yy +xx,yy @@",
		"-old",
		"+new",
	}, "\n"))
	if err == nil || !strings.Contains(err.Error(), "invalid hunk header") {
		t.Fatalf("Parse() error = %v, want invalid hunk header", err)
	}
}

func TestParserRejectsExistingFileDiffWithoutHunks(t *testing.T) {
	_, err := NewParser().Parse(strings.Join([]string{
		"diff --git a/main.txt b/main.txt",
		"--- a/main.txt",
		"+++ b/main.txt",
	}, "\n"))
	if err == nil || !strings.Contains(err.Error(), "contains no valid hunks") {
		t.Fatalf("Parse() error = %v, want no valid hunks", err)
	}
}
