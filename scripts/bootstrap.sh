#!/usr/bin/env bash
# Resolves the `resolved` binary into one bin dir and prints it. A PATH-found
# binary at the expected version is symlinked; otherwise it is downloaded from
# the matching GitHub release. Idempotent; safe to run on every invocation.
set -euo pipefail

RELEASE_BASE="${RESOLVED_DOWNLOAD_BASE:-https://github.com/noamsto/resolved/releases/download}"

plugin_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
expected="$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$plugin_root/.claude-plugin/plugin.json")"
if [ -z "$expected" ]; then
  echo "bootstrap: cannot read version from $plugin_root/.claude-plugin/plugin.json" >&2
  exit 1
fi

cache="${XDG_CACHE_HOME:-$HOME/.cache}/resolved"
bin="$cache/bin"
mkdir -p "$bin"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "bootstrap: unsupported OS $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "bootstrap: unsupported arch $(uname -m)" >&2; exit 1 ;;
esac

fetch() { # url dest
  curl -fsSL "$1" -o "$2" || {
    echo "bootstrap: download failed: $1" >&2
    echo "bootstrap: check network, or put resolved on PATH yourself (go install / nix build)" >&2
    exit 1
  }
}

need=0
found="$(command -v resolved 2>/dev/null || true)"
if [ -n "$found" ] && [ "$found" != "$bin/resolved" ]; then
  v="$("$found" version 2>/dev/null || true)"
  if [ "$v" = "$expected" ] || [ "$v" = "dev" ]; then
    [ "$v" = "dev" ] && echo "bootstrap: using local dev build of resolved ($found)" >&2
    ln -sf "$found" "$bin/resolved"
  else
    [ -n "$v" ] && echo "bootstrap: resolved on PATH is $v, want $expected — using release binary" >&2
    need=1
  fi
else
  v="$("$bin/resolved" version 2>/dev/null || true)"
  if [ -x "$bin/resolved" ] && { [ "$v" = "$expected" ] || [ "$v" = "dev" ]; }; then
    :
  else
    need=1
  fi
fi

if [ "$need" -eq 1 ]; then
  tmp="$(mktemp -d "$cache/tmp.XXXXXX")" # same fs as $bin → atomic mv
  trap 'rm -rf "$tmp"' EXIT
  echo "bootstrap: downloading resolved v$expected (${os}/${arch})" >&2
  fetch "$RELEASE_BASE/v$expected/resolved_${os}_${arch}.tar.gz" "$tmp/resolved.tar.gz"
  tar -xzf "$tmp/resolved.tar.gz" -C "$tmp"
  [ -f "$tmp/resolved" ] || { echo "bootstrap: release tarball missing resolved" >&2; exit 1; }
  chmod +x "$tmp/resolved"
  mv -f "$tmp/resolved" "$bin/resolved"
fi

echo "bootstrap: ✓ $expected" >&2
echo "$bin"
