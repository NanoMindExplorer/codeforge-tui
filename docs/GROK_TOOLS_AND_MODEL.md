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

## Phase G4 — OS sandbox ✅

Grok-compatible profiles: `off` · `workspace` · `read-only` · `strict` · `devbox`

| Item | Detail |
|------|--------|
| CLI | `--sandbox <profile>` (TUI + agent) |
| Env | `CODEFORGE_SANDBOX` / `GROK_SANDBOX` |
| Config | `sandbox.profile` + `sandbox.deny` |
| Slash | `/sandbox` |
| Shell | bubblewrap when available, else soft path + optional `unshare -n` |
| Tools | soft `CheckWrite` / `CheckRead` on file tools |
| Docs | [SANDBOX.md](./SANDBOX.md) |

## Phase G5 — Skills ✅

Grok-compatible `SKILL.md` packages (reusable procedures).

| Item | Detail |
|------|--------|
| Discovery | `.codeforge/skills`, `.grok/skills`, `~/.codeforge/skills`, Claude/Cursor compat |
| Format | YAML frontmatter + markdown body |
| Slash | `/skills` · `/name` · `/skill:name` |
| Prompt | Catalog injected into agent + headless system prompt |
| Config | `skills.paths` / `ignore` / `disabled` / compat flags |
| Docs | [SKILLS.md](./SKILLS.md) |
| Example | `examples/skills/commit/SKILL.md` |

## Phase G6 — Subagents & Personas ✅

| Item | Detail |
|------|--------|
| Types | `explore` · `plan` · `general-purpose` |
| Params | `prompt`/`task`, `capability_mode`, `isolation`, `persona`, `description` |
| Isolation | `worktree` → git worktree under `.codeforge/worktrees/` |
| Personas | bundled researcher/concise/reviewer + YAML/TOML + config |
| Slash | `/personas` |
| Docs | [SUBAGENTS.md](./SUBAGENTS.md) |

## Phase G7 — Background subagents ✅

| Item | Detail |
|------|--------|
| `background` | Async spawn returns `sub-N` id |
| `get_subagent_output` | Poll status/output; optional `wait_ms` |
| Alias | `get_command_or_subagent_output` |
| `resume_from` | Continue a finished job with new prompt |
| Slash | `/subagents` list · show · cancel |
| Tracking | Sync runs also get ids for resume |
| Persist | `~/.codeforge/subagents/*.json` across restarts |

## Phase G8 — Coulds closed ✅

| Item | Detail |
|------|--------|
| Process Landlock | Linux process-wide write (and strict read) rules at startup |
| Process Seatbelt | macOS `sandbox-exec` re-exec with generated profile |
| ACP `x.ai/*` | Filesystem, git, worktree, search, terminal, session, subagent, … |
| Subagent persist | Cross-session job store for resume_from |

## Phase G9 — Native reasoning streams ✅

| Item | Detail |
|------|--------|
| StreamToken.Reasoning | Provider thinking deltas |
| CompletionResponse.Reasoning | Full CoT on Complete (agent loop) |
| Grok/OpenAI | `reasoning_content` / `include_reasoning` / `reasoning_effort` |
| Gemini | `thinkingConfig.includeThoughts` + `thought` parts |
| Claude | Extended thinking budget + thinking blocks |
| TUI | thinking block; replaces synthetic `planning…` |
| ACP | `agent_thought_chunk` |
| Docs | [REASONING.md](./REASONING.md) |

## Phase G10 — pager.toml matrix ✅

| Item | Detail |
|------|--------|
| Files | `~/.codeforge/pager.toml`, `~/.grok/pager.toml`, project variants, YAML |
| Sections | layout, scrollbar, scroll, display, animation, blocks.*, prompt, todo, terminal, ui |
| Slash | `/pager` · `/pager reload` |
| Example | `examples/pager.toml` |
| Docs | [PAGER.md](./PAGER.md) |

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
