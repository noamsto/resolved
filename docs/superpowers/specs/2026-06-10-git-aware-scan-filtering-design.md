# Git-aware scan filtering + parallel scan

**Date:** 2026-06-10
**Status:** Approved (design)

## Problem

`resolved scan` was slow on a monorepo and the user observed it scanning files it
shouldn't. Investigation of `internal/cli/targets.go` found two file-selection
paths with very different behavior:

| Path | When | Filtering |
|------|------|-----------|
| `git ls-files` | default, `--staged`, `--diff` (no path args) | git filters: tracked files only, so `.gitignore` is respected, submodules appear as a gitlink dir entry (dropped by the `IsDir()` check), nested-worktree files are never listed |
| `filepath.WalkDir` | explicit path args | **none** — walks everything: `.git/`, submodule contents, nested worktrees, gitignored build artifacts, plus `--exclude` base-name globs |

Three goals, confirmed with the user:

1. **Git-aware filtering** for directory args — respect `.gitignore`, skip `.git`,
   submodules, and nested worktrees (the raw walk leaks all of these).
2. **gitattributes skip** — drop `linguist-generated` / `linguist-vendored` files
   in every mode.
3. **Parallelize the scan** — the clean whole-repo scan was a *sequential* loop
   reading + lexing 4,268 tracked files. `.gitignore` filtering does not change
   that number (git already excludes ignored files); concurrency does.

## Architectural decision: git is the oracle

Rather than reimplement gitignore/submodule/worktree/gitattributes matching in Go
(extra deps, re-derives git's well-tested edge cases), defer to git itself:
`git ls-files` for selection and `git check-attr` for attributes. The two
selection paths converge on identical semantics, and we write almost no matching
logic. Outside a git repo we keep `filepath.WalkDir` as a dumb fallback.

## Design

### Component 1 — Git-aware target resolution (`internal/cli/targets.go`)

Resolve the git root once per run (`git -C dir rev-parse --show-toplevel`); a
non-nil error means "not a git repo" → fallback mode.

`expandPaths` changes only for **directory** inputs:

- **Directory arg, inside a git repo** → `git -C dir ls-files -- <arg>` (tracked
  files only). Output paths join with `dir`, matching the existing default path.
  This inherits gitignore, excludes `.git/`, lists submodules only as their
  gitlink (contents never recursed), and never lists nested-worktree files
  (separate index).
- **Explicit file arg** → kept verbatim, even if gitignored. Naming a file is
  intent. This is also how untracked *new* files stay reachable
  (`resolved scan new.go`) under the tracked-only directory rule.
- **Not in a git repo** → existing `filepath.WalkDir` fallback, plus skip any
  `.git` directory.

Decision: directory args are **tracked-only**. `--others --exclude-standard` was
rejected because `--others` can resurface nested-worktree directories as untracked
entries, weakening the exclusion guarantee that is an explicit goal. Cost: bulk
directory scans skip untracked new files; mitigated by explicit-file scanning.

The default / `--staged` / `--diff` paths are unchanged.

### Component 2 — gitattributes filter (`internal/cli/targets.go`)

A new `filterByAttrs(dir, paths)` runs on the final candidate list **when in a git
repo**: a single batched `git -C dir check-attr -z --stdin linguist-generated
linguist-vendored`, feeding all candidate paths NUL-delimited on stdin. `-z` keeps
paths with spaces/colons unambiguous. A path is dropped when either attribute
reports a value in `{set, true}` (linguist markers appear both ways). One
subprocess regardless of file count.

`binary` / `-text` is intentionally out of scope: supported-extension gating in
`detect.Supported` already handles binaries, and dropping on `-text` risks
surprising omissions. Easy to add later.

### Component 3 — Single post-filter

Unify filtering: every mode (default, staged, diff, args) builds a raw candidate
list, then one pass applies `excluded()` base-name globs **and** `filterByAttrs`.
This removes the current duplication between `expandPaths` and `filterExisting`.

### Component 4 — Parallel scan (`internal/engine/engine.go`)

Replace the sequential loop in `scanTargets` with `errgroup.WithContext` +
`g.SetLimit(runtime.NumCPU())`. Each target writes into a preallocated `results[i]`
slot (no mutex), recording one of `{scanned, skipped, unreadable}`. The launch
loop checks `ctx.Err()` so the first `detect.Comments` error short-circuits the
remaining queue. After `Wait`, flatten `results` in index order and tally counts
— output stays byte-for-byte deterministic (findings are emitted in target order;
they are not globally sorted before rendering).

Concurrency safety verified: chroma's `RegexLexer.maybeCompile()` is guarded by
`r.mu.Lock()`; post-compile `Tokenise` reads immutable rules into a fresh per-call
iterator; `Coalesce` is per-call; `lexers.Match` is a read-only registry lookup.
Concurrent `detect.Comments` on the shared per-extension lexer is race-free.

## Edge cases & limitations

- **Args assumed within the current repo.** The git root is resolved once from the
  process cwd. A directory arg pointing outside that repo (`../sibling`,
  `/other/repo`) makes `git ls-files -- <arg>` fail with "outside repository" and
  aborts the scan, where the old `filepath.WalkDir` would have walked it. Accepted:
  `resolved` is a repo-scoped tool.
- **check-attr pairing.** `git check-attr -z` emits NUL-separated triples
  (`path \0 attr \0 info \0`), and the echoed path is not guaranteed to byte-match
  the input. Results are paired to inputs **by order** (one triple-group per input
  path, in order), never by string-matching the echoed path.
- **Unreadable files.** A supported file that fails to read is counted as neither
  `scanned` nor `skipped` — preserving today's summary numbers. The parallel
  rewrite's per-target state must tally `unreadable → neither`.

## Out of scope

- No new third-party dependencies (`golang.org/x/sync` already vendored).
- Untracked-but-unignored files remain unscanned in directory args (consistent
  with default mode).
- No change to default/staged/diff selection.

## Testing

Test what we own; trust git for what git owns.

- `targets_test.go` (core — our logic):
  - directory arg in a repo skips a **gitignored** file but includes tracked ones;
  - an explicit **file** arg is kept even when gitignored;
  - a non-repo dir still walks via the fallback, skipping `.git`;
  - `linguist-generated` / `linguist-vendored` files (fixture `.gitattributes`)
    are dropped, and a path with a space survives the `-z` round-trip.
- Submodule / nested-worktree exclusion follows from delegating to `git ls-files`;
  cover it with **one** cheap local-git integration test (`git worktree add` into
  a temp repo, assert its files are absent) rather than re-testing git broadly.
- `engine_test.go`: existing tests pass unchanged (proves determinism); add one
  asserting findings order is stable across many targets, and one for an
  unreadable supported file (counted as neither scanned nor skipped).
