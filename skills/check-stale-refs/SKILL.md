---
name: check-stale-refs
description: Use when the user asks whether code comments still reference open GitHub issues/PRs — "is this TODO still open?", "do any comments point at closed issues?", "check my stale issue refs", "are these references still valid?". Scans the repo with the `resolved` CLI and offers to fix stale references. Do NOT use for general code review or for issues unrelated to comment ⇄ issue staleness.
allowed-tools: Bash, Read, Edit
---

# Check stale issue/PR references

Scan the repo's code comments for GitHub issue/PR references that have gone
stale (the issue closed / the PR merged), then offer to fix them. Advisory:
never block a commit, never edit without the user's go-ahead.

## Step zero — resolve the binary (always)

Run the plugin's `scripts/bootstrap.sh` and capture its stdout (one line) as
`$BIN`. Locate the script, in order:

1. `<base>/../../scripts/bootstrap.sh`, where `<base>` is this skill's injected
   base directory (`.../plugins/cache/resolved/resolved/<version>/skills/check-stale-refs/`).
2. `./scripts/bootstrap.sh` — when working inside a `resolved` dev checkout.
3. `ls -t ~/.claude/plugins/cache/resolved/resolved/*/scripts/bootstrap.sh | head -1`.

If bootstrap exits non-zero, stop and show its stderr — do not continue.

## Step 1 — scan

Run, over the **whole repo** by default:

    PATH="$BIN:$PATH" resolved scan --json

(Each Bash call is a fresh shell, so the `PATH=` prefix must be on the scan
command itself, not a separate `export`.)

**Exit codes are data, not failure:**
- `0` — clean, no findings at/above the fail-on tier.
- `1` — findings matched the tier (default `stale`). This is the *expected*
  signal: the JSON was still printed to stdout — read it and continue.
- `2` — real error (bad flag, scan failure, or no GitHub credential:
  `GITHUB_TOKEN`/`GH_TOKEN` or `gh auth login`). Stop and surface the message.

## Step 2 — summarize

Parse the JSON. It is `{ "summary": {...}, "findings": [...] }`; each finding has
`file`, `line`, `raw`, `owner`, `repo`, `number`, `state`, `title`, `tier`. If
`findings` is empty, say the repo is clean and stop. Otherwise lead with the
`summary` counts, then list findings grouped by `tier` in the order
stale → closed → gone → open → unknown (a header per non-empty tier), one finding
per line as `file:line  owner/repo#number  "title"` — matching the CLI's own
human report:

    stale (2)
      internal/foo.go:42  noamsto/resolved#123  "Title of the issue"

## Step 3 — offer to fix

For each actionable reference (tier `stale`, `closed`, or `gone`), offer to update
or remove the comment, and wait for the user's go-ahead before editing. Use the
finding's `raw` text to locate the exact comment; Read the file region first, then
Edit. Never edit unprompted.
