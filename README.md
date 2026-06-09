# resolved

Scan a repository's code comments for GitHub issue/PR references and report the
ones that are **stale** — a closed/merged issue still referenced by a TODO/FIXME
comment. The CLI companion to [`resolved.nvim`](https://github.com/noamsto/resolved.nvim).

## Install

```bash
go install github.com/noamsto/resolved/cmd/resolved@latest
```

Requires a GitHub credential: `GITHUB_TOKEN`/`GH_TOKEN`, or `gh auth login`.

## Usage

```bash
resolved scan                # scan git-tracked files in the current repo
resolved scan path/ a.go     # scan specific paths
resolved scan --staged       # only staged files (pre-commit)
resolved scan --diff main    # only files changed vs main (CI)
resolved check #123          # status of one reference
resolved explore            # interactive TUI: browse findings, open issue/editor, refresh
```

Output is auto-detected: a TTY gets human output, a pipe gets JSON. Exit codes:
`0` clean, `1` stale found (configurable via `--fail-on`), `2` tool error.

Bare `#123` references (without an `owner/repo` prefix) are matched against the
origin repo only when you pass `--bare` (disabled by default — too noisy in
active repos). Full URLs and `owner/repo#n` forms are always resolved.

Comments are extracted with tree-sitter; supported languages: Go, Python,
JavaScript/JSX, TypeScript/TSX, Rust, Ruby, Java, C/C++, Bash. Files in other
languages are skipped and counted in the summary
(`(N skipped: unsupported language)`).

See `resolved scan --help` for all flags.

### explore (TUI)

`resolved explore` opens an interactive two-pane browser: a tier-sorted list of
references on the left (stale first, color-coded) and a detail pane on the right
showing the selected reference's title, state, location, URL, triggering keyword,
and the source comment line. Keys: `j`/`k` move, `enter` opens the issue/PR in
your browser, `e` opens the source line in `$EDITOR`, `y` copies `file:line` and
`Y` copies the issue URL to the clipboard (terminal OSC52 — works over SSH/tmux
with `set-clipboard on`), `r` re-scans (bypassing the cache), `q` quits. Requires
an interactive terminal.

Sorting: press `s` to cycle tier → by-file (grouped under file headers) → recency
(newest first). Theme it with `--theme mocha|latte|frappe|macchiato` (default
`mocha`, Catppuccin). The list shows tier-color-coded references (stale=red,
gone=peach, closed=muted, open=green); a wider terminal shows longer file paths.

Inside tmux, `explore` relaunches itself in a floating popup (90% × 90%) so the
TUI overlays your current pane instead of taking it over; closing the TUI returns
you to where you were. Pass `--no-popup` to run inline in the current pane.
