# Terminal matrix (Termux / SSH / color)

## Recommended flags

| Scenario | Flags / env |
|----------|-------------|
| Desktop truecolor | default `CODEFORGE_THEME=groknight` |
| SSH / high latency | `--no-motion --compact` or `CODEFORGE_SSH_TUNE=1` |
| Termux / phone | `CGO_ENABLED=0` build, `--no-motion` |
| 16-color only | `CODEFORGE_COLOR=16` |
| Monochrome a11y | `NO_COLOR=1` (forces minimal palette + no motion) |
| No chrome | `--minimal` |
| Plain markdown (lean RAM) | `CODEFORGE_PLAIN_MD=1` or `-tags plainmd` |

## Color levels

| Level | Detection | Themes |
|-------|-----------|--------|
| truecolor | `COLORTERM=truecolor` | All |
| 256 | `TERM=*256*` | All (quantized) |
| 16 | basic ANSI | GrokNight/Day/Aurora preferred; truecolor-only themes hidden in `/theme` picker |
| none | `NO_COLOR` or `CODEFORGE_COLOR=none` | Minimal / monochrome |

## Automated smoke (Batch F)

```bash
make smoke-matrix
```

Manual dogfood sheet: [dogfood/BATCH_F.md](./dogfood/BATCH_F.md). Health dump: `codeforge doctor` or `/doctor`.

## Termux install (summary)

```bash
pkg install -y golang git
git clone https://github.com/NanoMindExplorer/codeforge.git && cd codeforge
bash contrib/termux/build.sh
export XAI_API_KEY=…   # or GEMINI_API_KEY
codeforge --no-motion --compact
```

See also [INSTALL.md](../INSTALL.md) and [contrib/termux/README.md](../contrib/termux/README.md).

## Build matrix (release)

GoReleaser targets: `linux/amd64`, `linux/arm64` (Termux), `darwin/amd64`, `darwin/arm64`, `windows/amd64`.
