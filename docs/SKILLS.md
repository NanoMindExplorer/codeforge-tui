# Skills (Phase G5)

Grok-compatible **reusable prompt packages**. A skill is a directory with a `SKILL.md` file that teaches the agent a repeatable procedure.

## Quick start

```bash
# User skill (all projects)
mkdir -p ~/.codeforge/skills/commit
cat > ~/.codeforge/skills/commit/SKILL.md <<'EOF'
---
name: commit
description: Create conventional git commits. Use when the user wants to commit or runs /commit skill.
---

# Commit skill

1. Run `git status` and `git diff`
2. Stage relevant files
3. Write a conventional commit message
4. Run `git commit`
EOF

codeforge
# /skills          — list
# /commit          — run skill (if no built-in collision)
# /skills commit   — show body
# /skills reload   — rescan disk
```

Project-local (shared via git):

```text
.codeforge/skills/review-pr/SKILL.md
.grok/skills/review-pr/SKILL.md     # also discovered (Grok layout)
```

## Discovery (priority high → low)

| Location | Source label |
|----------|----------------|
| `./.codeforge/skills/`, `./.grok/skills/`, `./.agents/skills/` | `local` |
| `./.claude/skills/`, `./.cursor/skills/` | `claude` / `cursor` (compat, on by default) |
| `~/.codeforge/skills/`, `~/.grok/skills/`, `~/.grok/bundled/skills/` | `user` |
| `skills.paths` in config | `extra` |

Same skill **name**: higher priority wins (local overrides user).

Flat command files (Claude legacy) also load:

```text
.codeforge/commands/ship.md   →  /ship
.grok/commands/*.md
```

## SKILL.md format

```markdown
---
name: review-pr
description: Review a pull request carefully. Use when user says review PR or /review-pr.
when-to-use: pull request review
user-invocable: true
disable-model-invocation: false
---

# Review PR

1. Fetch PR diff
2. Check tests and security
3. Summarize findings
```

| Field | Meaning |
|-------|---------|
| `name` | Slash id (default: directory name) |
| `description` | Shown in catalog; drives when the model should apply the skill |
| `when-to-use` | Extra trigger hints |
| `user-invocable` | Appear as `/name` (default true) |
| `disable-model-invocation` | Hide from auto catalog (slash only) |

## Using skills

| Action | How |
|--------|-----|
| List | `/skills` |
| Show | `/skills <name>` |
| Reload | `/skills reload` |
| Run | `/name` or `/skill:name` if built-in collides |
| Auto | Catalog is injected into the agent system prompt; model follows matching skills |

Headless/CI also gets the catalog after rules injection.

## Config

`~/.config/codeforge/config.yaml`:

```yaml
skills:
  paths:
    - ~/team-skills
  ignore:
    - ~/team-skills/wip
  disabled:
    - experimental-skill
  compat_claude: true
  compat_cursor: true
```

## vs project rules vs plugins

| | **Rules** (`AGENTS.md`) | **Skills** | **Plugins** |
|--|-------------------------|------------|-------------|
| Scope | Always-on project policy | Task-specific procedures | External tools (YAML bridge) |
| Invoke | Automatic | Catalog + `/name` | Agent tool call |
| Format | Markdown | `SKILL.md` + frontmatter | `plugin.yaml` |

## Related

- Grok user guide: Skills (`08-skills.md`)
- [GROK_TOOLS_AND_MODEL.md](./GROK_TOOLS_AND_MODEL.md) Phase G5
