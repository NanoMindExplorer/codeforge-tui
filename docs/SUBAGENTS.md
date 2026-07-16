# Subagents & Personas (Phase G6)

Grok-compatible **spawn_subagent** with agent types, capability modes, git worktree isolation, and personas.

## spawn_subagent parameters

| Parameter | Description |
|-----------|-------------|
| `task` / `prompt` | Task for the child (required; either name works) |
| `description` | Short 3–5 word label |
| `subagent_type` / `mode` | `explore` (default) · `plan` · `general-purpose` |
| `capability_mode` | `read-only` · `read-write` · `execute` · `all` |
| `isolation` | `none` (default) · `worktree` |
| `persona` | Named persona overlay (e.g. `researcher`) |
| `max_iterations` | Cap tool loop (default 6, max 16) |
| `background` | If `true`, return job id immediately (Phase G7) |
| `resume_from` | Finished job id to continue with a new prompt (Phase G7) |

### Agent types

| Type | Tools | Use |
|------|-------|-----|
| **explore** | Read/search/web (no writes) | Research codebase |
| **plan** | Explore + `write_plan` | Design a plan without editing source |
| **general-purpose** | Full registry minus nested spawn | Implementation work |

### Capability modes

Override the default tool set for the type:

| Mode | Read | Write | Shell |
|------|------|-------|-------|
| `read-only` | ✅ | — | — |
| `read-write` | ✅ | ✅ | — |
| `execute` | ✅ | — | ✅ |
| `all` | ✅ | ✅ | ✅ |

### Isolation: worktree

```text
isolation: worktree
```

Creates a git worktree under `.codeforge/worktrees/<label>-<id>` on a new branch, runs the subagent there, then removes the worktree. Requires a git repository. Changes are discarded unless the child commits and you recover the branch before cleanup.

## Personas

Personas inject a `<system-reminder>` into the subagent system prompt (tone, format, focus) without changing tools.

### Bundled

| Name | Focus |
|------|--------|
| `researcher` | Cite paths; evidence-first |
| `concise` | Short high-signal answers |
| `reviewer` | Bugs, security, severity tags |

### Custom

```yaml
# .codeforge/personas/security.yaml
name: security
description: Security-focused review
instructions: |
  Hunt for injection, secret leaks, and auth bugs.
  Severity-tag every finding.
```

Also: `~/.codeforge/personas/*.yaml`, `.grok/personas/*.toml`, config:

```yaml
# ~/.config/codeforge/config.yaml
subagents:
  personas:
    researcher:
      instructions: "Always quote file:line."
  extra_dirs:
    - ~/team-personas
```

### Slash

```text
/personas              # list
/personas researcher   # show body
/personas reload
```

## Example tool call

```json
{
  "prompt": "Map how auth middleware works",
  "description": "auth map",
  "subagent_type": "explore",
  "persona": "researcher",
  "capability_mode": "read-only"
}
```

```json
{
  "task": "Design caching for the API",
  "subagent_type": "plan",
  "persona": "concise"
}
```

```json
{
  "prompt": "Fix flaky test in pkg/foo",
  "subagent_type": "general-purpose",
  "isolation": "worktree",
  "capability_mode": "all"
}
```

## Background subagents (Phase G7)

```json
{
  "prompt": "Map all HTTP handlers",
  "subagent_type": "explore",
  "background": true,
  "description": "http map"
}
```

Returns immediately:

```text
Background subagent sub-1 started (explore).
Poll: get_subagent_output id=sub-1
```

Then:

```json
{ "id": "sub-1", "wait_ms": 30000 }
```

Tools: `get_subagent_output` · alias `get_command_or_subagent_output`  
Slash: `/subagents` · `/subagents show sub-1` · `/subagents cancel sub-1`

Sync runs are also recorded with an id so you can `resume_from` them.

### Resume

```json
{
  "prompt": "Now check auth only",
  "resume_from": "sub-1"
}
```

Prior messages + system are restored; the new prompt is appended.

## Persistence (cross-session)

Jobs are saved under `~/.codeforge/subagents/<id>.json` (override with `CODEFORGE_SUBAGENTS_DIR`).

- Every `Put` / status update writes disk
- On bootstrap, finished jobs are **reloaded** (resume_from works after restart)
- Jobs still `running` on disk (process crash) are marked `failed`
- Sequence numbers continue from the highest restored id

## Honest limits

- Nested `spawn_subagent` is disabled inside children
- Worktree cleanup always runs after the child finishes (including background)
- Persisted messages are capped (~40 turns, 200KB output) to keep disk lean

## Related

- Grok user guide: Subagents (`16-subagents.md`)
- [SKILLS.md](./SKILLS.md) · [SANDBOX.md](./SANDBOX.md)
