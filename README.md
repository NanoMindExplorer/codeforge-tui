# CodeForge

> Terminal AI Coding Companion — open, modular, vendor-neutral — **dan terasa seperti dari masa depan.**

**Created by NanoMind · 2026 · Apache 2.0**  
**Codename:** Neo-Forge · **Version:** `v0.3.0`

Building the future of terminal AI coding, one keystroke at a time.

## Quick Start

```bash
# Install (release or from source)
curl -fsSL https://raw.githubusercontent.com/NanoMindExplorer/codeforge/main/install.sh | sh

# Free Gemini key → https://aistudio.google.com/apikey
export GEMINI_API_KEY="AIzaSy..."
codeforge
```

From source:

```bash
git clone https://github.com/NanoMindExplorer/codeforge.git
cd codeforge
CGO_ENABLED=0 go build -ldflags="-s -w" -o codeforge ./cmd/codeforge/
./codeforge
```

## Features (v0.3.0 Neo-Forge)

| Area | Capability |
|------|------------|
| **TUI** | 3-pane layout (Chat · Diff · Files), compact tab mode &lt;100 cols |
| **Theme** | Aurora Dark design tokens · `CODEFORGE_THEME` · `~/.codeforge/theme.yaml` |
| **Chat** | Viewport scroll · multi-line textarea · glamour markdown + syntax highlight |
| **Diff** | Gutter line numbers · +N/-M badges · multi-file tabs · pending badge |
| **Trust** | **Plan mode** (default) stages `write_file` → multi-file review · **Act** mode optional |
| **Workflow** | `Ctrl+K` fuzzy palette · `@file` mention · sessions · `/undo` checkpoints |
| **Providers** | Gemini · Claude · OpenAI-compatible · Ollama (local) |
| **Motion** | Gradient border · typewriter system msgs · toast · `--no-motion` kill switch |

## Keybindings

| Key | Action |
|-----|--------|
| `i` | INSERT (chat) |
| `I` | INSERT with `/act` |
| `Ctrl+K` | Command palette |
| `@` | File mention picker |
| `Shift+P` | Toggle Plan ↔ Act |
| `Tab` / `1` `2` `3` | Switch panes |
| `j` `k` `g` `G` | Scroll |
| `?` | Help |
| `q` | Quit |

## Slash commands

```
/act /read /ls /grep /run /explain /fix
/provider /model /mode /sessions /undo
/status /commit /cost /clear /help /about /quit
```

## Plan vs Act

- **PLAN** (default, safe): agent may read/search/run freely; every `write_file` is **staged** and shown in Diff with `⏳ PENDING`. After the turn, a review overlay lets you accept/reject per file.
- **ACT**: writes apply immediately (power-user / rapid iteration). Toggle with `Shift+P` or `/mode act`.

## Sessions & undo

- Conversations auto-save under `~/.codeforge/sessions/`
- `/sessions` lists and resumes
- Every applied write is checkpointed; `/undo` restores the last file

## Environment

| Var | Purpose |
|-----|---------|
| `GEMINI_API_KEY` | Google Gemini (default free tier) |
| `ANTHROPIC_API_KEY` | Claude |
| `OPENAI_API_KEY` / `OPENAI_BASE_URL` | OpenAI or compatible endpoint |
| `OLLAMA_HOST` / `OLLAMA_MODEL` | Local Ollama |
| `CODEFORGE_THEME` | `aurora` (default) or `light` |
| `CODEFORGE_NO_MOTION=1` | Disable animations |
| `NERD_FONT=1` | Prefer Nerd Font glyphs |

## Flags

```
codeforge [workdir] [--no-motion] [--skip-wizard] [-v] [-h]
```

## Tech stack

- Go 1.25 · Bubble Tea · Bubbles · Lipgloss · Glamour · Harmonica  
- go-colorful · sahilm/fuzzy · muesli/reflow · go-git · Viper  
- Pure Go, `CGO_ENABLED=0` (Termux / Android friendly)

## Architecture

```
internal/
  agent/ provider/ tool/ git/ diff/ config/   # core (stable)
  theme/ keymap/ session/ checkpoint/        # Neo-Forge foundation
  ui/{components,markdown,diffview,palette,filepicker,review}
  tui/                                       # Bubble Tea orchestrator
```

## License

Apache License 2.0 · **NanoMind** — Original Creator — 2026
