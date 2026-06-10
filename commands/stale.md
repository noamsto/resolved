---
description: Scan the repo for code comments that reference stale (closed/merged) GitHub issues or PRs. Whole-repo by default; pass paths or flags to narrow.
allowed-tools: Bash, Read, Edit
---

Run the **check-stale-refs** skill to scan this repo for stale GitHub issue/PR
references and offer to fix them. The skill owns the full recipe (check `resolved`
is installed → `resolved scan --json` → summarize → offer-fix, including scan
exit-code handling); your only job here is to translate `$ARGUMENTS` into the scan
scope and then invoke that skill.

**Scope from `$ARGUMENTS`** (default: whole repo — staleness is independent of
what you are touching, so the repo is the natural unit):

- Bare path args (e.g. `internal/ cmd/foo.go`) → pass as `resolved scan <paths>`.
- Flags pass straight through to `resolved scan`: `--diff <ref>`, `--staged`,
  `--fail-on stale|closed|any`, `--keywords ...`, `--exclude ...`.
- To narrow to "just my branch" for speed, pass `--diff <default-branch>`, which
  scans files that differ from `<ref>`. Resolve the default branch dynamically
  rather than hard-coding `main`: `--diff "$(git rev-parse --abbrev-ref origin/HEAD | sed 's@^origin/@@')"`
  (if `origin/HEAD` isn't set, name the branch explicitly).

If `$ARGUMENTS` is empty, scan the whole repo.
