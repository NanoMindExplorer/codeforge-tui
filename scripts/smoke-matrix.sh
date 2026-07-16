#!/usr/bin/env bash
# smoke-matrix.sh — Batch F automated smoke (color / flags don't break binary).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VER="$(tr -d '[:space:]' < VERSION)"
BIN="${SMOKE_BIN:-}"
if [[ -z "$BIN" ]]; then
  BIN=$(mktemp)
  trap 'rm -f "$BIN"' EXIT
  CGO_ENABLED=0 go build -ldflags="-s -w -X main.ProjectVersion=${VER}" -o "$BIN" ./cmd/codeforge/
fi

fail=0
run() {
  local label="$1"; shift
  if env "$@" "$BIN" version 2>/dev/null | grep -q "$VER"; then
    echo "  ✓ $label"
  else
    echo "  ✗ $label"
    fail=1
  fi
}

echo "Terminal matrix smoke (version only — no TUI):"
run "default" 
run "COLOR=256" CODEFORGE_COLOR=256
run "COLOR=16" CODEFORGE_COLOR=16
run "COLOR=none" CODEFORGE_COLOR=none
run "NO_COLOR" NO_COLOR=1
run "SSH_TUNE" CODEFORGE_SSH_TUNE=1
run "MINIMAL" CODEFORGE_MINIMAL=1
run "COMPACT" CODEFORGE_COMPACT=1
run "NO_MOTION" CODEFORGE_NO_MOTION=1
run "PLAIN_MD" CODEFORGE_PLAIN_MD=1

# help/about CLI must work without keys
if "$BIN" --help >/dev/null 2>&1; then
  echo "  ✓ --help"
else
  echo "  ✗ --help"
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "smoke-matrix OK"
exit 0
