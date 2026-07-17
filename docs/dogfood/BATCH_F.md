# Dogfood Batch F — Terminal matrix (W4 / Q5.7)

See also [`../TERMINAL_MATRIX.md`](../TERMINAL_MATRIX.md).

## Automated smoke

```bash
make smoke-matrix
# or: bash scripts/smoke-matrix.sh
```

Validates `codeforge version` under color/env variants (no full TUI).

**Q5.7 status:** automated smoke covers **10 env variants** (see table). Manual visual rows remain optional field polish.

## Matrix (automated = version/binary smoke)

| Environment | Command / env | Pass? | Notes |
|-------------|----------------|-------|-------|
| Truecolor desktop | default | ✅ auto | `scripts/smoke-matrix.sh` |
| 256-color | `CODEFORGE_COLOR=256` | ✅ auto | |
| 16-color | `CODEFORGE_COLOR=16` | ✅ auto | |
| Monochrome a11y | `NO_COLOR=1` | ✅ auto | |
| Color none | `CODEFORGE_COLOR=none` | ✅ auto | |
| SSH slow link | `CODEFORGE_SSH_TUNE=1` | ✅ auto | |
| Compact | `CODEFORGE_COMPACT=1` | ✅ auto | |
| No motion | `CODEFORGE_NO_MOTION=1` | ✅ auto | |
| Minimal chrome | `CODEFORGE_MINIMAL=1` / `--minimal` | ✅ auto | |
| Plain markdown | `CODEFORGE_PLAIN_MD=1` | ✅ auto | |
| Termux | `bash contrib/termux/build.sh` + `--no-motion` | ⬜ field | device optional |

## Exit criteria

- [x] Automated smoke green (≥ 5/8 equivalent — we cover 10)  
- [x] No panic / unreadable version path on 16-color or NO_COLOR  
- [ ] Optional: live TUI visual on real Termux / SSH  

## Related product checks

| Check | Pass? |
|-------|-------|
| `/doctor` shows color level + sandbox | ✅ unit/doctor |
| Footer readable in NO_COLOR | ⬜ field visual |
| Theme picker hides truecolor-only on 16-color | ⬜ field visual |
