#!/usr/bin/env bash
# acp-fixture.sh — Q6.4 multi-turn ACP smoke via go test (no live API key).
# Golden path: initialize → session → prompt with tool events.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export GOSUMDB=off
echo "ACP multi-turn fixture (unit):"
go test ./internal/acp/ -count=1 -run 'TestMultiTurnJSONRPCFixture|TestServeStdioScripted|TestACPinitializeSessionNewPrompt|TestSessionCancel' -v
echo "ACP WebSocket fixture:"
go test ./internal/acp/ -count=1 -run 'TestServeMux' -v
echo "OK: ACP fixtures passed"
