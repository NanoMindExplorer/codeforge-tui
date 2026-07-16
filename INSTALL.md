# Install CodeForge TUI

## Install matrix

| Platform | Command | Verify |
|----------|---------|--------|
| Linux / macOS | `curl -fsSL https://raw.githubusercontent.com/NanoMindExplorer/codeforge/main/install.sh \| sh` | `codeforge version` |
| From source | `make build` | matches `VERSION` |
| Termux | `bash contrib/termux/build.sh` | `codeforge version` |
| Homebrew | `Formula/codeforge.rb` or tap after release | `codeforge version` |
| Pin release | `CODEFORGE_VERSION=v1.9.0 sh install.sh` | tag match |

## One-liner

```bash
curl -fsSL https://raw.githubusercontent.com/NanoMindExplorer/codeforge/main/install.sh | sh
```

Detects OS/arch, prefers GitHub Releases, falls back to build-from-source.

## Termux (Android)

```bash
pkg install -y golang git curl
# recommended:
curl -fsSL https://raw.githubusercontent.com/NanoMindExplorer/codeforge/main/install.sh | sh
# or from clone:
git clone https://github.com/NanoMindExplorer/codeforge.git && cd codeforge
bash contrib/termux/build.sh

echo 'export XAI_API_KEY=…' >> ~/.bashrc   # or GEMINI_API_KEY
source ~/.bashrc
codeforge --no-motion --compact   # recommended on slow devices
# optional: export CODEFORGE_SSH_TUNE=1
```

Full Termux notes: [`contrib/termux/README.md`](./contrib/termux/README.md).

Upgrade from v0.8 sessions:

```bash
codeforge session migrate
# see docs/SESSION_MIGRATION.md
```

## Ubuntu / Debian

```bash
sudo apt install -y golang-go git
git clone https://github.com/NanoMindExplorer/codeforge.git
cd codeforge
go mod tidy
CGO_ENABLED=0 go build -ldflags="-s -w" -o codeforge ./cmd/codeforge/
sudo mv codeforge /usr/local/bin/
export GEMINI_API_KEY="AIzaSy..."
codeforge
```

## macOS

```bash
brew install go git   # if needed
git clone https://github.com/NanoMindExplorer/codeforge.git
cd codeforge
CGO_ENABLED=0 go build -ldflags="-s -w" -o codeforge ./cmd/codeforge/
sudo mv codeforge /usr/local/bin/
```

## Get free Gemini API key

1. Visit https://aistudio.google.com/apikey  
2. Sign in with Google  
3. Create API Key (`AIzaSy...`)  
4. `export GEMINI_API_KEY=...`

## Optional providers

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export OPENAI_API_KEY=sk-...
export OPENAI_BASE_URL=https://api.openai.com/v1   # or Groq/Together/etc.
# Local offline:
ollama pull llama3.2
export OLLAMA_MODEL=llama3.2
```

## First-run wizard

On first launch without a valid key, CodeForge shows the **setup wizard**  
(provider pick → paste key → validate → default model). State is stored in  
`~/.codeforge/onboarding.json` so the second launch does not re-prompt after skip/complete.

Skip with `--skip-wizard` (or `/setup` later inside the TUI).

### Key priority

| Priority | When |
|----------|------|
| 1 | `XAI_API_KEY` / `GROK_API_KEY` → **grok** |
| 2 | `GEMINI_API_KEY` → gemini |
| 3 | `config.yaml` `default_provider` |
| 4 | Other registered providers (claude / openai / ollama) |

Override anytime: `/provider <name>`. Inspect sources: `/provider` (no args).

### Headless / CI (O7)

```bash
# Happy path
export XAI_API_KEY=…   # or GEMINI_API_KEY
codeforge agent --json "summarize this repo"

# No provider configured → exit 2 + structured JSON
unset XAI_API_KEY GROK_API_KEY GEMINI_API_KEY ANTHROPIC_API_KEY OPENAI_API_KEY
codeforge agent --json "hi"
# {
#   "ok": false,
#   "code": "no_provider",
#   "error": "No AI provider configured",
#   "hint": "Set XAI_API_KEY / GEMINI_API_KEY or run codeforge TUI /setup"
# }
echo $?   # → 2
```

| Exit | Meaning |
|------|---------|
| 0 | Success |
| 1 | Runtime / agent failure |
| 2 | Config: `no_provider` or `auth` |

## Config

- `~/.config/codeforge/config.yaml` — providers, permissions, theme  
- `~/.codeforge/theme.yaml` — color token overrides  
- `~/.codeforge/sessions/` — saved conversations  
- `~/.codeforge/checkpoints/` — file undo snapshots  

## Key bindings (short)

| Key | Action |
|-----|--------|
| i | INSERT mode |
| Esc | NORMAL mode |
| Ctrl+K | Command palette |
| @ | Mention file |
| Shift+P | Plan / Act |
| /act task | Agent mode |
| q | Quit |

Created by NanoMind — 2026
