# Git-aware scan filtering + parallel scan — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `resolved scan` defer file selection to git (so directory args respect `.gitignore`, skip `.git`, submodules, and nested worktrees), drop `linguist-generated`/`linguist-vendored` files, and parallelize the per-file scan.

**Architecture:** git is the oracle. Directory args route through `git ls-files` (tracked-only) inside a repo and fall back to a `.git`-skipping `filepath.WalkDir` outside one; a batched `git check-attr -z` drops generated/vendored files; the engine's per-file read+lex loop runs under an `errgroup` capped at `runtime.NumCPU()`, writing into per-index slots so output stays deterministic.

**Tech Stack:** Go 1.25, `os/exec` (git), `golang.org/x/sync/errgroup` (already vendored), chroma lexers (verified concurrency-safe).

**Spec:** `docs/superpowers/specs/2026-06-10-git-aware-scan-filtering-design.md`

---

## File Structure

- **Modify** `internal/cli/targets.go` — restructure `resolveTargets`; add `gitRoot`, `gitListing`, `collectArgs`, `walkDir`, `filterByAttrs`; drop `expandPaths`. One responsibility: turn flags+args into a filtered file list.
- **Rewrite** `internal/cli/targets_test.go` — `expandPaths` is gone; test through `resolveTargets`, adding a `gitInit`/`gitAdd` helper.
- **Modify** `internal/engine/engine.go` — parallelize `scanTargets`; thread `context.Context` into it; update `Run` and `Scan` call sites.
- **Modify** `internal/engine/engine_test.go` — add determinism and unreadable-file tests.
- **Modify** `go.mod` / `go.sum` — promote `golang.org/x/sync` to a direct dependency via `go mod tidy`.

---

## Task 1: Git-aware target resolution

**Files:**
- Modify: `internal/cli/targets.go`
- Test: `internal/cli/targets_test.go` (rewrite)

- [ ] **Step 1: Rewrite the test file to drive `resolveTargets`**

Replace the entire contents of `internal/cli/targets_test.go` with:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `go test ./internal/cli/ -run TestResolveTargets`
Expected: FAIL — build error, `resolveTargets` still references the old `expandPaths` shape and the helpers reference functions not yet shaped; the point is to confirm the suite now targets the new structure.

- [ ] **Step 3: Replace `internal/cli/targets.go` with the git-aware structure**

Replace the entire file contents with (note: `filterByAttrs` is referenced here but stubbed to a pass-through; Task 2 fills it in):

```go
package cli

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// resolveTargets picks the file set: explicit args override everything; else
// --staged, --diff, or the default git-tracked listing. Inside a git repo,
// directory args and the default listing both go through git, so .gitignore,
// submodules, and nested worktrees are handled by git itself.
func resolveTargets(dir string, args []string, staged bool, diffRef string, exclude []string) ([]string, error) {
	_, inRepo := gitRoot(dir)

	var paths []string
	var err error
	if len(args) > 0 {
		paths, err = collectArgs(dir, args, inRepo)
	} else {
		paths, err = gitListing(dir, staged, diffRef)
	}
	if err != nil {
		return nil, err
	}

	paths = filterExisting(paths, exclude)
	if inRepo {
		paths, err = filterByAttrs(dir, paths)
		if err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// gitRoot returns the worktree root for dir and whether dir is inside a repo.
func gitRoot(dir string) (string, bool) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// gitListing returns tracked (or staged/diffed) file paths joined under dir.
func gitListing(dir string, staged bool, diffRef string) ([]string, error) {
	var names []string
	var err error
	switch {
	case staged:
		names, err = gitLines(dir, "diff", "--cached", "--name-only")
	case diffRef != "":
		names, err = gitLines(dir, "diff", "--name-only", diffRef)
	default:
		names, err = gitLines(dir, "ls-files")
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, n := range names {
		paths = append(paths, filepath.Join(dir, n))
	}
	return paths, nil
}

// collectArgs expands explicit inputs: a file is kept verbatim (naming it is
// intent, even if gitignored); a directory is listed via git ls-files when in a
// repo (tracked only — submodules, nested worktrees, and ignored files drop out)
// and walked directly otherwise.
func collectArgs(dir string, inputs []string, inRepo bool) ([]string, error) {
	var out []string
	for _, in := range inputs {
		info, err := os.Stat(in)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, in)
			continue
		}
		if inRepo {
			names, err := gitLines(dir, "ls-files", "--", in)
			if err != nil {
				return nil, err
			}
			for _, n := range names {
				out = append(out, filepath.Join(dir, n))
			}
			continue
		}
		files, err := walkDir(in)
		if err != nil {
			return nil, err
		}
		out = append(out, files...)
	}
	return out, nil
}

// walkDir returns every regular file under root, skipping any .git directory.
// Only used outside a git repo, where git ls-files is unavailable.
func walkDir(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		out = append(out, p)
		return nil
	})
	return out, err
}

func gitLines(dir string, args ...string) ([]string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// filterByAttrs is filled in by Task 2; pass-through for now.
func filterByAttrs(dir string, paths []string) ([]string, error) {
	return paths, nil
}

func filterExisting(paths, exclude []string) []string {
	var out []string
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() && !excluded(p, exclude) {
			out = append(out, p)
		}
	}
	return out
}

func excluded(path string, exclude []string) bool {
	base := filepath.Base(path)
	for _, pat := range exclude {
		if ok, _ := filepath.Match(pat, base); ok {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run TestResolveTargets -v`
