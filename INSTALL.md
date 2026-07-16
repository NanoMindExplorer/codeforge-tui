# Install CodeForge TUI

## One-liner

```bash
curl -fsSL https://raw.githubusercontent.com/NanoMindExplorer/codeforge/main/install.sh | sh
```

Detects OS/arch, prefers GitHub Releases, falls back to build-from-source.

## Termux (Android)

```bash
pkg install -y golang git
git clone https://github.com/NanoMindExplorer/codeforge.git
cd codeforge
go mod tidy
CGO_ENABLED=0 go build -ldflags="-s -w" -o codeforge ./cmd/codeforge/
cp codeforge $PREFIX/bin/

echo 'export GEMINI_API_KEY=AIzaSy...' >> ~/.bashrc
source ~/.bashrc
codeforge --no-motion   # recommended on slow devices
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

On first launch without a valid key, CodeForge shows a 3-step wizard  
(keys → provider tip → keybindings). Skip with `--skip-wizard`.

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
