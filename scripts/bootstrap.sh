#!/usr/bin/env bash
# Resolves the `resolved` binary and prints the directory containing it. If it
# is not on PATH, prints install instructions and exits non-zero — the plugin
# never downloads or executes a fetched binary; the user installs it themselves.
set -euo pipefail

found="$(command -v resolved 2>/dev/null || true)"
if [ -z "$found" ]; then
  # printf is a shell builtin, so this works even with a near-empty PATH.
  printf '%s\n' \
    'bootstrap: resolved is not installed. Install it, then re-run:' \
    '  nix profile install github:noamsto/resolved' \
    '  go install github.com/noamsto/resolved/cmd/resolved@latest' \
    '  # or, in Home Manager, import the flake module: programs.resolved.enable = true;' >&2
  exit 1
fi

plugin_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
expected="$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$plugin_root/.claude-plugin/plugin.json")"
v="$("$found" version 2>/dev/null || true)"
if [ -n "$expected" ] && [ -n "$v" ] && [ "$v" != "$expected" ] && [ "$v" != "dev" ]; then
  echo "bootstrap: using resolved $v on PATH (plugin expects $expected)" >&2
fi

echo "bootstrap: ✓ resolved ${v:-?} ($found)" >&2
dirname "$found"
