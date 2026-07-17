# CodeForge → Grok 4.5 Parity Roadmap

**Goal:** Make CodeForge feel and behave **1:1** with Grok Build TUI (Grok 4.5 class) in layout, interaction, session lifecycle, permissions, and agent surface — without becoming a closed fork of proprietary code.

**Current baseline:** CodeForge **v1.9.3**  
- Phase 0–9 + **G1–G10** + **W1–W4** (release automation, onboarding, packaging, release gate)  
- Remaining Could: Grok.com billing/OAuth, welcome-screen polish

**Reference:** Grok user-guide docs (`~/.grok/docs/user-guide/`) — theming, shortcuts, sessions, plan mode, permissions, agent ACP.

---

## How to read this plan

| Term | Meaning |
|------|---------|
| **Parity** | Same *user-visible behavior* and *mental model*, not line-for-line source |
| **Phase** | Shipable slice: each ends with demo checklist + tests green |
| **Must** | Required for “1:1 enough to dogfood as Grok replacement for coding” |
| **Should** | Strongly expected by Grok users |
| **Could** | Nice-to-have / later polish |

Phases are **sequential** where later ones depend on earlier UI foundations. Some platform work (Phase 5–6) can parallelize after Phase 2.

---

## Gap map (today vs Grok)

| Area | Grok 4.5 | CodeForge v0.8 | Gap |
|------|----------|----------------|-----|
| Layout | Scrollback + bottom prompt + footer | ✅ Approximate | Medium (padding, sticky, scrollbar) |
| Themes | GrokNight/Day, Tokyo, RosePine, Oscura, auto | ✅ Phase 3 | Small |
| Blocks | Foldable tool/diff/thinking/prompt blocks | Flat lines + accent bar | **Large** |
| Focus/Esc | Double-Esc clear/rewind, steal-Esc stack | Basic Tab/Esc | Medium |
| Vim scrollback | Optional `j/k` fold, turn jumps | Partial scroll keys | Medium |
| Slash UX | Instant autocomplete menu | Hint strip only | Medium |
| `@` | Path + line ranges + gitignore | Fuzzy file list | Medium |
| Sessions | UUID dirs, resume picker, fork, rewind, compact | ✅ Phase 4 v2 layout | Small |
| Plan mode | Read-only plan.md + approval UI | ✅ Phase 5 DESIGN/BUILD/YOLO | Small |
| Permissions | allow/deny/ask + modes + hooks | ✅ Phase 6 | Small |
| TODOs | Task badges in footer | ✅ Phase 7 | — |
| Thinking | Animated reasoning blocks | Spinner only | Medium |
| ACP / IDE | `grok agent stdio` / serve | ✅ Phase 8 stdio + WebSocket | Small |
| Sandbox | OS sandbox for shell | ✅ Phase G4 soft+bwrap profiles | Small (no process-wide Landlock) |
| Compact / minimal | Full + compact + minimal | ✅ Phase 3 | — |

---

## Phase 0 — Freeze the contract (1–2 days)

**Purpose:** Agree what “1:1” means so work doesn’t sprawl.

### Deliverables
- [x] This roadmap accepted (Must / Should / Could tagged per feature)
- [x] Golden **interaction checklist** → `docs/DOGFOOD.md`
- [ ] Screenshot board: Grok vs CodeForge (optional maintainer assets)
- [x] Non-goals: OS sandbox, Grok.com OAuth, full `x.ai/*` ACP, full pager.toml

### Exit criteria
Team (or you) can answer “done for v1.0 Grok-parity” with a yes/no checklist → **yes for Must/Should shipped; dogfood is ongoing**.

---

## Phase 1 — Scrollback engine (foundation)  **Must**

**Purpose:** Replace flat chat lines with a real **block list** like Grok’s pager.

### Work items
1. **Block model** (`internal/tui/blocks/`)
   - Types: `UserPrompt`, `Assistant`, `ToolCall`, `ToolResult`, `Diff`, `System`, `Thinking` (stub OK)
   - Each block: id, parent turn id, collapsed bool, raw + rendered cache
2. **Viewport over blocks** (not raw strings)
   - Select block with ↑/↓ (simple) or j/k (vim mode flag)
   - `h`/`l` or Left/Right: collapse / expand selected
   - `E`: expand/collapse all
