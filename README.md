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

See `resolved scan --help` for all flags.

### explore (TUI)

`resolved explore` opens an interactive two-pane browser: a tier-sorted list of
references on the left (stale first, color-coded) and a detail pane on the right
showing the selected reference's title, state, location, URL, triggering keyword,
and the source comment line. Keys: `j`/`k` move, `enter` opens the issue/PR in
your browser, `e` opens the source line in `$EDITOR`, `r` re-scans (bypassing the
cache), `q` quits. Requires an interactive terminal.