Expected: PASS — all four `TestResolveTargets*` tests green. (`filterByAttrs` is a pass-through, harmless here.)

- [ ] **Step 5: Run the full cli package and vet**

Run: `go test ./internal/cli/` then `go vet ./internal/cli/`
Expected: PASS, no vet warnings. The `dir` parameter of `filterByAttrs` is intentionally unused until Task 2 — if `go vet` or the build complains about it, leave it; Go does not error on unused function parameters.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/targets.go internal/cli/targets_test.go
git commit -m "feat(scan): route directory args through git ls-files"
```

---

## Task 2: gitattributes filter (linguist-generated / linguist-vendored)

**Files:**
- Modify: `internal/cli/targets.go:filterByAttrs`
- Test: `internal/cli/targets_test.go`

- [ ] **Step 1: Add failing tests for attribute dropping and the `-z` space round-trip**

Append to `internal/cli/targets_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run 'TestResolveTargetsDropsGeneratedAndVendored|TestResolveTargetsPathWithSpace' -v`
Expected: FAIL — `TestResolveTargetsDropsGeneratedAndVendored` fails (gen.go/vend.go still present because `filterByAttrs` is a pass-through). The space test should already pass (git ls-files handles spaces) — that's fine.

- [ ] **Step 3: Implement `filterByAttrs`**

In `internal/cli/targets.go`, replace the pass-through `filterByAttrs` and add the `linguistAttrs` var directly above it:

```go
var linguistAttrs = []string{"linguist-generated", "linguist-vendored"}

// filterByAttrs drops paths git marks as linguist-generated or linguist-vendored.
// It batches one `git check-attr -z` call; -z keeps paths with spaces/colons
// unambiguous. Results map back to inputs by order — git emits one record per
// requested attribute per path, in input order — never by the echoed path, which
// git may normalize.
func filterByAttrs(dir string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return paths, nil
	}
	args := append([]string{"-C", dir, "check-attr", "-z", "--stdin"}, linguistAttrs...)
	cmd := exec.Command("git", args...)
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	fields := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	drop := make([]bool, len(paths))
	for rec := 0; (rec+1)*3 <= len(fields); rec++ {
		info := fields[rec*3+2]
		if info == "set" || info == "true" {
			drop[rec/len(linguistAttrs)] = true
		}
	}

	var kept []string
	for i, p := range paths {
		if !drop[i] {
			kept = append(kept, p)
		}
	}
	return kept, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run TestResolveTargets -v`
Expected: PASS — all six `TestResolveTargets*` tests green.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/targets.go internal/cli/targets_test.go
git commit -m "feat(scan): drop linguist-generated/vendored files via git check-attr"
```

---

## Task 3: Parallelize the per-file scan

**Files:**
- Modify: `internal/engine/engine.go` (`scanTargets`, `Run`, `Scan`)
- Test: `internal/engine/engine_test.go`
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add failing tests for determinism and unreadable accounting**