3. **Sticky user headers** when scrolling past a prompt
4. **Follow-tail** when streaming (auto jump bottom; break on manual scroll)
5. **Scrollbar** optional thumb (theme `scrollbar_*`)

### Exit criteria
- [ ] Streaming assistant + multi-step tools appear as separate foldable blocks  
- [ ] Collapse a tool block → diamond header only; expand restores body  
- [ ] Manual scroll does not fight auto-follow; `G` resumes follow  
- [ ] Smoke + golden string tests for 3 block kinds  

**Depends on:** nothing  
**Unlocks:** Phase 2–3 visuals, Phase 4 plan UI  

---

## Phase 2 — Grok input & focus fidelity  **Must**

**Purpose:** Keyboard and composer feel identical to Grok simple mode (+ optional vim).

### Work items
1. **Focus stack / steal-Esc**  
   Overlay → slash menu → @ picker → palette → clear/rewind semantics  
2. **Double-Esc (800ms)**  
   - Non-empty prompt → clear (save to history)  
   - Empty + history → open **rewind picker** (Phase 4 can flesh storage)  
3. **Ctrl+C policy** (match Grok)  
   Running turn: clear draft first, second cancels agent; never confused with Esc  
4. **Prompt history** ↑/↓ when empty or at edges  
5. **`[ui].vim_mode`** config + `/vim-mode`  
6. **Slash autocomplete menu** (list above prompt, fuzzy, Enter/Tab complete)  
7. **`@` picker**  
   - Respect `.gitignore`  
   - Support `@path:10-50` line ranges  
   - `!` prefix for hidden files  

### Exit criteria
- [ ] User who knows Grok shortcuts needs no CodeForge cheatsheet for input  
- [ ] Slash and @ menus feel “instant” (<16ms key response target)  
- [ ] Automated teatest for Esc×2 and Tab focus  

---

## Phase 3 — Theme & chrome parity  **Should** (visually 1:1)

**Purpose:** Look like GrokNight / GrokDay on real terminals.

### Work items
1. Full **Grok color slots** (map names from theming doc):  
   `accent_thinking`, `accent_running`, `md_*`, scrollbar, selection_border, …
2. Built-in themes: **GrokNight, GrokDay, TokyoNight, RosePineMoon, OscuraMidnight** + `auto`
3. **`/theme` picker** with live preview (list overlay)
4. **Truecolor / 256 / 16** quantization helper
5. **OSC 12** cursor → `accent_user`; reset OSC 112 on exit
6. **Compact mode** padding matrix (match Grok outer_vpad / hpad)
7. Optional **`--minimal`**: no chrome, terminal-native 16 colors
8. Syntax theme selection per UI theme (reuse glamour styles; optional chroma tmTheme later)

### Exit criteria
- [x] Side-by-side screenshots indistinguishable at a glance on truecolor  
- [x] `/theme` + env `CODEFORGE_THEME=auto` documented  

**Shipped in v0.9.2** — themes package + themepicker + quantize + OSC + `--minimal`.

---

## Phase 4 — Session lifecycle (Grok sessions)  **Must**

**Purpose:** Sessions as durable, rewindable work units.

### Work items
1. Storage layout closer to Grok:
   ```
   ~/.codeforge/sessions/<encoded-cwd>/<session-id>/
     summary.json
     updates.jsonl      # event stream
     chat_history.jsonl
     plan.md / plan.json
     rewind_points.jsonl
   ```
2. **`/new`** (alias `/clear` behavior split: clear vs new session id)
3. **`/resume`** full-screen session picker (preview, cwd, time)
4. **`/fork`** branch conversation
5. **`/rewind`** picker + restore files from rewind points (integrate checkpoint)
6. **`/compact`** + auto-compact at N% context (config threshold)
7. **`/context`** token breakdown UI
8. **`/session-info`**
9. Headless + TUI share the same session writer

### Exit criteria
- [x] Kill terminal → `codeforge` → `/resume` → identical scrollback blocks  
- [x] Rewind undoes last agent file writes from that turn  
- [x] Export/import still work against new layout  

