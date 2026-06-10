---
name: check-stale-refs
description: Use when the user asks whether code comments still reference open GitHub issues/PRs — "is this TODO still open?", "do any comments point at closed issues?", "check my stale issue refs", "are these references still valid?". Scans the repo with the `resolved` CLI and offers to fix stale references. Do NOT use for general code review or for issues unrelated to comment ⇄ issue staleness.
allowed-tools: Bash, Read, Edit
---

# Check stale issue/PR references

Scan the repo's code comments for GitHub issue/PR references that have gone
stale (the issue closed / the PR merged), then offer to fix them. Advisory:
never block a commit, never edit without the user's go-ahead.

## Step zero — check `resolved` is installed

This plugin uses the `resolved` CLI from your PATH; it never downloads a binary.
Confirm it's present:

    command -v resolved

If that prints nothing, stop and tell the user to install it, then re-run:

    nix profile install github:noamsto/resolved
    # or
    go install github.com/noamsto/resolved/cmd/resolved@latest
    # or, in Home Manager: programs.resolved.enable = true;

## Step 1 — scan

Run, over the **whole repo** by default, appending the exit code to stdout so a
non-zero scan exit is not surfaced as a failed tool call:

    resolved scan --json; echo "scan_exit=$?"

(The trailing `echo` makes the Bash call exit `0` regardless — the real scan exit
lands in the `scan_exit=` line, with the JSON above it.)

Read `scan_exit` — it is **data, not failure**:
- `0` — clean, no findings at/above the fail-on tier.
- `1` — findings matched the tier (default `stale`). This is the *expected*
  signal: parse the JSON above and continue.
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
