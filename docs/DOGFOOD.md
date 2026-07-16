# Dogfood checklist (Phase 9)

Use this side-by-side with Grok Build TUI for a day of real work. Mark each item when CodeForge feels “good enough.”

**Daily log:** copy [`docs/dogfood/TEMPLATE.md`](./dogfood/TEMPLATE.md) → `docs/dogfood/YYYY-MM-DD.md` and fill it after each session.  
**Local helper:** `make dogfood-help`

## Core coding loop

| Task | Grok | CodeForge | Notes |
|------|------|-----------|-------|
| Open project, type a question | | | |
| `@` attach file, get answer | | | |
| `/act` multi-step edit | | | |
| BUILD staged write → review apply | | | |
| YOLO immediate write | | | |
| DESIGN `/plan` → approve `a` | | | |
| `/undo` last write | | | |
| `/commit` + `/push` | | | |

## Session lifecycle

| Task | Pass? |
|------|-------|
| Kill terminal mid-task → `/resume` | |
| `/fork` branch conversation | |
| `/rewind` (or 2× Esc) restore files | |
| `/compact` long thread | |
| `session migrate` after upgrade from v0.8 | |

## Theme / terminal matrix

| Environment | Command | Pass? |
|-------------|---------|-------|
| Truecolor desktop | default | |
| 256-color | `CODEFORGE_COLOR=256` | |
| 16-color / basic | `CODEFORGE_COLOR=16` | |
| NO_COLOR a11y | `NO_COLOR=1` | |
| SSH slow link | `CODEFORGE_SSH_TUNE=1` or `--no-motion --compact` | |
| Termux | `--no-motion` build `CGO_ENABLED=0` | |
| Minimal chrome | `--minimal` | |

## Permissions & safety

| Task | Pass? |
|------|-------|
| `rm -rf` denied by default rule | |
| Shell ask modal y/n/a | |
| Hook PreToolUse deny (exit 2) | |
| DESIGN blocks project writes | |

## Automation

| Task | Pass? |
|------|-------|
| `codeforge agent --json "…"` | |
| `codeforge agent --always-approve --model …` | |
| `codeforge agent stdio` (scripted/test) | |
| `codeforge agent serve --secret …` health | |

## Product surface

| Task | Pass? |
|------|-------|
| `/todos` badge in footer | |
| Enter block viewer · y copy | |
| Inline diff under write tools | |
| `/tasks` background shell | |
| `/settings` toggles | |

## Honest remaining gaps (Could)

- Landlock/Seatbelt best-effort on restricted kernels/containers (see process=none in status)  
- Grok.com billing/OAuth — out of scope  

When this checklist stays green for **two weeks of daily use**, the v1.0 “Grok-compatible” claim is fair.
