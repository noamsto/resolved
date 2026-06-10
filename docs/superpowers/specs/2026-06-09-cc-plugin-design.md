# `resolved` — Claude Code plugin

> Design spec. Status: approved 2026-06-09. Unit: the `resolved` Claude Code plugin (packaging the existing scan engine).

## Problem

The `resolved` engine is done: `resolved scan --json` emits a stable per-finding
schema, `--fail-on` gates the exit code, `--diff`/`--staged` scope the scan, and
chroma-based comment extraction covers ~250 languages. What's missing is a way to
*use it from inside a Claude Code session* — so that when you're working in a repo
and wondering whether a `// TODO(#42)` still points at an open issue, Claude can
answer without you leaving the editor or remembering a CLI invocation.

This is a packaging exercise, not an engine change. The reference implementation
is the sibling `agent-smith` plugin, which solved the identical problem (ship a CC
plugin wrapping a static, `CGO_ENABLED=0` Go binary that must land on PATH).
`resolved` is the simpler case: one binary, no duckdb dependency.

## Design

### 1. Packaging (self-marketplaced)

- `.claude-plugin/plugin.json` — `name: resolved`, a one-line description, and a
  `version` string. `version` is **load-bearing**: it drives the bootstrap's
  download URL and is bound by the version invariant below.
- `.claude-plugin/marketplace.json` — one plugin, `source: ./`, owner `noamsto`.
  Install path:
  1. `/plugin marketplace add noamsto/resolved`
  2. `/plugin install resolved@resolved`
  3. ask Claude about stale refs, or run `/stale`

The marketplace serves `main`, not tags.

### 2. Binary distribution (download the static binary)

The binary is pure Go (`CGO_ENABLED=0`) — static, cross-compiles trivially, no
runtime deps beyond `git` and a GitHub credential (see Assumptions). So the
plugin ships *no* binary and downloads a release asset on first use.

**`.goreleaser.yaml`** (extend the existing file):
- Already builds `resolved` for `{linux,darwin} × {amd64,arm64}`, CGO off, version
  stamped via `-X github.com/noamsto/resolved/internal/cli.version={{.Version}}`.
- Add a **pinned archive name template**:
  `name_template: "resolved_{{.Os}}_{{.Arch}}"`. Load-bearing — `bootstrap.sh`
  constructs the download URL from this exact pattern. Keep the default
  `checksums.txt`.

**`.github/workflows/release.yml`** (new):
- On `v*` tag push → `goreleaser/goreleaser-action`.
- **Version invariant guard:** the job fails if the tag (minus `v`) ≠
  `plugin.json`'s `version`. Tag ⇔ `plugin.json` ⇔ binary `resolved version`
  always agree.

