# Dogfood checklist (Phase 9 + field program)

Use this side-by-side with Grok Build TUI for real work.  
**Program:** [dogfood/PROGRAM.md](./dogfood/PROGRAM.md) (10 working days)  
**Latest automated evidence:** [dogfood/RESULTS.md](./dogfood/RESULTS.md) · `make dogfood`

**Daily log:** copy [`dogfood/TEMPLATE.md`](./dogfood/TEMPLATE.md) → `dogfood/YYYY-MM-DD.md`  
**Batches:** [B–C](./dogfood/BATCH_BC.md) · [D–E](./dogfood/BATCH_DE.md) · [F](./dogfood/BATCH_F.md)  
**Scorecard:** [dogfood/SCORECARD.md](./dogfood/SCORECARD.md)  
**Release gate:** [RELEASE_GATE.md](./RELEASE_GATE.md)

### Status legend

| Mark | Meaning |
|------|---------|
| ✅ | Passed (automated evidence and/or logged field session) |
| 🧪 | Passed **automated / headless** only — TUI UX not yet field-signed |
| ⬜ | Not yet done (needs human session) |
| ❌ | Failed |

**Baseline run:** 2026-07-16 · CodeForge **v1.9.0** · `make dogfood` → **PASS=13 FAIL=0 HUMAN=12**  
Live model: Gemini (env key). Host: Linux/aarch64. Details: [RESULTS.md](./dogfood/RESULTS.md).

---

## Core coding loop (Batch A)

| Task | Grok | CodeForge | Evidence |
|------|------|-----------|----------|
| Open project, type a question | ⬜ | 🧪 | Live ping `DOGFOOD_PING`; TUI still HUMAN |
| `@` attach file, get answer | ⬜ | ⬜ | HUMAN — TUI `@` picker |
| `/act` multi-step edit | ⬜ | 🧪 | Live agent: read_file+write_file on sample main.go |
| BUILD staged write → review apply | ⬜ | 🧪 | `TestDogfood_A_BUILD_StagedWrite` / staged unit tests |
| YOLO immediate write | ⬜ | 🧪 | `TestDogfood_A_YOLO_ImmediateWrite` |
| DESIGN `/plan` → approve | ⬜ | 🧪 | DESIGN blocks project writes; WritePlan ok |
| `/undo` last write | ⬜ | 🧪 | Checkpoint path covered lightly; full TUI undo HUMAN |
| `/commit` + `/push` | ⬜ | ⬜ | HUMAN — needs real git remote |

**Batch A automated slice:** ✅ green · **Full TUI field:** ⬜ in progress (Day 1–3 of program)

---

## Session lifecycle (Batch B)

| Task | Pass? | Evidence |
|------|-------|----------|
| Kill terminal mid-task → `/resume` | ⬜ | HUMAN crash-resume |
| `/fork` branch conversation | 🧪 | `session.Fork` integration |
| `/rewind` (or 2× Esc) restore files | 🧪 | rewind points API; Esc UX HUMAN |
| `/compact` long thread | ⬜ | HUMAN |
| `session migrate` after upgrade from v0.8 | 🧪 | migrate package + CLI exists |

---

## Theme / terminal matrix (Batch F)

| Environment | Command | Pass? | Evidence |
|-------------|---------|-------|----------|
| Truecolor desktop | default | ⬜ | HUMAN visual |
| 256-color | `CODEFORGE_COLOR=256` | 🧪 | smoke-matrix + theme tests |
| 16-color / basic | `CODEFORGE_COLOR=16` | 🧪 | smoke-matrix |
| NO_COLOR a11y | `NO_COLOR=1` | 🧪 | smoke-matrix + parity test |
| SSH slow link | `CODEFORGE_SSH_TUNE=1` | 🧪 | smoke env; real SSH HUMAN |
| Termux | `contrib/termux/build.sh` | 🧪 | build path; device HUMAN |
| Minimal chrome | `--minimal` | 🧪 | smoke-matrix |

---

## Permissions & safety (Batch C)

| Task | Pass? | Evidence |
|------|-------|----------|
| `rm -rf` denied by default rule | ✅ | permission tests + always-approve still denies |
| Shell ask modal y/n/a | ⬜ | HUMAN interactive |
| Hook PreToolUse deny (exit 2) | ✅ | hooks unit + dogfood test |
| DESIGN blocks project writes | ✅ | ModeDesign + WritePlan tests |

---

## Automation (Batch E)

| Task | Pass? | Evidence |
|------|-------|----------|
| `codeforge agent --json "…"` | ✅ | live agent ok=true, tools read/write |
| `codeforge agent --always-approve --model …` | ✅ | used in live dogfood run |
| `codeforge agent stdio` (scripted) | 🧪 | initialize JSON-RPC ok; multi-turn HUMAN/IDE |
| `codeforge agent serve --secret …` health | ⬜ | not run this baseline |
| no key → exit 2 `no_provider` | ✅ | dogfood-run + headless tests |

---

## Product surface (Batch D / Phase 7)

| Task | Pass? | Evidence |
|------|-------|----------|
| `/todos` badge in footer | 🧪 | todos.Badge unit path |
| Enter block viewer · y copy | ⬜ | HUMAN |
| Inline diff under write tools | 🧪 | staged diff in unit tests |
| `/tasks` background shell | ⬜ | HUMAN |
| `/settings` toggles | ⬜ | HUMAN |
| skills / personas / subagents | 🧪 | tools registered; UI HUMAN |
| reasoning streams | 🧪 | live agent returned thinking event |

---

## How to re-run evidence

```bash
make dogfood                 # auto + live if API key set
DOGFOOD_LIVE=0 make dogfood  # offline only
```

Artifacts: `docs/dogfood/RESULTS.md`, `docs/dogfood/results.json`, `docs/dogfood/runs/`.

---

## Honest remaining gaps (Could)

- Landlock/Seatbelt best-effort on restricted kernels/containers  
- Grok.com billing/OAuth — out of scope  
- Homebrew Formula sha256 only after GitHub Release assets  
- **Interactive TUI dogfood days 1–10** still the bar for full 1:1 Grok claim  

### Claim language (honest)

| Claim | Status after this baseline |
|-------|----------------------------|
| “Engineered for Grok-compatible workflows” | ✅ Supported (modes, tools, ACP, safety) |
| “1:1 Grok in daily TUI use” | ⬜ **Not yet** — needs PROGRAM.md field days |
| Headless/CI agent ready | ✅ Supported with live evidence |

When batches **A–C** are field-green and **D–F** ≥80% per PROGRAM.md, update SCORECARD verdict and README claim.