**Shipped in v0.9.3** — `~/.codeforge/sessions/<encoded-cwd>/<id>/` with summary + jsonl,
`/resume` `/new` `/fork` `/rewind` `/compact` `/context` `/session-info`, headless session writer.

---

## Phase 5 — Plan mode (Grok plan, not just write Plan)  **Must**

**Purpose:** Planning is a **read-only phase** with approval — not only staged writes.

### Work items
1. Session modes cycle: **Normal → Plan → Always-approve (Act/yolo) → Normal** (`Shift+Tab`)
2. In **Plan (design)**:
   - Deny write tools (except `plan.md` in session dir)
   - Agent explores + writes plan file  
3. **`/plan`**, **`/view-plan`**
4. **Approval surface** on exit plan:
   - Scroll plan, `a` approve, `s` request changes, `q` quit plan  
5. After approve → implementation turn (existing Act/Plan-write gate still applies)
6. Keep **write Plan/Act** naming clear in UI:  
   - Footer: `DESIGN` vs `BUILD` vs `YOLO` (or Grok labels)

### Exit criteria
- [x] User can force “design only” and never touch disk until `a`  
- [x] Approval UI matches Grok keybindings for plan review  

**Shipped in v0.9.4** — Shift+Tab BUILD→DESIGN→YOLO; write gate plan.md only;
`write_plan` / `exit_plan_mode` tools; `/plan` `/view-plan`; approval a/s/q.

---

## Phase 6 — Permissions, hooks, safety  **Must** for trust parity

**Purpose:** Grok’s allow/deny/ask pipeline.

### Work items
1. Config:
   ```toml
   # or yaml
   permission_mode = "default" | "plan" | "always_approve" | "dont_ask"
   [[permissions]]
   tool = "run_command"
   pattern = "rm *"
   effect = "deny" | "ask" | "allow"
   ```
2. Authorization order: hooks → deny → ask → allow → remembered → defaults  
3. Read-only auto-approve list (read/grep/list/search)  
4. Interactive **ask modal** (y/n/always for session)  
5. **PreToolUse / PostToolUse hooks** (scripts like Grok)  
6. Dangerous command list never uses “remember”  
7. Optional: OS sandbox research spike (bubblewrap/seatbelt) — Could for Phase 6.1  

### Exit criteria
- [x] `deny run_command(rm)` hard-blocks  
- [x] `ask` prompts once; “always” remembered per project  
- [x] Plan design mode still cannot write code files  

**Shipped in v0.9.5** — `internal/permission` engine + hooks, ask modal y/n/a/d,
`/permissions` `/hooks`, headless `--always-approve` / `--dont-ask`.

---

## Phase 7 — Agent surface & product commands  **Should**

**Purpose:** Same verbs users type in Grok.

### Work items
1. Slash parity set: `/new` `/resume` `/compact` `/context` `/fork` `/rewind` `/copy` `/theme` `/vim-mode` `/settings` `/plan` `/todos`
2. **TODO/task list** block + footer badge `2/5`
3. **Thinking blocks** (if provider streams reasoning; else synthetic “planning…” block)
4. **Diff blocks** in scrollback (not only side pane) with expand/collapse
5. **Fullscreen block viewer** (Enter on selected block)
6. **Copy** `y` block / `Y` metadata  
7. Background task list (long shell) with cancel  
8. Align headless flags with Grok: `--always-approve`, `--model`

### Exit criteria
- [x] Slash menu covers ≥90% of Grok’s daily commands  
- [x] Diff appears inline under tool write like Grok  

**Shipped in v0.9.6** — `/todos` `/tasks` `/settings` `/copy`, thinking + inline
diff blocks, Enter viewer, y/Y copy, bg shell, `--model`.

---

## Phase 8 — ACP / IDE bridge  **Should** for ecosystem 1:1

**Purpose:** Editors talk to CodeForge like Grok agent mode.

### Work items
1. `codeforge agent stdio` — JSON-RPC ACP subset  
   - session/new, session/load, prompt, cancel  
   - stream tool calls + text  
2. `codeforge agent serve --bind --secret` (WebSocket)  
3. Document Zed/Neovim client wiring  
4. Map ACP permissions to Phase 6 engine  

### Exit criteria
- [x] Minimal ACP client can run a turn and show tools  
- [x] CI test with scripted stdio client  