**Version invariant.** Because the marketplace serves `main`, `plugin.json`'s
`version` only changes in the release commit, and the `vX.Y.Z` tag goes on exactly
that commit. *Corollary (agent-smith's shipping erratum):* `claude plugin update`
skips re-syncing content when installed and served versions match — so prompt/skill
edits do **not** reach installed plugins without a version bump + tag. Any content
change that must ship needs a patch release (binaries rebuilt with the new stamp
even if unchanged).

**`scripts/bootstrap.sh`** (new — adapted from agent-smith, single binary, no duckdb):
1. **Self-locating, no `${CLAUDE_PLUGIN_ROOT}`** (that var is only substituted in
   hooks/MCP configs, not skill/command markdown). Derive the plugin root from the
   script's own path (`$(dirname "$0")/..`) and read the expected version from
   `<root>/.claude-plugin/plugin.json`.
2. Materialize `${XDG_CACHE_HOME:-$HOME/.cache}/resolved/bin/resolved` ("`$BIN`"):
   - **PATH hit** at the expected version (`resolved version` — bare string) →
     symlink it into `$BIN` (nix/dev users short-circuit, no download). An
     unstamped `dev` build is **trusted** with a one-line note. A stamped-but-
     mismatched version warns and falls through to download.
   - **Cache miss/mismatch** → download `resolved_<os>_<arch>.tar.gz` from
     `releases/download/v<version>/` via `curl -fsSL` (no `gh` — it needs auth even
     for public repos), unpack, `chmod +x`, atomic `mv` into `$BIN`.
3. One `uname -s/-m` → goreleaser (`linux/darwin`, `amd64/arm64`) case-mapping block.
4. Race-safe (temp dir on the same fs, atomic `mv`), idempotent, quiet when
   satisfied (one `✓` line), actionable error on download failure. `shellcheck`-clean.
5. Prints `$BIN` on stdout (one line).

**Delta from agent-smith:** `resolved` exposes version as the `resolved version`
subcommand (bare string via cobra), not `--version`. The bootstrap probes
`resolved version` accordingly. Single binary, single archive, no duckdb branch.

### 3. Surfaces

Both surfaces share one recipe: **bootstrap → `resolved scan … --json` → summarize
findings → warn and offer to fix → wait for go-ahead.** The flow is *advisory* — it
never blocks a commit and never edits without confirmation. The skill is the single
source of truth for the recipe; the command delegates to it.

**`skills/check-stale-refs/SKILL.md`** — auto-trigger, **on-demand intent only**.
Frontmatter declares `allowed-tools: Bash, Read, Edit` (bootstrap+scan need Bash,
the fix step needs Read/Edit). Fires when the user is reasoning about stale/dead
issue references — "is this TODO still open?", "do any comments point at closed
issues?", "check my issue refs". No commit-lifecycle coupling. Recipe:
1. **Step zero:** run the plugin's `scripts/bootstrap.sh` — at `<base>/../../scripts/bootstrap.sh`
   from the skill's injected base directory (`.../cache/resolved/resolved/<version>/skills/check-stale-refs/`),
   `./scripts/bootstrap.sh` in a dev checkout, else
   `ls -t ~/.claude/plugins/cache/resolved/resolved/*/scripts/bootstrap.sh | head -1`.
   Capture stdout (one line) as `$BIN`. On failure, stop and show its error.
2. `PATH="$BIN:$PATH" resolved scan --json` over the **whole repo** by default (each
   Bash call is a fresh shell — the `PATH=` prefix is per-invocation).
   **Exit codes are data, not failure:** `0` = clean, `1` = findings matched the
   `--fail-on` tier (the *expected* signal — capture stdout and continue), `2` = real
   error (bad flag, scan failure, missing GitHub credential — stop and surface it).
   Invoke so a `1` exit doesn't abort the step and stdout is still captured.
3. Parse the JSON `findings`. Summarize as `file:line → <state> #<number> "<title>"`,
   grouped by tier (stale / closed). Lead with the `summary` counts. `findings` empty
   → say so and stop.
4. **Warn + offer to fix:** for stale/closed refs, offer to update or remove the
   comment, then wait for the user's go-ahead before editing. Never edits unprompted.

**`commands/stale.md`** — explicit, on-demand `/stale [paths...]`. Thin by
*delegation*, not duplication: a command is a prompt, so its body parses
`$ARGUMENTS` into a scope (paths and/or narrowing flags) and then **instructs Claude
to invoke the `check-stale-refs` skill with that scope** — the skill remains the one
place the bootstrap+scan+fix recipe (incl. exit-code handling) lives. Scope:
- **Default: whole-repo scan.** Staleness is independent of what you're touching, so
  the repo is the natural unit; `--staged`/`--diff` are speed/noise knobs, not the
  default.
- Optional `$ARGUMENTS` path args narrow the scan to those paths.
- Pass-through narrowing flags: `--diff <ref>`, `--staged`, `--fail-on`,
  `--keywords`, `--exclude`. When narrowing to "just my branch" for speed, scope via
  the merge-base with the default branch (`--diff "$(git rev-parse --abbrev-ref origin/HEAD)"`
  or the repo's actual default), never a hard-coded `main`.

The on-disk cache (`internal/cache`) keeps repeated whole-repo scans cheap, so
whole-repo-by-default is not expensive in practice.

### 4. README

Add an **Install** quick start (the three-step marketplace flow above; note the
binary auto-downloads on first use, only `git` + a GitHub credential assumed) and a
**Develop** section (`nix develop` / `go test` / `nix build`). Keep it secondary to
the existing engine docs.

## Assumptions

- `git` is available; the user is inside a git repo for `--diff`/`--staged`.
- A GitHub credential exists: `GITHUB_TOKEN`/`GH_TOKEN` env, else `gh auth token`.
  The plugin does **not** manage credentials — same bar as agent-smith. Missing
  credentials surface as the engine's existing error.

## Out of scope

- Windows, Homebrew tap, auto-update beyond version-pinned re-download.
- **Commit-blocking / `--fail-on` gating as a default behavior** — the plugin is
  advisory; `--fail-on` remains an opt-in pass-through flag, not a gate the skill
  enforces.
- A pre-commit / Stop hook trigger (considered and dropped: staleness isn't a
  per-commit event, so auto-firing on every commit is noise).
- Replacing the GitHub GraphQL client or the comment extractor.

## Alternatives considered

- **`--staged` default scope** — rejected: staleness is independent of the staged
  diff, so staged-only systematically *misses* the real stale refs (they live in
  files you aren't committing). Kept as an opt-in flag.
- **Stop/PreCompact hook trigger** — rejected: couples a repo-wide GitHub-API scan
  to the commit lifecycle for a problem that isn't commit-shaped.
- **`go install` for distribution** — rejected: requires a Go toolchain at install
  time; fails the "install plugin, ask, done" bar. The static release asset needs no
  toolchain.
- **Bundle the binary in the plugin repo** — rejected: bloats the marketplace
  checkout and can't match the host OS/arch; the release asset is per-platform.
- **Command self-contained (no delegation to skill)** — rejected here in favor of a
  single source of truth, since the skill and command run the identical recipe.
  (agent-smith kept its commands standalone, but it had no overlapping skill.)
