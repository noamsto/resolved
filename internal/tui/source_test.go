package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommentSnippetReadsLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.go")
	if err := os.WriteFile(p, []byte("package x\n// TODO see #1\nfunc y(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newSourceCache()
	got := c.snippet(p, 2)
	if !strings.Contains(got, "TODO see #1") {
		t.Fatalf("snippet = %q, want the line-2 comment", got)
	}
}

func TestCommentSnippetMissingFile(t *testing.T) {
	c := newSourceCache()
	if got := c.snippet("/no/such/file.go", 3); got != "(source unavailable)" {
		t.Fatalf("snippet = %q, want (source unavailable)", got)
	}
}

func TestCommentSnippetOutOfRange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.go")
	_ = os.WriteFile(p, []byte("one\n"), 0o644)
	c := newSourceCache()
	if got := c.snippet(p, 99); got != "(source unavailable)" {
		t.Fatalf("snippet = %q, want (source unavailable) for out-of-range line", got)
	}
}