**Shipped in v0.9.7** — `codeforge agent stdio` / `serve`, JSON-RPC subset,
permissions mapped, `docs/ACP.md`, `internal/acp` tests with fake runner.

---

## Phase 9 — Polish & dogfood  **Must** before claiming 1:1

### Work items
1. Side-by-side dogfood week: same tasks in Grok and CodeForge  
2. Performance: 10k-line scrollback, 60fps animations optional  
3. Termux / 16-color / SSH matrix  
4. Accessibility: NO_COLOR, reduce motion  
5. Migration guide from CodeForge v0.8 sessions → Phase 4 layout  
6. Tag **v1.0.0-grok-parity** only when Phase 0 checklist is green  

### Exit criteria
- [x] Dogfood checklist documented (`docs/DOGFOOD.md`)  
- [x] Public README “Grok-compatible TUI” with honest remaining Coulds  
- [x] Performance: viewport O(visible) + body cap + auto-collapse  
- [x] Termux / color / SSH matrix documented (`docs/TERMINAL_MATRIX.md`)  
- [x] Accessibility: `NO_COLOR`, reduce motion, SSH tune  
- [x] Session migration: `codeforge session migrate` + `docs/SESSION_MIGRATION.md`  
- [x] Version **v1.0.0** + integration smoke tests  

**Shipped in v1.0.0** — polish, a11y, migrate, docs, integration package.

---

## Suggested calendar (solo maintainer)

| Phase | Effort (solo) | Cumulative |
|-------|---------------|------------|
| 0 Contract | 1–2 d | 2 d |
| 1 Blocks | 1–2 w | ~2.5 w |
| 2 Input/focus | 1 w | ~3.5 w |
| 3 Theme | 3–5 d | ~4.5 w |
| 4 Sessions | 1.5–2 w | ~6.5 w |
| 5 Plan design | 1 w | ~7.5 w |
| 6 Permissions | 1.5–2 w | ~9.5 w |
| 7 Agent surface | 1–1.5 w | ~11 w |
| 8 ACP | 1.5–2 w | ~13 w |
| 9 Dogfood | 2 w | **~15 w** |

Parallelize: Phase 3 after Phase 1; Phase 8 after Phase 6.

---

## Execution order we will follow in-repo

```
Phase 0–9  COMPLETE for v1.0.0 Grok-compatible claim
  1 blocks · 2 input · 3 theme · 4 sessions · 5 design plan
  6 permissions · 7 product surface · 8 ACP · 9 polish
```

---

## Definition of “1:1 done”

All of the following are true:

1. A Grok user can use CodeForge for a full day **without a cheatsheet**.  
2. Scrollback is **block-native** (fold, select, sticky prompt).  
3. Sessions **resume / rewind / compact** reliably.  
4. **Design-plan** and **permission ask/deny** protect the disk.  
5. Headless + (ideally) ACP cover automation.  
6. Visual theme **GrokNight** matches at a glance on truecolor.

Out of scope for “1:1” claim (Could forever):
- Grok.com billing/OAuth  
- Proprietary Grok model stack  
- Every `pager.toml` knob  
- Full OS sandbox on all platforms  

---

## Next action

**Phase 0–9 + G1–G10 + W1–W4 are shipped** on `main` (baseline **v1.9.0**).

Remaining work is **field dogfood** (fill scorecard / batches A–F) and optional **Coulds** (Grok.com OAuth, welcome polish). When ready to publish binaries:

```bash
make release-gate
git tag v1.9.0 && git push origin v1.9.0
```

When implementing further changes, each PR should:
- Keep this living checklist honest (or link to CHANGELOG)
- Keep `make ci` / `make release-gate` green  
- Run `make fmt` (or rely on the pre-commit hook)

---

## Living checklist

> **Status sync:** last updated for **v1.9.3** + audit phases **Q0–Q3**.  
> Detail for each phase is in the body of this doc; this section is the at-a-glance status only.  
> Post-parity quality track: see [`AUDIT_AND_ROADMAP.md`](./AUDIT_AND_ROADMAP.md) (Q0 CI · Q1 agent · Q2 TUI · **Q3 secrets/config**).

