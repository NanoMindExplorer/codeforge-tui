# Auth & onboarding (multi-provider)

CodeForge supports **several API keys at once**. Only **one provider is active** (footer + `/provider`).

## Quick start (≤ 3 minutes)

```bash
# Option A — wizard (recommended first run)
codeforge

# Option B — env then launch
export XAI_API_KEY=xai-…      # Grok 4.5 (preferred when set)
# or:
export GEMINI_API_KEY=AIza…   # free tier
codeforge --skip-wizard

# Option C — inside TUI
/setup gemini AIza…
/setup grok xai-…
/provider              # show every key + why active
/provider gemini       # switch without re-pasting
```

## Which provider becomes active?

| Priority | When |
|----------|------|
| 1 | `~/.codeforge/onboarding.json` preference (set by wizard or `/provider` / `/setup`) |
| 2 | `default_provider` in `~/.config/codeforge/config.yaml` |
| 3 | First present key in order: **grok → gemini → claude → openai** |
| 4 | Ollama if chosen and reachable |

**Important:** having both `XAI_API_KEY` and `GEMINI_API_KEY` does **not** mean both run at once.  
The footer shows the active one. Other keys stay available via `/provider <name>`.

### Multi-key first run

If the wizard detects **2+ keys** and you never set a preference, it asks:

```text
You have 2 providers with keys. Pick the DEFAULT active one:
  [1] grok     env:XAI_API_KEY
  [2] gemini   env:GEMINI_API_KEY
```

Your choice is saved so the next launch is stable.

### Single-key first run

Confirms the detected key (Enter) or lets you **add another** (`a`).

### No keys

Full catalog with env names, key shapes, and docs URLs — paste once, done.

## Commands

| Command | Effect |
|---------|--------|
| `/setup` | Full multi-provider status + how-to |
| `/setup <provider>` | Activate existing key, or show paste form |
| `/setup <provider> <key> [model]` | Save key, set active, persist preference |
| `/provider` | Same status table (sources + *why* active) |
| `/provider <name>` | Switch active + persist preference |
| `/doctor` | Health: keys, model, color, sandbox |
| `/model` | List/switch models for **current** provider |

## Files

| Path | Role |
|------|------|
| `~/.codeforge/onboarding.json` | completed / skipped / preferred provider / welcome_shown |
| `~/.config/codeforge/config.yaml` | `default_provider` + optional `providers.*.api_key` |

## Env vars

| Provider | Env |
|----------|-----|
| Grok | `XAI_API_KEY` or `GROK_API_KEY` |
| Gemini | `GEMINI_API_KEY` |
| Claude | `ANTHROPIC_API_KEY` |
| OpenAI-compatible | `OPENAI_API_KEY` (+ optional `OPENAI_BASE_URL`) |
| Ollama | `OLLAMA_HOST` (default localhost) |

## Headless / CI

```bash
# No key → exit 2 + JSON code no_provider
codeforge agent --json "hi"

# With key — same resolution order as TUI
export GEMINI_API_KEY=…
codeforge agent --json "summarize README"
```

## Footer hints

| Footer | Meaning |
|--------|---------|
| `⚠ no API key · /setup` | Nothing validates — run setup |
| `2 keys · /provider` | Multiple keys; active is the model chip on the left |
| (no badge) | Single healthy provider |

## Design goals

1. **Never silent** about which provider is active when multiple keys exist.  
2. **Never drop** other keys when switching (`/provider` only changes active).  
3. **≤ 3 minutes** to first successful turn for a new user.  
4. Same resolution rules in wizard, TUI, headless, and `codeforge doctor`.
