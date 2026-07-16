# CodeForge ACP (Agent Client Protocol)

Phase 8 of Grok parity: run CodeForge as an **ACP agent** so IDEs and editors can drive multi-turn tool-using sessions over **stdio** or **WebSocket**.

Spec: [Agent Client Protocol](https://agentclientprotocol.com)

## Commands

```bash
# IDE subprocess (JSON-RPC lines on stdin/stdout; logs on stderr)
codeforge agent --model gemini-2.5-flash --always-approve stdio

# Network server
codeforge agent serve --bind 127.0.0.1:2419 --secret mytoken
# Auth: ?secret=mytoken  or  Authorization: Bearer mytoken
# Env: CODEFORGE_AGENT_SECRET
```

Shared flags (before the mode name):

| Flag | Description |
|------|-------------|
| `-m, --model` | Model id for the active provider |
| `--always-approve` / `--yolo` | Map to permission always_approve (default for ACP) |
| `--dont-ask` | Deny tools that would prompt |
| `--plan` | DESIGN permission mode (plan.md only) |
| `-C, --workdir` | Default project root |
| `--max-iter` | Agent loop iterations (default 12) |
| `--bind` | `serve` only |
| `--secret` | `serve` only |

## Supported methods

| Method | Direction | Notes |
|--------|-----------|--------|
| `initialize` | C→A | Protocol version + capabilities |
| `authenticate` | C→A | No-op (empty result) |
| `session/new` | C→A | `{ cwd, mcpServers?, _meta? }` → `{ sessionId }` |
| `session/load` | C→A | Resume CodeForge session by id |
| `session/prompt` | C→A | Prompt blocks → streams updates → `{ stopReason }` |
| `session/cancel` | C→A | Notification; cancels in-flight turn |
| `session/update` | A→C | `agent_message_chunk`, `tool_call`, `tool_call_update` |

Permissions use the Phase 6 engine. ACP defaults to **always_approve** so IDEs are not blocked on interactive ask; use `--dont-ask` for CI lockdown.

Sessions are stored under `~/.codeforge/sessions/` (same layout as the TUI).

## Minimal client (bash + jq)

```bash
export GEMINI_API_KEY=…
codeforge agent --always-approve stdio <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}
{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/path/to/project"}}
EOF
```

Scripted CI coverage lives in `internal/acp/server_test.go` (fake agent runner — no API key required).

## Zed

1. Install the `codeforge` binary on `PATH`.
2. Configure an external agent / custom agent that spawns:

   ```text
   codeforge agent --model <your-model> --always-approve stdio
   ```

3. Set working directory to the project root (Zed usually passes the workspace as cwd; you can also pass `-C`).

Exact JSON keys depend on Zed’s external-agent schema (see [Zed external agents](https://zed.dev/docs/ai/external-agents)). Point the command at CodeForge’s `agent stdio` transport.

## Neovim

Any ACP-capable plugin that can spawn a stdio agent works, for example:

- **CodeCompanion** / **avante.nvim** — if configured for ACP stdio agents  
- **agent-shell** style integrations that speak JSON-RPC lines  

Example spawn:

```vim
" Conceptually: command = {"codeforge", "agent", "--always-approve", "stdio"}
```

Ensure API keys are available in the Neovim environment (`GEMINI_API_KEY`, etc.).

## TypeScript sketch

```typescript
import { spawn } from "child_process";
import * as readline from "readline";

const proc = spawn("codeforge", ["agent", "--always-approve", "stdio"]);
const rl = readline.createInterface({ input: proc.stdout! });

function request(id: number, method: string, params: object) {
  proc.stdin!.write(JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n");
}

request(1, "initialize", { protocolVersion: 1 });
// read result lines from rl; then session/new; then session/prompt
// listen for method === "session/update" notifications
```

## WebSocket

```bash
codeforge agent serve --bind 127.0.0.1:2419 --secret secret
# Connect: ws://127.0.0.1:2419/?secret=secret
# Send the same newline-delimited JSON-RPC messages as stdio.
```

Health: `GET http://127.0.0.1:2419/health` → `ok`