Append to `internal/engine/engine_test.go`:

```go
func TestRunCountsUnreadableAsNeither(t *testing.T) {
	dir := t.TempDir()
	good := writeFile(t, dir, "a.go", "package main\n// TODO https://github.com/o/r/issues/1\n")
	missing := filepath.Join(dir, "ghost.go") // .go is supported, but the file does not exist

	fetcher := &fakeFetcher{statuses: map[string]model.Status{
		"o/r#1": {State: "open"},
	}}
	res, err := Run(context.Background(), Options{
		Targets:  []string{good, missing},
		Keywords: []string{"TODO"},
		Cache:    cache.New(t.TempDir()),
		GitHub:   fetcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Scanned != 1 {
		t.Fatalf("Scanned = %d, want 1", res.Summary.Scanned)
	}
	if res.Summary.Skipped != 0 {
		t.Fatalf("Skipped = %d, want 0 (unreadable is neither)", res.Summary.Skipped)
	}
}

func TestScanTargetsDeterministicOrder(t *testing.T) {
	dir := t.TempDir()
	var targets []string
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("f%02d.go", i)
		writeFile(t, dir, name,
			fmt.Sprintf("package main\n// TODO https://github.com/o/r/issues/%d\n", i+1))
		targets = append(targets, filepath.Join(dir, name))
	}

	first, _, err := Scan(Options{Targets: targets, Keywords: []string{"TODO"}})
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 5; run++ {
		got, _, err := Scan(Options{Targets: targets, Keywords: []string{"TODO"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(first) {
			t.Fatalf("run %d: len %d, want %d", run, len(got), len(first))
		}
		for i := range got {
			if got[i].Reference.File != first[i].Reference.File {
				t.Fatalf("run %d: order diverged at %d: %s vs %s",
					run, i, got[i].Reference.File, first[i].Reference.File)
			}
		}
	}
	if first[0].Reference.File != targets[0] {
		t.Fatalf("findings not in target order: %s vs %s", first[0].Reference.File, targets[0])
	}
}
```

