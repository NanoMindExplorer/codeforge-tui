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
| `initialize` | C→A | Protocol version + capabilities + `xaiExtensions` list |
| `authenticate` | C→A | No-op (empty result) |
| `session/new` | C→A | `{ cwd, mcpServers?, _meta? }` → `{ sessionId }` |
| `session/load` | C→A | Resume CodeForge session by id |
| `session/prompt` | C→A | Prompt blocks → streams updates → `{ stopReason }` |
| `session/cancel` | C→A | Notification; cancels in-flight turn |
| `session/update` | A→C | `agent_message_chunk`, `tool_call`, `tool_call_update` |

### x.ai/* extensions (Grok-compatible)

Advertised in `initialize` → `agentCapabilities.xaiExtensions`. Representative set:

| Prefix | Methods |
|--------|---------|
| `x.ai/fs/*` | `list`, `exists`, `read_file`, `write_file` |
| `x.ai/git/*` | `status`, `stage`, `commit`, `diffs`, `discard` |
| `x.ai/git/worktree/*` | `list`, `create`, `remove`, `apply`, `gc` |
| `x.ai/search/*` | `content`, `fuzzy/open`, `fuzzy/change` |
| `x.ai/terminal/*` | `create`, `kill`, `output`, `wait_for_exit` |
| `x.ai/session/*` | `fork`, `resolve_local_for_worktree_resume` |
| `x.ai/*` | `prompt_history`, `rewind/list`, `rewind/apply`, `compact_conversation` |
| `x.ai/subagent/*` | `list`, `get`, `cancel` |
| `x.ai/auth/*` | `get_url`, `submit_code` (stubs — use API keys) |
| `x.ai/feedback`, `x.ai/telemetry/status` | acknowledged |

Notifications: `x.ai/search/fuzzy/status`, `x.ai/git/worktree/status` (and standard `session/update`).

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

## `codeforge/error` session update (Q6.3)

When a provider/agent error occurs mid-turn, the agent may emit a **structured**
`session/update` notification in addition to a human-readable text chunk:

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "…",
    "update": {
      "sessionUpdate": "codeforge/error",
      "code": "rate_limit",
      "message": "Rate limited by the provider",
      "hint": "Wait a moment, then retry",
      "retry": true
    }
  }
}
```

| Field | Type | Meaning |
|-------|------|---------|
| `sessionUpdate` | string | Always `"codeforge/error"` |
| `code` | string | Stable machine code (`auth`, `rate_limit`, `network`, `timeout`, `budget`, `cancelled`, `max_iterations`, `unsupported`, …) |
| `message` | string | User-facing summary (no stack / raw JSON dump) |
| `hint` | string | Optional recovery guidance |
| `retry` | bool | Client may offer “retry last turn” |

**IDE guidance:** surface `message` + `hint` in the UI; use `code` for icons / analytics; if `retry` is true, offer re-send of the last user prompt.

Standard JSON-RPC `error` objects on responses remain for protocol failures (`-32600` parse/params/method).

## Cancel / interrupt (Q6.5)

```json
{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"…"}}
```

This is a **notification** (no `id`). The agent cancels the in-flight `session/prompt` context; the prompt response (when sent) uses `stopReason: "cancelled"`.

## Multi-session isolation (Q6.2)

Each `session/new` creates an independent tool registry with its own permission
engine (`Registry.Authorizer`). Nested `spawn_subagent` children resolve the
authorizer from the **session registry first**, then the process-wide fallback.
Concurrent sessions must not share a single global authorizer for tool gates.

## CI fixtures (Q6.1 / Q6.4)

```bash
bash scripts/acp-fixture.sh
# or: go test ./internal/acp/ -count=1
```

Covers: WebSocket health/auth/initialize, multi-turn prompt+tool events (fake runner),
stdio scripted initialize, session cancel interrupt.
