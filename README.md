# CodeForge

**Terminal AI Coding Companion** — open, modular, vendor-neutral — *and it feels like the future.*

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/)
[![Version](https://img.shields.io/badge/version-v0.3.0-22D3EE)](https://github.com/NanoMindExplorer/codeforge)

| | |
|---|---|
| **Author** | NanoMind |
| **Year** | 2026 |
| **License** | Apache License 2.0 |
| **Codename** | Neo-Forge |
| **Version** | `v0.8.0` |

CodeForge is a single-binary TUI that puts a multi-provider AI coding agent in your terminal: stream chat, call tools on your project, review file writes safely (Plan mode), and **integrate with GitHub** (PRs, issues, checks, push/pull — the same class of workflows modern AI coding agents use) — without leaving the keyboard.

---

## Grok 4.5 parity

CodeForge v0.8+ aims for **Grok Build TUI–compatible** UX. The phased plan to true 1:1 parity:

→ **[docs/GROK_PARITY_ROADMAP.md](./docs/GROK_PARITY_ROADMAP.md)** (Phase 0–9)

## Table of contents

1. [Features](#features)
2. [Requirements](#requirements)
3. [Installation](#installation)
4. [API keys & providers](#api-keys--providers)
5. [GitHub setup](#github-setup)
6. [Quick start](#quick-start)
7. [User guide](#user-guide)
8. [GitHub integration](#github-integration)
9. [Keybindings reference](#keybindings-reference)
10. [Slash commands reference](#slash-commands-reference)
11. [CLI flags](#cli-flags)
12. [Environment variables](#environment-variables)
13. [Configuration files](#configuration-files)
14. [Typical workflows](#typical-workflows)
15. [Architecture](#architecture)
16. [Development & tests](#development--tests)
17. [Distribution](#distribution)
18. [Troubleshooting](#troubleshooting)
19. [License & credits](#license--credits)

---

## Features

| Area | What you get |
|------|----------------|
| **TUI** | **Grok 4.5–style**: full-width scrollback + bottom `❯` prompt · GrokNight theme · optional Diff/Files drawers (`Ctrl+B`). |
| **Streaming chat** | Real-time token stream; assistant replies rendered as **Markdown** with **syntax-highlighted** code (Glamour). |
| **Agent loop** | Tool-calling agent: `read_file`, `write_file`, `list_dir`, `grep_search`, `run_command` (sandboxed to project root). |
| **Trust layer** | **Plan mode (default):** writes are staged → multi-file **review** before disk. **Act mode:** writes apply immediately. |
| **Diff pane** | Rich unified diffs: gutters, `+N/-M` badges, multi-file tabs, pending badge. |
| **Files pane** | Live project listing, AI “touched” highlights, optional git status glyphs. |
| **Workflow** | `Ctrl+K` fuzzy palette · `@file` attachments · persistent **sessions** · `/undo` checkpoints · toasts. |
| **Providers** | **Gemini** · **Claude** · **OpenAI-compatible** · **Ollama** (local/offline). |
| **GitHub** | **`gh` / token**: PRs, issues, comments, reviews, diffs, **CI babysit**, push/pull — slash commands + agent `github` tool. |
| **Surgical edits** | **`search_replace`** + **`apply_patch`** (Plan-staged) preferred over full-file rewrites. |
| **Monorepo** | Multi-root workspace (`workspace.extra_roots`) + smart ignores / secret file skips. |
| **Live tools** | Tool progress streaming (babysit polls, long outputs) in the chat timeline. |
| **Project rules** | Auto-loads `AGENTS.md`, `CLAUDE.md`, `.codeforge/rules.md`, … into system prompts. |
| **Codebase index** | Offline keyword/symbol index + `codebase_search` tool. |
| **Diagnostics** | `diagnostics` tool (`go build` / `vet` / `test` / custom). |
| **Research sub-agent** | Read-only `research` tool for broad investigation. |
| **MCP** | Configure stdio MCP servers → tools registered as `mcp_*`. |
| **Budget** | `budget.max_cost_usd` hard-stop + status bar meter. |
| **Secret redaction** | Strips keys/tokens/`.env` before model context. |
| **Docs fetch** | `fetch_url` for public HTTPS (SSRF-safe). |
| **Headless CI** | `codeforge agent --json "…"` for pipelines (exit codes + JSON). |
| **Plugins** | YAML command plugins in `~/.codeforge/plugins/` → `plugin_*` tools. |
| **Session sync** | Export/import CLI + `CODEFORGE_SESSIONS_DIR` shared storage. |
| **Telemetry** | Opt-in, local JSONL, no prompts/source (privacy-first). |
| **Theme** | Aurora Dark design tokens; light theme; optional `~/.codeforge/theme.yaml`. |
| **Motion** | Breathing gradient borders, typewriter system messages, toast notifications. Disable with `--no-motion`. |
| **Portable** | Pure Go, `CGO_ENABLED=0`, Termux / Android friendly (~21MB single binary). |

---

## Requirements

- **OS:** Linux, macOS, Windows (via terminal); Termux on Android supported.
- **Go:** 1.25+ (only if building from source).
- **Terminal:** UTF-8; 256-color or truecolor recommended. Optional [Nerd Font](https://www.nerdfonts.com/) for richer icons (`NERD_FONT=1`).
- **Git:** optional but recommended (status / auto-commit).
- **API key:** at least one provider (Gemini free tier is the easiest start).

---

## Installation

### One-line installer

```bash
curl -fsSL https://raw.githubusercontent.com/NanoMindExplorer/codeforge/main/install.sh | sh
```

Detects OS/arch, prefers GitHub Releases, falls back to build-from-source when no release asset exists.

### Build from source

```bash
git clone https://github.com/NanoMindExplorer/codeforge.git
cd codeforge
go mod tidy
CGO_ENABLED=0 go build -ldflags="-s -w" -o codeforge ./cmd/codeforge/
sudo mv codeforge /usr/local/bin/   # or: cp codeforge "$PREFIX/bin/" on Termux
```

### Termux (Android)

```bash
pkg install -y golang git
git clone https://github.com/NanoMindExplorer/codeforge.git
cd codeforge
CGO_ENABLED=0 go build -ldflags="-s -w" -o codeforge ./cmd/codeforge/
cp codeforge "$PREFIX/bin/"
# Prefer no-motion on slow devices
codeforge --no-motion
```

### Verify

```bash
codeforge version
# → codeforge 0.8.0
```

---

## API keys & providers

Set **at least one** environment variable before a productive session.

| Provider | Environment | Notes |
|----------|-------------|--------|
| **Gemini** (default when set) | `GEMINI_API_KEY` | Free key: [Google AI Studio](https://aistudio.google.com/apikey) · default model `gemini-2.5-flash` |
| **Claude** | `ANTHROPIC_API_KEY` | Default model `claude-sonnet-4-20250514` |
| **OpenAI / compatible** | `OPENAI_API_KEY` | Optional `OPENAI_BASE_URL` (Groq, Together, Azure-style gateways, etc.) · default `gpt-4o-mini` |
| **Ollama** (local) | — | Auto-registers if `ollama serve` is reachable · `OLLAMA_HOST`, `OLLAMA_MODEL` |

Examples:

```bash
# Gemini (recommended free start)
export GEMINI_API_KEY="AIzaSy..."

# Claude
export ANTHROPIC_API_KEY="sk-ant-..."

# OpenAI-compatible endpoint
export OPENAI_API_KEY="sk-..."
export OPENAI_BASE_URL="https://api.openai.com/v1"

# Local Ollama
ollama pull llama3.2
export OLLAMA_MODEL="llama3.2"
# optional: export OLLAMA_HOST="http://127.0.0.1:11434"
```

Persist in your shell profile (`~/.bashrc`, `~/.zshrc`) as needed.

Inside CodeForge you can still switch with:

- `/provider` — list / switch (`gemini`, `claude`, `openai`, `ollama`)
- `/model` — list / switch model IDs for the current provider

---

## GitHub setup

CodeForge talks to GitHub the same way advanced AI coding tools do:

1. **Preferred:** [GitHub CLI](https://cli.github.com/) authenticated  
2. **Alternative:** Personal access token in the environment  

### Option A — `gh` CLI (recommended)

```bash
# Install gh (examples)
# Debian/Ubuntu: sudo apt install gh
# macOS: brew install gh
# Termux: pkg install gh

gh auth login
# scopes: repo, read:org, workflow (if you push Actions YAML)

gh auth status
```

CodeForge shells out to `gh` for PR/issue/check operations when it is on `PATH`.

### Option B — token only

```bash
export GITHUB_TOKEN="ghp_..."   # or GH_TOKEN
# classic PAT: repo scope (and workflow if needed)
# fine-grained: repository access + pull requests, issues, contents
```

REST mode is used when `gh` is missing or fails, as long as a token is set.

### Verify inside CodeForge

```text
/gh auth
/gh repo
```

Status bar shows `gh:@username` when auth succeeds.

---

## Quick start

```bash
# 1. Key
export GEMINI_API_KEY="AIzaSy..."

# 2. Open CodeForge in a project directory
cd /path/to/your/project
codeforge

# Or pass the project path
codeforge /path/to/your/project

# Skip first-run wizard; disable animations (SSH / Termux)
codeforge --skip-wizard --no-motion
```

**First 60 seconds inside the TUI (Grok simple mode):**

1. **Just type** — prompt is focused by default (`❯`).
2. Press **Enter** → stream a chat answer into the scrollback.
3. Type **`/act fix tests`** → agent with tools.
4. **`@`** → attach a file; **`Ctrl+K`** → palette; **`Shift+Tab`** → Plan/Act.
5. Press **`?`** anytime for help.

---

## User guide

### Interface (Grok 4.5–style)

```
┃ you
┃ fix the race in worker.go

┃ ⠋ working
┃ ◆ read_file  worker.go
┃   ✓ 84 lines
┃ Here's the race: …

╭─ ❯ ask anything, /command, or @file ─────────────────╮
│ ▌                                                     │
╰───────────────────────────────────────────────────────╯
 PROMPT  PLAN  gemini · flash  gh:@you · main  $0.01  groknight  14:02
 tab focus  @ file  / commands  ctrl+k  shift+tab plan/act  ctrl+b panels
```

| Region | Role |
|--------|------|
| **Scrollback** | Full-width blocks with left accent bars (you / assistant / tools) |
| **Prompt** | Bottom `❯` composer — focused by default |
| **Footer** | PROMPT/SCROLL · PLAN/ACT · model · git/gh · cost · theme |
| **Panels** | Optional Diff + Files (`Ctrl+B`) |

### Focus & keys

| Key | Action |
|-----|--------|
| *(type)* | Auto-focus prompt |
| `Tab` | Prompt ↔ scrollback |
| `Esc` / **2× Esc** | Scrollback / clear prompt |
| `@` | File picker |
| `/` | Slash commands (+ hint strip) |
| `Ctrl+K` | Palette |
| `Shift+Tab` | Plan ↔ Act |
| `Ctrl+B` | Toggle side panels |
| `/theme` | Cycle GrokNight · Aurora · TokyoNight · GrokDay |
| `/compact-mode` | Tighter padding |

### Chat vs agent

| Path | How | Tools? | Best for |
|------|-----|--------|----------|
| **Streaming chat** | Type natural language → Enter | No | Q&A, explanations |
| **Agent** | `/act <task>` or `/read`, `/fix`, … | Yes | Edit code, search, builds |

Agent system behavior (summary):

- Prefer **read before write**
- Uses filesystem tools under the **project workdir** only (path sandbox)
- In **Plan** mode, `write_file` is staged until you approve

### Plan vs Act (trust layer)

This is the core safety model for production code.

| | **PLAN** (default) | **ACT** |
|---|-------------------|---------|
| `read_file` / `list_dir` / `grep_search` | Free | Free |
| `run_command` | Free (30s timeout) | Free |
| `write_file` | **Staged** → Diff shows `⏳ PENDING` → **review UI** | **Written immediately** |
| Toggle | `Shift+P` or `/mode plan` | `Shift+P` or `/mode act` |

**Recommendation:** keep **PLAN** for unfamiliar tasks; use **ACT** only for tight, trusted iteration loops.

### Review overlay

When the agent finishes a turn and there are pending writes:

| Key | Action |
|-----|--------|
| `j` / `k` | Move between changed files |
| `Space` | Toggle accept / reject for current file |
| `a` | Accept all |
| `r` | Reject all |
| `Enter` | Apply accepted files to disk (+ checkpoints) |
| `Esc` | Cancel review (leave pending / discard flow as implemented) |

Accepted files are written to disk and previous contents are checkpointed for `/undo`.

### File mentions (`@file`)

1. Enter **INSERT** (`i`).
2. Press **`@`** → fuzzy file picker opens.
3. Type to filter · `↑`/`↓` · **Enter** to select.
4. The prompt gains `@path` and the file body is **attached** as context for the next send.

Useful for: “explain this file”, “refactor this module”, without a separate `/read` first.

### Command palette

**Ctrl+K** opens a fuzzy overlay fed by three sources:

1. Slash commands (`/act`, `/fix`, …)
2. Project files
3. Saved sessions

Navigate with `↑`/`↓` (or `j`/`k`), confirm with **Enter**, close with **Esc**.

### Sessions

- Conversations are auto-saved under `~/.codeforge/sessions/`.
- Metadata includes messages, provider/model, workdir, cumulative cost/tokens, and a one-line preview.
- **`/sessions`** — list recent sessions.
- **`/sessions <id>`** — resume a session (restores transcript + provider/model when possible).
- Also available from the **Ctrl+K** palette under category `session`.

### Undo / checkpoints

- When a write is **applied** (review accept, or Act mode), a snapshot of the previous content is stored under `~/.codeforge/checkpoints/<session-id>/`.
- **`/undo`** restores the **last** written file from its checkpoint.

This complements—not replaces—git. Prefer git commits for permanent history.

### Git helpers

If the workdir is a git repository (CodeForge may init one if missing):

| Command | Effect |
|---------|--------|
| `/status` | Show branch + working tree status; refresh file glyphs |
| `/commit [msg]` | `git add -A` + commit (optional message) |
| `/push` | `git push -u origin HEAD` |
| `/pull` | `git pull` (ff-only, then plain pull fallback) |

---

## GitHub integration

### What you can do

| Capability | Slash command | Agent tool action |
|------------|---------------|-------------------|
| Auth / identity | `/gh auth` | `auth_status` |
| Repo metadata | `/gh repo` | `repo_view` |
| List / view PRs | `/pr list` · `/pr view [n]` | `pr_list` · `pr_view` |
| Create PR | `/pr create <title> [| body]` | `pr_create` |
| Merge PR | `/pr merge <n> [squash\|merge\|rebase]` | `pr_merge` |
| CI checks | `/pr checks [n]` | `checks` |
| Issues | `/issue list` · `/issue view` · `/issue create` | `issue_*` |
| Push / pull | `/push` · `/pull` | `push` · `pull` |
| Branch | `/gh branch [name]` | `branch_create` |
| Log | `/gh log` | `log` |

### End-to-end: ship a feature like an AI agent

```text
1. /mode plan                    # safe writes
2. /act implement feature X using search_replace/apply_patch
3. Review overlay → Enter        # apply patches
4. /commit feat: implement X
5. /push
6. /pr create feat: implement X | ## Summary …
7. /pr babysit --fix           # poll CI; on failure auto-agent-fix
```

Or in one agent turn:

```text
/act implement the change with search_replace, run tests, commit, push,
     open a PR, then babysit checks until green (fix and push if red)
```

### Surgical edits

Prefer agent tools:

| Tool | Use when |
|------|----------|
| `search_replace` | Exact old→new text (unique match or `replace_all`) |
| `apply_patch` | Multi-hunk / multi-file CodeForge patch format |
| `write_file` | New files or full rewrites only |

### Multi-root monorepo

In `~/.config/codeforge/config.yaml`:

```yaml
workspace:
  extra_roots:
    - ../shared-lib
    - /abs/path/to/package
  # optional override:
  # ignore_dirs: [node_modules, vendor, dist]
```

Paths resolve against primary workdir first, then extra roots. Grep skips secrets (`.env`, `*.pem`) and heavy dirs by default.

### PR babysit

```text
/pr babysit              # current branch PR
/pr babysit 42           # PR #42
/pr babysit 42 --fix    # on failure → agent fix loop
```

Also via agent: `github` action `babysit` / `babysit_once`.

### Project rules (AGENTS.md)

Place any of these in the project root (merged if several exist):

- `AGENTS.md` · `CLAUDE.md` · `CODEFORGE.md`
- `.codeforge/rules.md` · `.cursorrules` · `.github/copilot-instructions.md`

```text
/rules          # show loaded rules in chat
```

Rules are injected into every chat + agent system prompt.

### Codebase intelligence

```text
/index                              # stats
/act where is authentication handled?
# agent uses codebase_search → read_file → …
```

### MCP servers

```yaml
# ~/.config/codeforge/config.yaml
mcp:
  servers:
    - name: filesystem
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
```

Tools appear as `mcp_<server>_<tool>` for the agent.

### Cost budget

```yaml
budget:
  max_cost_usd: 2.0
  warn_at_usd: 1.0
```

```text
/budget
```

When the cap is hit, chat/agent submits are blocked until config is raised.

### Headless / CI mode (Tier-3)

```bash
# Human-readable
codeforge agent "run go test ./... and fix failures"

# Machine-readable (CI)
codeforge agent --json --workdir . "run go test ./internal/... "
echo $?   # 0 ok, 1 agent/tool failure

# Plan mode (stage writes — not applied)
codeforge agent --plan --json "propose a patch for README typos"
```

GitHub Actions example:

```yaml
- name: CodeForge agent
  env:
    GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}
  run: |
    codeforge agent --json --act "run go test ./... and summarize failures" | tee agent-out.json
```

### Plugins

Drop a YAML file into `~/.codeforge/plugins/` (see `examples/plugins/echo.plugin.yaml`):

```yaml
name: mytool
description: Does a thing
command: /path/to/binary
args: []
workdir_relative: true
```

Appears to the agent as `plugin_mytool`. Extra dirs: `plugins.dirs` in config or `CODEFORGE_PLUGIN_DIR`.

### Session sync (laptop ↔ VPS)

```bash
export CODEFORGE_SESSIONS_DIR="$HOME/Sync/codeforge-sessions"
codeforge session list
codeforge session export 20260716-101500 ./backup.json
codeforge session import ./backup.json
codeforge session export-all ./all-sessions/
```

### Telemetry (opt-in)

Default **off**. Enable local JSONL only:

```yaml
telemetry:
  enabled: true
  local_only: true
```

```bash
export CODEFORGE_TELEMETRY=1
# events → ~/.codeforge/telemetry/events.jsonl
# never includes source code or prompt text
```

### Architecture note

```text
internal/app/        shared bootstrap (TUI + headless)
internal/headless/   CI agent runner (--json)
internal/plugin/     YAML command plugins
internal/telemetry/  opt-in local analytics
internal/rules/      AGENTS.md loader
internal/index/      offline codebase index
internal/redact/     secret redaction
internal/github/     gh + babysit
internal/workspace/  multi-root sandbox
internal/tool/       agent tools
internal/agent/      tool loop + progress
```

---

## Keybindings reference

### Global

| Key | Action |
|-----|--------|
| `Ctrl+C` | Quit (session is saved) |
| `Ctrl+L` | Clear terminal screen |
| `q` | Quit from NORMAL (session is saved) |
| `?` | Show help text in chat |

### NORMAL mode

| Key | Action |
|-----|--------|
| `i` | INSERT (empty chat input) |
| `I` | INSERT with `/act ` prefilled |
| `/` | INSERT with `/` prefilled |
| `:` | COMMAND line |
| `Ctrl+K` | Command palette |
| `Shift+P` (`P`) | Toggle Plan ↔ Act |
| `1` / `2` / `3` | Focus Chat / Diff / Files |
| `Tab` / `Shift+Tab` | Cycle panes |
| `j` `k` / arrows | Scroll chat (or Diff navigation) |
| `g` / `G` | Top / bottom of chat |
| `PgUp` / `PgDn` · `Ctrl+U` / `Ctrl+D` | Page scroll |
| `n` / `p` | Next / previous file tab in Diff pane |

### INSERT mode

| Key | Action |
|-----|--------|
| Type | Edit multi-line prompt |
| `Enter` | Send chat **or** run slash command if line starts with `/` |
| `Esc` | Back to NORMAL |
| `@` | Open file mention picker |
| `Ctrl+K` | Open palette |
| `↑` / `↓` | Input history (previous prompts) |

### Review mode

See [Review overlay](#review-overlay).

---

## Slash commands reference

Type in INSERT (prefix `/`) or via `:` / palette. Aliases in parentheses.

### Agent & code

| Command | Description | Example |
|---------|-------------|---------|
| `/act` (`/a`) | Free-form agent task with tools | `/act add retries to HTTP client` |
| `/read` (`/r`) | Read and display a file | `/read internal/agent/agent.go` |
| `/ls` (`/list`) | List a directory | `/ls cmd` |
| `/grep` (`/find`) | Search project with regex | `/grep TODO` |
| `/run` | Run a shell command in project root (via agent) | `/run go test ./...` |
| `/explain` (`/e`) | Deep explanation of a file | `/explain main.go` |
| `/fix` | Find and fix bugs in a file | `/fix handler.go` |

### Provider & session

| Command | Description | Example |
|---------|-------------|---------|
| `/provider` (`/p`) | List or switch provider | `/provider claude` |
| `/model` (`/m`) | List or switch model | `/model gemini-2.5-pro` |
| `/mode` | Show or set Plan/Act | `/mode plan` · `/mode act` |
| `/sessions` | List or resume sessions | `/sessions` · `/sessions 20260716-101500` |
| `/undo` | Restore last applied write | `/undo` |
| `/cost` (`/c`) | Session tokens, cost, duration | `/cost` |
| `/clear` | Clear chat + start a fresh session id | `/clear` |

### Git & GitHub

| Command | Description |
|---------|-------------|
| `/status` (`/s`) | Local git status |
| `/commit [msg]` | Stage all + commit |
| `/push` | Push current branch to origin |
| `/pull` | Pull from remote |
| `/gh` … | GitHub hub (`/gh help` for full list) |
| `/pr` … | Pull requests (list/view/create/merge/checks) |
| `/issue` … | Issues (list/view/create) |

### Meta

| Command | Description |
|---------|-------------|
| `/help` (`/h` `/?`) | In-app help |
| `/about` | Version / author / stack |
| `/quit` (`/q` `/exit`) | Exit CodeForge |

Unknown `/…` strings are forwarded to the **agent** as a task.

**Tab** in the command line autocompletes known slash commands.

---

## CLI flags

```text
codeforge [workdir] [flags]

  workdir          Optional project directory (default: current directory)

  --no-motion      Disable animations (slow SSH / Termux)
  --skip-wizard    Skip first-run setup wizard
  -y, --yes        Same as --skip-wizard
  -h, --help       Print CLI help
  -v, --version    Print version
```

Examples:

```bash
codeforge
codeforge ~/src/myapp
codeforge --skip-wizard --no-motion ~/src/myapp
```

---

## Environment variables

| Variable | Purpose |
|----------|---------|
| `GEMINI_API_KEY` | Google Gemini |
| `ANTHROPIC_API_KEY` | Anthropic Claude |
| `OPENAI_API_KEY` | OpenAI or compatible API |
| `OPENAI_BASE_URL` | Override API base (default `https://api.openai.com/v1`) |
| `OLLAMA_HOST` | Ollama base URL (default `http://127.0.0.1:11434`) |
| `OLLAMA_MODEL` | Default Ollama model (default `llama3.2`) |
| `GITHUB_TOKEN` / `GH_TOKEN` | GitHub REST auth (optional if `gh auth login` is done) |
| `CODEFORGE_THEME` | `aurora` (default) or `light` |
| `CODEFORGE_NO_MOTION` | `1` / `true` disables motion |
| `CODEFORGE_PLAIN_MD` / `CODEFORGE_NO_GLAMOUR` | Skip rich markdown (faster / leaner) |
| `NERD_FONT` / `NERD_FONTS` | Prefer Nerd Font file/git glyphs |

Optional smaller binary (no glamour/chroma at compile time):

```bash
CGO_ENABLED=0 go build -tags plainmd -ldflags="-s -w" -o codeforge ./cmd/codeforge/
```

---

## Configuration files

| Path | Purpose |
|------|---------|
| `~/.config/codeforge/config.yaml` | Default provider, theme, git, permissions (example created on first run) |
| `~/.codeforge/theme.yaml` | Optional color token overrides |
| `~/.codeforge/sessions/*.json` | Saved conversations |
| `~/.codeforge/checkpoints/<session-id>/` | Pre-write file snapshots for `/undo` |

Example `config.yaml` keys (see generated file for full template):

```yaml
default_provider: gemini
theme: aurora
permissions:
  require_confirm_write: true   # Plan-style staging when true
  require_confirm_shell: true
  require_confirm_push: true
git:
  auto_commit: true
  commit_style: conventional
  branch_prefix: ai/
```

---

## Typical workflows

### 1. Ask about the codebase

```text
i
@ → pick main.go
Explain the control flow of this file
Enter
```

### 2. Safe agent edit (Plan mode)

```text
Confirm status bar shows PLAN (or /mode plan)
I
fix the nil pointer in internal/foo.go and add a unit test
Enter
… watch tools in Chat, pending diff in Diff …
Complete review: j/k · Space · Enter to apply
/commit          # optional
```

### 3. Fast iteration (Act mode)

```text
Shift+P          # → ACT (writes immediate)
/act rename Foo to Bar across the package
/undo            # if the last write was wrong
```

### 4. Resume yesterday’s session

```text
/sessions
/sessions 20260715-183022
```

### 5. Offline / local model

```bash
ollama serve
ollama pull qwen2.5-coder
export OLLAMA_MODEL=qwen2.5-coder
codeforge --skip-wizard
# then: /provider ollama
```

### 6. Open a PR from the TUI

```bash
gh auth login   # once
cd my-repo && codeforge --skip-wizard
```

```text
/gh branch feat/my-change
/act implement the change …
# review + apply if Plan mode
/commit feat: my change
/push
/pr create feat: my change | ## Summary\n- …\n\n## Test plan\n- [ ] …
/pr checks
```

---

## Architecture

```text
cmd/codeforge/          CLI entry, wizard, provider registration
internal/
  agent/                Tool-calling agent loop (events → TUI)
  provider/             Gemini · Claude · OpenAI · Ollama · MCP scaffold
  tool/                 Registry + sandboxed tools + StagedWriter + github tool
  github/               gh CLI + REST client (PRs, issues, checks, push)
  git/  diff/  config/  Supporting core
  theme/                Design tokens (single source of color truth)
  keymap/               Central keybindings
  session/              Persist / resume conversations
  checkpoint/           Local undo snapshots
  ui/
    components/         Panel, toast, badges
    markdown/           Glamour wrapper
    diffview/           Rich diff renderer
    palette/            Fuzzy command palette
    filepicker/         @file picker
    review/             Multi-file Plan review UI
  tui/                  Bubble Tea orchestrator (chat, panes, routing)
```

Design principles (Neo-Forge / Terminal Glass):

- **Depth over flatness** — elevation via surface/border tokens  
- **Motion carries meaning** — not decoration; kill-switch available  
- **Color is status language** — cyan AI · violet agent · emerald success · rose danger · amber attention  
- **Trust before write** — Plan mode default  

Strategy document: [`CODEFORGE_STRATEGY.md`](./CODEFORGE_STRATEGY.md).

---

## Development & tests

```bash
git clone https://github.com/NanoMindExplorer/codeforge.git
cd codeforge
go mod tidy

# Unit + smoke tests
go test ./...

# Build
CGO_ENABLED=0 go build -ldflags="-s -w" -o codeforge ./cmd/codeforge/

# Run against this repo
export GEMINI_API_KEY=...
./codeforge --skip-wizard .
```

CI (GitHub Actions): on push/PR runs `go test ./...` and a CGO-free build; version tags can trigger GoReleaser (see [`.goreleaser.yaml`](./.goreleaser.yaml)).

---

## Distribution

| Artifact | Location |
|----------|----------|
| Install script | [`install.sh`](./install.sh) |
| GoReleaser config | [`.goreleaser.yaml`](./.goreleaser.yaml) |
| CI workflow | [`.github/workflows/ci.yml`](./.github/workflows/ci.yml) |

Release matrix (intended): `linux/amd64`, `linux/arm64` (Termux), `darwin/arm64`, `windows/amd64`.

---

## Troubleshooting

| Symptom | What to try |
|---------|-------------|
| “Provider config” / no API key | Export `GEMINI_API_KEY` (or another provider). Re-run without empty keys. Use `--skip-wizard` once configured. |
| Empty / hanging stream | Check network and key validity. Gemini free tier has rate limits. |
| Agent can’t see files outside project | By design — tools are sandboxed to the workdir. |
| Writes don’t appear on disk | You are likely in **PLAN** mode — finish the **review** overlay (`Enter` to apply). Or switch `/mode act`. |
| Want to reverse a write | `/undo` for last applied file; or use git. |
| TUI feels laggy (SSH / phone) | `codeforge --no-motion` or `CODEFORGE_NO_MOTION=1`. |
| Icons look broken | Unset Nerd Font env, or install a Nerd Font and set `NERD_FONT=1`. |
| Ollama not listed | Ensure `ollama serve` is up; check `OLLAMA_HOST`. |
| Custom OpenAI proxy fails | Verify `OPENAI_BASE_URL` has no trailing slash issues; must expose `/chat/completions`. |
| Binary large (~21MB) | Expected with Glamour/Chroma; still pure Go / no CGO. |
| `/gh auth` fails | Run `gh auth login` or export `GITHUB_TOKEN` with `repo` scope. |
| `/pr create` fails | Ensure branch is pushed (`/push`), remote is GitHub, and you have permission. |
| Checks empty | Need `gh` CLI for best CI rollup; open PR first. |

---

## License & credits

**Apache License 2.0** — see [`LICENSE`](./LICENSE).

**Created by NanoMind — 2026**

Stack: Go · [Bubble Tea](https://github.com/charmbracelet/bubbletea) · Bubbles · Lipgloss · Glamour · go-git · Viper · and friends.

> *Terminal AI coding companion — open, modular, vendor-neutral — and it feels like the future.*
