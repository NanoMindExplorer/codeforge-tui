# Dogfood checklist (Phase 9)

Use this side-by-side with Grok Build TUI for a day of real work. Mark each item when CodeForge feels “good enough.”

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

- Process-wide Landlock/Seatbelt at startup (G4 ships per-command bwrap + soft path policy)  
- Full Grok `x.ai/*` ACP extension methods — subset only  
- Provider reasoning streams as real thought tokens — synthetic thinking OK  
- Full Grok welcome screen / pager.toml matrix — partial  

When this checklist stays green for **two weeks of daily use**, the v1.0 “Grok-compatible” claim is fair.
