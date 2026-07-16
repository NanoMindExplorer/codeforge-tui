#!/usr/bin/env bash
# bump-version.sh NEW_VERSION — update VERSION and common string locations.
set -euo pipefail

NEW="${1:-}"
if [[ -z "$NEW" ]]; then
  echo "Usage: $0 X.Y.Z"
  exit 1
fi
NEW="${NEW#v}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

OLD="$(tr -d '[:space:]' < VERSION 2>/dev/null || echo "")"
echo "$NEW" > VERSION
echo "VERSION: ${OLD:-?} → $NEW"

# main.go
if [[ -f cmd/codeforge/main.go ]]; then
  sed -i -E "s/ProjectVersion = \"[^\"]+\"/ProjectVersion = \"${NEW}\"/" cmd/codeforge/main.go
fi

# README
if [[ -f README.md ]]; then
  sed -i -E "s/version-v[0-9]+\.[0-9]+\.[0-9]+/version-v${NEW}/g" README.md
  sed -i -E "s/\`v[0-9]+\.[0-9]+\.[0-9]+\`/\`v${NEW}\`/g" README.md
  sed -i -E "s/CodeForge \*\*v[0-9]+\.[0-9]+\.[0-9]+\*\*/CodeForge **v${NEW}**/g" README.md
fi

# TUI about
if [[ -f internal/tui/model.go ]]; then
  sed -i -E "s/CodeForge TUI v[0-9]+\.[0-9]+\.[0-9]+/CodeForge TUI v${NEW}/g" internal/tui/model.go
fi

# MCP
if [[ -f internal/provider/mcp.go ]]; then
  sed -i -E 's/"version": "[0-9]+\.[0-9]+\.[0-9]+"/"version": "'"${NEW}"'"/g' internal/provider/mcp.go
fi

# ACP
if [[ -f internal/acp/server.go ]]; then
  sed -i -E 's/opt\.Version = "[0-9]+\.[0-9]+\.[0-9]+"/opt.Version = "'"${NEW}"'"/g' internal/acp/server.go
fi

# Formula
if [[ -f Formula/codeforge.rb ]]; then
  sed -i -E "s/version \"[^\"]+\"/version \"${NEW}\"/" Formula/codeforge.rb
fi

# Roadmap baseline if present
if [[ -f docs/GROK_PARITY_ROADMAP.md ]]; then
  sed -i -E "s/CodeForge \*\*v[0-9]+\.[0-9]+\.[0-9]+\*\*/CodeForge **v${NEW}**/g" docs/GROK_PARITY_ROADMAP.md
fi

echo "Run: bash scripts/check-version.sh"
bash scripts/check-version.sh
