# Dogfood Batch F — Terminal matrix (W4)

See also [`../TERMINAL_MATRIX.md`](../TERMINAL_MATRIX.md).

## Automated smoke

```bash
make smoke-matrix
# or: bash scripts/smoke-matrix.sh
```

Validates `codeforge version` under color/env variants (no full TUI).

## Manual matrix (fill Pass?)

| Environment | Command / env | Pass? | Notes |
|-------------|----------------|-------|-------|
| Truecolor desktop | default | | |
| 256-color | `CODEFORGE_COLOR=256` | | |
| 16-color | `CODEFORGE_COLOR=16` | | |
| Monochrome a11y | `NO_COLOR=1` | | |
| SSH slow link | `CODEFORGE_SSH_TUNE=1` or `--no-motion --compact` | | |
| Termux | `bash contrib/termux/build.sh` + `--no-motion` | | |
| Minimal chrome | `--minimal` | | |
| Plain markdown | `CODEFORGE_PLAIN_MD=1` | | |

## Exit criteria

- Automated smoke green  
- ≥ 5/8 manual rows pass on real terminals  
- No panic / unreadable UI on 16-color or NO_COLOR  

## Related product checks

| Check | Pass? |
|-------|-------|
| `/doctor` shows color level + sandbox | |
| Footer readable in NO_COLOR | |
| Theme picker hides truecolor-only on 16-color | |
