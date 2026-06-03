package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPathsWalksDirs(t *testing.T) {
	dir := t.TempDir()
	must := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("a.go")
	must("b.go")

	got, err := expandPaths([]string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d files, want 2: %v", len(got), got)
	}
}

func TestExpandPathsExcludeGlob(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "keep.go"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "skip_test.go"), []byte("x"), 0o644)

	got, err := expandPaths([]string{dir}, []string{"*_test.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != "keep.go" {
		t.Fatalf("exclude failed: %v", got)
	}
}
