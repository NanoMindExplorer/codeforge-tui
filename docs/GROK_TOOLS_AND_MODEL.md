# Grok 4.5 model + tool surface in CodeForge

Phased plan to match **Grok Build / Grok 4.5** agent capabilities inside CodeForge.

## Phase G1 — Model: Grok 4.5 (xAI API)

| Item | Detail |
|------|--------|
| Provider name | `grok` / `xai` |
| Endpoint | `https://api.x.ai/v1` (OpenAI-compatible) |
| Default model | `grok-4.5` |
| Auth | `XAI_API_KEY` or `GROK_API_KEY` |
| Optional | `XAI_BASE_URL` override |
| Context | 500k tokens |
| Pricing (list) | ~$2 / $6 per 1M in/out |

## Phase G2 — Tools (Grok built-ins) ✅

| Grok tool | CodeForge | Status |
|-----------|-----------|--------|
| `read_file` | `read_file` | ✅ |
| `search_replace` / `edit_file` | same + alias | ✅ |
| `write_file` | `write_file` | ✅ |
| `list_dir` / `list_directory` | same + alias | ✅ |
| `grep` | alias → `grep_search` | ✅ |
| `run_terminal_command` | alias → `run_command` | ✅ |
| `web_search` | DuckDuckGo (+ optional Brave) | ✅ |
| `web_fetch` | alias → `fetch_url` | ✅ |
| `todo_write` | `todo_write` | ✅ |
| `spawn_subagent` | explore \| general | ✅ |
| `memory_search` / `memory_write` | `~/.codeforge/memory/` | ✅ |
| `ask_user_question` | pending question for user | ✅ |
| `enter_plan_mode` / `exit_plan_mode` | same | ✅ |
| MCP | `mcp_*` | ✅ |
| GitHub / apply_patch / codebase_search / diagnostics | CodeForge extras | ✅ |

## Phase G3 — Integration ✅

- Prefer Grok when `XAI_API_KEY` / `GROK_API_KEY` is set
- Wizard accepts `xai-…` keys
- Permission read-only list includes Grok tools
- Agent system prompt lists Grok tool names

### Quick start

```bash
export XAI_API_KEY=xai-...
codeforge
# /provider → grok · /model grok-4.5
codeforge agent --model grok-4.5 --always-approve "list files and summarize README"
```