### Phase 0 — Contract
- [x] Roadmap written
- [x] Checklist accepted
- [x] Non-goals documented in this file

### Phase 1 — Scrollback engine
- [x] `internal/tui/blocks` — Block model + Store
- [x] Foldable tool/user/assistant/diff/system/thinking kinds
- [x] Select j/k · fold h/l · E expand-all
- [x] Follow-tail + G resume; manual scroll breaks follow
- [x] Sticky user header when scrolled past prompt
- [x] Scrollbar thumb
- [x] ChatModel wired to block store (streaming + agent tools)
- [x] Unit tests (fold, follow, sticky, stream)
- [ ] Dogfood screenshot vs Grok (optional / field)

### Phase 2 — Input & focus fidelity
- [x] Steal-Esc stack (review → palette → @ → command → slash → clear/rewind)
- [x] Double-Esc 800ms clear prompt (history) + rewind hint
- [x] Ctrl+C: clear draft → cancel turn → quit
- [x] Cancelable stream/agent context
- [x] Slash autocomplete menu (fuzzy, Tab/Enter)
- [x] @ picker: gitignore, !hidden, path:line-range
- [x] `/vim-mode` + config `ui.vim_mode`
- [x] Prompt history on clear
- [x] Tests (slashmenu, filepicker, smoke)

### Phase 3 — Theme system
- [x] GrokNight / GrokDay tokens + truecolor
- [x] Quantize 256 / 16 / NO_COLOR
- [x] `/theme` picker · auto dark/light
- [x] Compact / minimal / no-motion / SSH tune

### Phase 4 — Sessions
- [x] v2 session layout under `~/.codeforge/sessions/`
- [x] `/resume` `/new` `/fork` `/rewind` `/compact` `/context`
- [x] `codeforge session migrate|export|import`
- [x] Checkpoints / undo for writes

### Phase 5 — Design plan
- [x] BUILD / DESIGN / YOLO session modes (Shift+Tab)
- [x] `/plan` · `write_plan` · plan review overlay
- [x] DESIGN write-gate (plan.md only)

### Phase 6 — Permissions & hooks
- [x] Permission engine (allow/deny/ask + modes)
- [x] Shell ask modal · dangerous-command rules
- [x] PreToolUse / PostToolUse hooks
- [x] OS sandbox profiles (soft / bwrap / Landlock / Seatbelt best-effort)

### Phase 7 — Product surface
- [x] `/todos` · `/tasks` · background shell
- [x] Block viewer · copy · inline diff
- [x] `/settings` · memory · GitHub slash surface

### Phase 8 — ACP / headless
- [x] `codeforge agent` one-shot + `--json`
- [x] `agent stdio` / `agent serve` ACP JSON-RPC
- [x] `x.ai/*` extensions · skills on ACP path

### Phase 9 — Polish & dogfood
- [x] `docs/DOGFOOD.md` + terminal matrix
- [x] Session migration docs
- [x] v1.0.0 Grok-compatible claim foundations
- [ ] Field dogfood 2-week green (ongoing — see scorecard)

### G1–G10 — Grok 4.5 tools & model
- [x] Grok 4.5 provider (xAI OpenAI-compatible)
- [x] Grok tool names / aliases · spawn_subagent
- [x] Skills (SKILL.md) · personas · background subagents
- [x] Native reasoning streams · pager.toml matrix
- [x] Subagent persist · permission parity audit (v1.8.1)

### W1–W4 — Release readiness
- [x] **W1** Release automation + ProviderError + dogfood templates (v1.8.2)
- [x] **W2** Onboarding `/setup` + reasoning fallback + headless codes (v1.8.3)
- [x] **W3** Install matrix · Termux package · release-notes (v1.8.4)
- [x] **W4** `make release-gate` · `/doctor` · scorecard · Batch F (v1.9.0)
- [ ] Tag **v1.9.0** + GitHub Release assets (when publishing)
- [ ] Human dogfood H1–H5 filled in [RELEASE_GATE.md](./RELEASE_GATE.md)

---

*Maintainer: NanoMind · CodeForge · 2026*  
*Companion to product code at **v1.9.0** (phases 0–9 + G1–G10 + W1–W4). Older “v0.8.0+” wording is historical only.*
