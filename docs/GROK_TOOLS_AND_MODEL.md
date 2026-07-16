# Grok 4.5 model + tool surface in CodeForge

Phased plan to match **Grok Build / Grok 4.5** agent capabilities inside CodeForge.

## Phase G1 — Model: Grok 4.5 (xAI API) ✅

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
| `glob` / `glob_file_search` / `find_files` | recursive glob (ignores node_modules, .git, …) | ✅ |
| `grep` | alias → `grep_search` | ✅ |
| `run_terminal_command` | alias → `run_command` | ✅ |
| `web_search` | DuckDuckGo (+ optional Brave) | ✅ |
| `web_fetch` | alias → `fetch_url` | ✅ |
| `todo_write` | `todo_write` | ✅ |
| `spawn_subagent` | explore (RO tools) \| general | ✅ |
| `memory_search` / `memory_write` | `~/.codeforge/memory/` + slash `/memory` | ✅ |
| `ask_user_question` / `ask_user` | modal option picker (1–9) + free text | ✅ |
| `enter_plan_mode` / `exit_plan_mode` | same | ✅ |
| MCP | `mcp_*` | ✅ |
| GitHub / apply_patch / codebase_search / diagnostics | CodeForge extras | ✅ |

### G2 polish (v1.1.1)

- **glob_file_search** registered in main registry + RO explore subagents
- Aliases: `glob`, `find_files`, `ask_user`
- Slash **`/memory list|add|search`** for humans
- **ask_user_question** opens interactive overlay (digit keys select options)
- Tool icons for Grok names in context pane / blocks
- Agent system prompt lists glob + ask_user aliases

## Phase G3 — Integration ✅

- Prefer Grok when `XAI_API_KEY` / `GROK_API_KEY` is set
- Wizard accepts `xai-…` keys
- Permission read-only list includes Grok tools + glob aliases
- Agent system prompt lists Grok tool names

### Quick start

```bash
export XAI_API_KEY=xai-...
codeforge
# /provider → grok · /model grok-4.5
codeforge agent --model grok-4.5 --always-approve "list files and summarize README"
```

### Slash memory

```text
/memory                  # help
/memory list             # recent notes
/memory add use go modules
/memory search modules
```
