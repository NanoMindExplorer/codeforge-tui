#!/usr/bin/env bash
# check-version.sh — single source of truth for CodeForge version strings.
# Exit 0 if VERSION, main.go, README, about text, MCP client, and Formula agree.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ ! -f VERSION ]]; then
  echo "ERROR: VERSION file missing"
  exit 1
fi

VER="$(tr -d '[:space:]' < VERSION)"
if [[ ! "$VER" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-].*)?$ ]]; then
  echo "ERROR: VERSION looks invalid: $VER"
  exit 1
fi

fail=0
check() {
  local file="$1" pattern="$2" label="${3:-$file}"
  if [[ ! -f "$file" ]]; then
    echo "WARN: missing $file (skip)"
    return 0
  fi
  if ! grep -qE "$pattern" "$file"; then
    echo "FAIL: $label — expected version $VER (pattern: $pattern)"
    fail=1
  else
    echo "OK:   $label"
  fi
}

echo "Checking version consistency for $VER"
echo "────────────────────────────────────"

# Source of truth in code
check "cmd/codeforge/main.go" "ProjectVersion = \"$VER\"" "cmd/codeforge/main.go ProjectVersion"

# README badge + table (table cells may use `vX.Y.Z` with optional spaces)
check "README.md" "version-v${VER}" "README badge"
if [[ -f README.md ]]; then
  if grep -qE "\*\*Version\*\*.*v?${VER}" README.md || grep -qE "v${VER}" README.md; then
    echo "OK:   README version table / mentions v${VER}"
  else
    echo "FAIL: README — expected version $VER"
    fail=1
  fi
fi

# TUI about (hardcoded display)
if grep -q 'aboutText\|CodeForge TUI v' internal/tui/model.go 2>/dev/null; then
  check "internal/tui/model.go" "CodeForge TUI v${VER}" "TUI aboutText"
fi

# MCP clientInfo
if grep -q 'clientInfo' internal/provider/mcp.go 2>/dev/null; then
  check "internal/provider/mcp.go" "\"version\": \"${VER}\"" "MCP client version"
fi

# ACP default version
if grep -q 'opt.Version == ""' internal/acp/server.go 2>/dev/null; then
  check "internal/acp/server.go" "opt.Version = \"${VER}\"" "ACP default version"
fi

# Formula (must match for release; sha256 may still be placeholder)
if [[ -f Formula/codeforge.rb ]]; then
  check "Formula/codeforge.rb" "version \"${VER}\"" "Homebrew Formula version"
fi

# Goreleaser uses tag {{.Version}} — just ensure file exists
if [[ -f .goreleaser.yaml ]]; then
  echo "OK:   .goreleaser.yaml present"
else
  echo "FAIL: .goreleaser.yaml missing"
  fail=1
fi

echo "────────────────────────────────────"
if [[ $fail -ne 0 ]]; then
  echo "Version check FAILED for $VER"
  echo "Update all locations or run: scripts/bump-version.sh $VER"
  exit 1
fi
echo "All version checks passed ($VER)"
exit 0