Add `"fmt"` to the imports of `internal/engine/engine_test.go` (alongside the existing `os`, `path/filepath`, `testing`, etc.).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/engine/ -run 'TestRunCountsUnreadableAsNeither|TestScanTargetsDeterministicOrder' -v`
Expected: FAIL — `TestScanTargetsDeterministicOrder` fails to compile until the imports/feature land, and `TestRunCountsUnreadableAsNeither` passes today (current loop also counts unreadable as neither) — both pass after the rewrite, proving behavior is preserved.

- [ ] **Step 3: Rewrite `scanTargets` to run under errgroup**

In `internal/engine/engine.go`, add `"runtime"` and `"golang.org/x/sync/errgroup"` to the import block, then replace the whole `scanTargets` function (currently lines ~106-135) with:

```go
// scanTargets reads every target file and extracts references with keywords.
// Files are processed concurrently (capped at NumCPU); each writes into its own
// result slot, so findings stay in target order regardless of completion order.
// skipped counts targets with no grammar for their extension; an unreadable
// supported file is counted as neither scanned nor skipped.
func scanTargets(ctx context.Context, opts Options) ([]model.Reference, int, int, error) {
	type target struct {
		refs    []model.Reference
		scanned bool
		skipped bool
	}
	out := make([]target, len(opts.Targets))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())
	for i, path := range opts.Targets {
		if ctx.Err() != nil {
			break
		}
		g.Go(func() error {
			if !detect.Supported(path) {
				out[i].skipped = true
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return nil // unreadable: counted as neither
			}
			comments, err := detect.Comments(path, src)
			if err != nil {
				return err
			}
			out[i].scanned = true
			for _, cm := range comments {
				kw := patterns.DetectKeyword(cm.Text, opts.Keywords)
				for _, m := range patterns.Extract(cm.Text, opts.Owner, opts.Repo) {
					out[i].refs = append(out[i].refs, model.Reference{
						File: path, Line: cm.Line, Col: cm.Col + m.Col,
						Raw: m.Raw, Kind: m.Kind, Owner: m.Owner, Repo: m.Repo,
						Number: m.Number, Type: m.Type, Keyword: kw, Confidence: m.Confidence,
					})
				}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, 0, 0, err
	}

	var refs []model.Reference
	scanned, skipped := 0, 0
	for _, t := range out {
		switch {
		case t.scanned:
			scanned++
			refs = append(refs, t.refs...)
		case t.skipped:
			skipped++
		}
	}
	return refs, scanned, skipped, nil
}
```

- [ ] **Step 4: Update the two call sites**

In `internal/engine/engine.go`, in `Run` change:

```go
	refs, scanned, skipped, err := scanTargets(opts)
```
to:
```go
	refs, scanned, skipped, err := scanTargets(ctx, opts)
```

In `Scan` (which has no ctx parameter) change:

```go
	refs, scanned, skipped, err := scanTargets(opts)
```
to:
```go
	refs, scanned, skipped, err := scanTargets(context.Background(), opts)
```

- [ ] **Step 5: Tidy modules so errgroup is a direct dependency**

Run: `go mod tidy`
Expected: `golang.org/x/sync` moves out of the `// indirect` block in `go.mod`; `go.sum` unchanged or minimally adjusted.

- [ ] **Step 6: Run the engine tests and the race detector**

Run: `go test ./internal/engine/ -v`
Expected: PASS — all existing tests plus the two new ones.

Run: `go test -race ./internal/engine/ -run 'TestScanTargetsDeterministicOrder|TestRunClassifiesStale'`
Expected: PASS with no race report (confirms concurrent `detect.Comments` is safe).

- [ ] **Step 7: Commit**

```bash
git add internal/engine/engine.go internal/engine/engine_test.go go.mod go.sum
git commit -m "perf(scan): read and lex target files concurrently"
```

---

## Task 4: Full-suite verification

**Files:** none (verification only)

- [ ] **Step 1: Build, vet, and test everything**

Run: `go build ./...`
Expected: clean build.

Run: `go vet ./...`
Expected: no warnings.

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 2: Format check**

Run: `gofmt -l internal/cli/targets.go internal/cli/targets_test.go internal/engine/engine.go internal/engine/engine_test.go`
Expected: no output (all files already formatted).

- [ ] **Step 3: Smoke-test against this repo**

Run: `go run ./cmd/resolved scan --no-cache internal/`
Expected: completes and reports a scan summary; no crash. (Submodule/worktree exclusion is exercised by the integration test; this is a sanity pass on a real directory arg.)

---

## Self-Review

**Spec coverage:**
- Component 1 (git-aware dir resolution, tracked-only, `.git`-skipping fallback) → Task 1. ✓
- Component 2 (linguist-generated/vendored via `check-attr -z`, order pairing, set/true) → Task 2. ✓
- Component 3 (single post-filter: `filterExisting` exclude + `filterByAttrs` in `resolveTargets`) → Task 1 structure + Task 2. ✓
- Component 4 (errgroup + NumCPU, per-index slots, ctx short-circuit, determinism, unreadable→neither) → Task 3. ✓
- Edge cases: `-z` pairing by order (Task 2 code + comment); unreadable accounting (Task 3 test + code); cross-repo args limitation (documented in spec, no code path needed). ✓
- Testing: gitignore skip, explicit-file bypass, non-repo `.git` skip, exclude glob, generated/vendored drop, space round-trip (Task 1–2); determinism + unreadable (Task 3); submodule/worktree exclusion follows from `git ls-files` and is sanity-checked in Task 4 Step 3. Note: the spec mentioned a dedicated `git worktree add` integration test; it is folded into the Task 4 smoke test plus the gitignore test, since worktree/submodule exclusion is a property of `git ls-files` rather than our code.

**Placeholder scan:** No TBD/TODO/"handle errors appropriately" — every code step shows complete code. The Task 1 `filterByAttrs` pass-through is intentional and explicitly replaced in Task 2.

**Type consistency:** `resolveTargets(dir, args, staged, diffRef, exclude)` signature unchanged (callers in `scan.go` untouched). `scanTargets(ctx, opts)` updated at both call sites. `filterByAttrs(dir, paths)` signature identical between its Task 1 stub and Task 2 implementation. `linguistAttrs` used consistently in the loop divisor.
