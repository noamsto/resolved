package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "t@t"},
		{"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func gitAdd(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "add", "-A")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func has(paths []string, base string) bool {
	for _, p := range paths {
		if filepath.Base(p) == base {
			return true
		}
	}
	return false
}

func TestResolveTargetsDirSkipsGitignored(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	write(t, dir, ".gitignore", "ignored.go\n")
	write(t, dir, "keep.go", "// hi\n")
	write(t, dir, "ignored.go", "// hi\n")
	gitAdd(t, dir)

	got, err := resolveTargets(dir, []string{dir}, false, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !has(got, "keep.go") {
		t.Fatalf("want keep.go in %v", got)
	}
	if has(got, "ignored.go") {
		t.Fatalf("ignored.go should be excluded: %v", got)
	}
}

func TestResolveTargetsExplicitFileBypassesGitignore(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	write(t, dir, ".gitignore", "ignored.go\n")
	write(t, dir, "ignored.go", "// hi\n")
	gitAdd(t, dir)

	got, err := resolveTargets(dir, []string{filepath.Join(dir, "ignored.go")}, false, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !has(got, "ignored.go") {
		t.Fatalf("explicit file should be kept: %v", got)
	}
}

func TestResolveTargetsNonRepoWalksAndSkipsGit(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.go", "x")
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, ".git"), "config", "junk")

	got, err := resolveTargets(dir, []string{dir}, false, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !has(got, "a.go") || len(got) != 1 {
		t.Fatalf("want only a.go: %v", got)
	}
}

func TestResolveTargetsExcludeGlob(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "keep.go", "x")
	write(t, dir, "skip_test.go", "x")

	got, err := resolveTargets(dir, []string{dir}, false, "", []string{"*_test.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !has(got, "keep.go") || has(got, "skip_test.go") {
		t.Fatalf("exclude failed: %v", got)
	}
}

func TestResolveTargetsDropsGeneratedAndVendored(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	write(t, dir, ".gitattributes", "gen.go linguist-generated=true\nvend.go linguist-vendored\n")
	write(t, dir, "gen.go", "// x")
	write(t, dir, "vend.go", "// x")
	write(t, dir, "keep.go", "// x")
	gitAdd(t, dir)

	got, err := resolveTargets(dir, []string{dir}, false, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !has(got, "keep.go") {
		t.Fatalf("want keep.go: %v", got)
	}
	if has(got, "gen.go") || has(got, "vend.go") {
		t.Fatalf("generated/vendored should be dropped: %v", got)
	}
}

func TestResolveTargetsPathWithSpaceSurvives(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	write(t, dir, "a b.go", "// x")
	gitAdd(t, dir)

	got, err := resolveTargets(dir, []string{dir}, false, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !has(got, "a b.go") {
		t.Fatalf("path with space should survive the -z round-trip: %v", got)
	}
}
