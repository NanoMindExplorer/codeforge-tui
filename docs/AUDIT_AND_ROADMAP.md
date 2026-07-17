# CodeForge comprehensive audit & improvement roadmap

**Audit date:** 2026-07-17  
**Baseline:** `v1.9.3` (`main`)  
**Scope:** entire Go module `github.com/codeforge/tui`, scripts, CI, docs, packaging  

**Q0 status:** implemented — race job, coverage floor (`scripts/coverage-floor.txt` = 34%), offline dogfood CI, govulncheck (warn), claim language links.  
**Q1 status:** implemented — agent unit tests (~88%), LoopError, rate-limit retry, redact, headless codes.  
**Q2 status:** implemented — TUI orchestrator split; `model.go` **~638 LOC** (from ~3.8k); keys/stream/session/slash files + `AppServices` + unit tests.

This document is the single source of truth for **what the codebase is today**, **what hurts**, and **how to improve it in ordered phases**.

---

## 1. Executive summary

| Dimension | Grade | One-line verdict |
|-----------|-------|------------------|
| **Product surface** | **A−** | Grok-parity feature set is unusually complete for an open TUI agent |
| **Release machinery** | **A** | VERSION SSOT, CI gates, GoReleaser, install matrix work |
| **Safety model** | **A−** | Permissions + DESIGN gate + deny-dangerous shell are real |
| **UX of errors / onboarding** | **B+** | Structured errors + multi-provider setup; TUI polish still uneven |
| **Architecture** | **C+** | God-file TUI orchestrator; globals; weak boundaries |
| **Test depth** | **C** | Suite green, but ~34% statement coverage; critical packages untested |
| **Security hygiene** | **B−** | Redaction + sandbox exist; keys in plaintext config; no secret scanning CI |
| **Field proof (dogfood)** | **C** | Automated green; interactive 10-day program incomplete |
| **Observability** | **C** | Optional error log + telemetry stub; no structured tracing |
| **Overall ship readiness** | **B** | Solid early-stable product; not yet “maintainable for a team” |

**Bottom line:** CodeForge is a **feature-rich v1.x** that can already replace a large slice of Grok Build workflows in headless/CI and much of the TUI. After Q2 the TUI orchestrator is **decomposed** (`model.go` ~638 LOC). Remaining risks: **test blind spots** (`app`, config save), **config/secrets** (Q3), and **incomplete interactive dogfood** before a hard “1:1 Grok daily driver” claim.

---

## 2. Quantitative inventory

| Metric | Value |
|--------|-------|
| Go source files | ~181 |
| Test files (`*_test.go`) | ~50 |
| Go LOC (approx.) | **~35,500** |
| `gofmt -l .` dirty files | **0** |
| `go test ./...` | **PASS** (baseline audit run) |
| Statement coverage (approx.) | **~33.7%** total |
| Packages under `internal/` | **34** |
| Packages with **zero** tests | **20** (see §4.2) |
| Largest file (pre-Q2) | `internal/tui/model.go` was **~3,824 LOC** → **~638 LOC** after Q2 |
| Largest TUI files (post-Q2) | `slash.go` ~1.1k · `keys.go` ~1.0k · `chat.go` ~668 |
| Secondary large files | `pager.go` ~840, `tokens.go` ~699, `chat.go` ~668, `errors.go` ~639, `agent.go` ~318 |
| Binary size (CGO=0) | **~23–24 MiB** (under 30 MiB CI gate) |
| Module | `github.com/codeforge/tui` · **Go 1.25** |
| Last public tag | **v1.9.3** (multi-OS assets published) |

### Package map (logical layers)

```text
cmd/codeforge          CLI + TUI entry + wizard
internal/app           Bootstrap (config, providers, tools, index, MCP)
internal/provider      LLM backends + errors + reasoning fallback
internal/agent         Tool-calling loop (Complete only)
internal/tool          Tools + staged writes + subagents
internal/permission    Allow/deny/ask + dangerous rules
internal/session       Persist / fork / rewind / compact
internal/tui + ui/*    Bubble Tea UI (orchestrator + widgets)
internal/acp           IDE protocol (stdio + WS + x.ai/*)
internal/sandbox       Soft / bwrap / Landlock / Seatbelt
internal/onboarding    Multi-provider first-run
scripts/ + .github/    CI, release, dogfood, gofmt hooks
```

---

## 3. Strengths (keep protecting)

1. **End-to-end coding agent stack** — providers, tools, permissions, sessions, ACP, headless JSON.  
2. **Write safety triad** — BUILD (staged) / DESIGN (plan-only) / YOLO — with unit evidence.  
3. **Dangerous shell policy** that still denies `rm -rf` under always-approve.  
4. **Release engineering** — `VERSION`, `check-version`, `release-gate`, GoReleaser, Formula sha256, install.sh.  
5. **ProviderError taxonomy** — user-facing messages without stack/JSON dumps; optional jsonl log.  
6. **Multi-provider onboarding** — ResolveActive priority + wizard paths + `/setup` / `/provider`.  
7. **Grok tool surface** — aliases, spawn_subagent, skills, personas, pager.toml matrix.  
8. **Dogfood harness** — `make dogfood` produces measurable RESULTS (not only empty checklists).  
9. **gofmt discipline** — clean tree + pre-commit + CI gate.  
10. **Docs volume** — ONBOARDING, ERRORS, SANDBOX, ACP, SUBAGENTS, RELEASE_GATE, etc.

---

## 4. Findings (detailed)

Severity: **P0** ship-blocker · **P1** high · **P2** medium · **P3** nice-to-have

### 4.1 Architecture & maintainability

| ID | Sev | Finding | Evidence / impact |
|----|-----|---------|-------------------|
| A1 | **P0** → **mitigated (Q2)** | God orchestrator split: `model.go` shell + `keys.go` / `slash.go` / `slash_github.go` / `stream.go` / `session_ctl.go` / `services.go` | Add slash commands in `slash.go` without editing a 4k file |
| A2 | **P1** | Slash/command handling is a giant switch inside the same file | ~100+ case arms; hard to extend safely |
| A3 | **P1** | Process-wide **globals** (`todos.Global`, `bgtask.Global`, sandbox/workspace globals, `tool.SubagentAuthorizer`) | Hidden coupling; hard to test in parallel; race risk under multi-session ACP |
| A4 | **P1** | `internal/agent` is a single file with **no unit tests** | Core loop untested for tool iteration, auth deny, max-iter, cancel |
| A5 | **P2** | `internal/app.Bootstrap` does too much (index, MCP, plugins, personas, sandbox…) | Slow headless boots; hard to inject fakes |
| A6 | **P2** | Duplicated “system” knowledge (slash lists in `model.go`, help text, README) | Drift (already seen historically with version strings) |
| A7 | **P2** | UI packages under `internal/ui/*` largely untested | Widgets break without CI signal |

### 4.2 Testing & quality gates

| ID | Sev | Finding | Evidence |
|----|-----|---------|----------|
| T1 | **P1** | **~34%** statement coverage overall | `go test -cover` total |
| T2 | **P1** | **No tests:** `agent`, `app`, `config`, `checkpoint`, `git`, `keymap`, `research`, `telemetry`, most `ui/*` | Blind spots on critical paths |
| T3 | **P1** | ACP **websocket** path ~0% coverage; many `xai/*` write paths untested | IDE serve mode fragile |
| T4 | **P2** | Integration dogfood tests are solid but **not** wired as required CI job name/badge | Easy to ignore `make dogfood` |
| T5 | **P2** | No race detector in CI (`go test -race`) | Globals + goroutines (agent, spawn, stream pumps) |
| T6 | **P3** | No fuzzing on permission pattern matching / path sandbox | Security edge cases |

**Packages with zero tests (audit):**  
`cmd/codeforge`, `agent`, `app`, `checkpoint`, `config`, `diff`, `git`, `keymap`, `research`, `telemetry`, `ui/blockview`, `ui/clipboard`, `ui/components`, `ui/diffview`, `ui/markdown`, `ui/palette`, `ui/permask`, `ui/planreview`, `ui/review`, `ui/settings`.

### 4.3 Security & trust

| ID | Sev | Finding | Impact |
|----|-----|---------|--------|
| S1 | **P1** | API keys stored **plaintext** in `~/.config/codeforge/config.yaml` via `SaveProviderKey` | Disk compromise = key theft |
| S2 | **P1** | Config write merges via Viper may **rewrite/partially drop** unrelated keys if not fully loaded | Silent config loss risk |
| S3 | **P2** | YOLO / `--always-approve` is powerful; UX must keep **deny rules** highly visible | User over-trust |
| S4 | **P2** | Shell tool still executes user/model-driven commands; soft sandbox default often `off` | Workspace escape depends on profile |
| S5 | **P2** | Redact package exists but not applied on **all** tool outputs / error raw paths | Occasional secret leakage to model |
| S6 | **P3** | No CI secret scanning / dependency vulnerability gate (`govulncheck`) | Supply chain |
| S7 | **P3** | Telemetry package present — must stay **opt-in default off** and documented | Privacy |

### 4.4 Provider / agent correctness

| ID | Sev | Finding | Impact |
|----|-----|---------|--------|
| L1 | **P1** | Agent loop uses **Complete**, not Stream, for tool turns | Higher latency UX vs streaming-first competitors |
| L2 | **P1** | Claude **Stream** path does not support tools (documented); inconsistency across providers | User surprise |
| L3 | **P2** | Max tool-result truncation (24k) is hard-coded | Large file edits can confuse model |
| L4 | **P2** | Max iterations error is plain `fmt.Errorf` | Less structured than ProviderError |
| L5 | **P2** | No automatic retry for rate_limit (only reasoning unsupported) | Worse recovery vs Grok |
| L6 | **P3** | Cost accounting may drift from real provider billing | Budget false confidence |

### 4.5 TUI / product UX

| ID | Sev | Finding | Impact |
|----|-----|---------|--------|
| U1 | **P1** | Interactive dogfood still **HUMAN**-heavy (attach `@`, review overlay, crash-resume, Termux device) | 1:1 claim incomplete |
| U2 | **P1** | Welcome / ASCII art / system flood on first launch may clutter small terminals | First impression |
| U3 | **P2** | Footer multi-key badge good; still easy to miss *which* model pricing/context | Power-user friction |
| U4 | **P2** | Settings / palette / block viewer low test + uneven polish | Perceived unfinished |
| U5 | **P2** | No dedicated “recover last failed turn” affordance | After rate limit / network |
| U6 | **P3** | Localization mixed (ID/EN strings in slash help historically) | Inconsistent voice |

### 4.6 Sessions / durability

| ID | Sev | Finding | Impact |
|----|-----|---------|--------|
| D1 | **P1** | Crash mid-task + `/resume` **not field-proven** in dogfood | Data trust |
| D2 | **P2** | Session storage layout v2 complex; migration tested but edge cwd encodings | Multi-machine sync |
| D3 | **P2** | Checkpoint/undo coverage thin | Accidental data loss fear |
| D4 | **P3** | No encryption at rest for session transcripts | Sensitive code in `~/.codeforge` |

### 4.7 ACP / IDE

| ID | Sev | Finding | Impact |
|----|-----|---------|--------|
| C1 | **P1** | WebSocket server untested | `agent serve` regressions silent |
| C2 | **P2** | Multi-session concurrency vs process globals | Race / cross-talk |
| C3 | **P2** | `codeforge/error` extension not standardized with clients | IDE UX variance |
| C4 | **P3** | Protocol versioning / capability negotiation minimal | Future compatibility |

### 4.8 Performance & resource use

| ID | Sev | Finding | Impact |
|----|-----|---------|--------|
| P1 | **P2** | Bootstrap indexes codebase by default (headless can be heavy) | Slow CI agent starts |
| P2 | **P2** | Binary ~24MB; plainmd builds help Termux | Mobile still heavy |
| P3 | **P2** | Scrollback growth / glamour rendering cost on long sessions | SSH lag |
| P4 | **P3** | No metrics for tool latency / token burn beyond cost footer | Ops blind |

### 4.9 Release / packaging / ops

| ID | Sev | Finding | Impact |
|----|-----|---------|--------|
| R1 | **P2** | Homebrew **tap repo missing**; formula only in-repo + release asset | `brew install` path incomplete |
| R2 | **P2** | Go **1.25** requirement may exclude older distros | Contributor friction |
| R3 | **P3** | Termux package not in official termux-packages | Install still manual |
| R4 | **P3** | Scorecard still says v1.9.0 baseline | Doc drift |

### 4.10 Documentation & claims

| ID | Sev | Finding | Impact |
|----|-----|---------|--------|
| O1 | **P1** | “1:1 Grok daily driver” must stay **qualified** until PROGRAM.md days complete | Credibility |
| O2 | **P2** | Many docs; no single **ARCHITECTURE.md** for contributors | Onboarding devs |
| O3 | **P3** | Dogfood SCORECARD version lag | Confusion |

---

## 5. Risk heat map

```text
                    Impact →
                 Low        Med         High
        High |  P3 docs   U2 clutter   A1 model.go
Likelihood   |  R3 termux  P1 index    T2 no agent tests
        Med  |  U6 i18n    L5 retry    S1 plaintext keys
        Low  |  P4 metrics C4 proto    S7 telemetry
```

**Top 5 risks to address first**

1. **A1** Split `model.go` before more features land.  
2. **T2/A4** Test `agent` + permission + staged write integration.  
3. **S1/S2** Safer config/key storage + non-destructive config write.  
4. **U1/D1** Finish interactive dogfood (resume, review overlay).  
5. **C1/C2** ACP serve concurrency + tests.

---

## 6. Improvement phases (maximal detail)

Naming: **Phase Qx** = Quality/repair track (distinct from historical Grok parity phases 0–9 / W1–W4).

### Phase Q0 — Stabilize & instrument (3–5 days) ✅ **DONE**

**Goal:** No silent production foot-guns; make quality measurable.

| # | Work item | DoD | Status |
|---|-----------|-----|--------|
| Q0.1 | `go test -race` on critical packages | CI job `race` + `make test-race` | ✅ (skips on platforms without race, e.g. android/arm64) |
| Q0.2 | Coverage floor | `scripts/coverage-check.sh` + floor file **33%** + artifact | ✅ |
| Q0.3 | Offline dogfood in CI | job `dogfood-offline` + `make dogfood-offline` | ✅ |
| Q0.4 | SCORECARD / AUDIT stamps v1.9.3 | Docs match tag | ✅ |
| Q0.5 | `govulncheck ./...` warn mode | CI job + `make govulncheck` | ✅ (`continue-on-error` / non-strict) |
| Q0.6 | README claim table + AUDIT/PROGRAM links | No overclaim | ✅ |

**Exit:** CI measures race + coverage floor + offline dogfood.

---

### Phase Q1 — Core loop correctness (1–1.5 weeks) **P0/P1** ✅ **DONE**

**Goal:** The agent/tool/permission path is bulletproof and tested.

| # | Work item | Detail | DoD | Status |
|---|-----------|--------|-----|--------|
| Q1.1 | **`internal/agent` unit tests** | Mock provider: text, tools, auth deny, max iter, cancel, reasoning + rate-limit info | ≥85% agent coverage | ✅ **~88.6%** |
| Q1.2 | **Structured max-iter / cancel errors** | `LoopError` + `UserMessage()` for FormatUserError | Friendly TUI | ✅ |
| Q1.3 | **Rate-limit retry (1×)** | Interruptible sleep; injectable `Config.Sleep` | Mock tests | ✅ |
| Q1.4 | **Permission integration tests** | Alias parity, DESIGN, hooks in agent loop | `q1_test.go` | ✅ |
| Q1.5 | **Staged write E2E** | BUILD stage → apply → checkpoint undo | Integration | ✅ |
| Q1.6 | **Redact on tool outputs** | `redact.Redact` on forModel path | Unit test | ✅ |
| Q1.7 | **Headless error codes** | `mapAgentError` for all codes + loop codes | `codes_test.go` | ✅ |

**Exit:** Core loop covered; no known untested auth/tool iteration path.

---

### Phase Q2 — Decompose TUI orchestrator (2–3 weeks) **P0 architecture** ✅ **DONE**

**Goal:** `model.go` becomes a thin shell.

Implemented as same-package file split (zero behavior change; methods stay on `Model`):

| File | Responsibility | ~LOC |
|------|----------------|------|
| `internal/tui/model.go` | Types, `New`/`Init`/`Update`/`View`, layout | **~638** |
| `internal/tui/keys.go` | `handleKeyMsg` + modal updaters (Esc stack, Shift+Tab, palette…) | ~1.0k |
| `internal/tui/slash.go` | `executeSlashCommand` + help/autocomplete | ~1.1k |
| `internal/tui/slash_github.go` | `/gh` `/pr` `/issue` handlers | ~440 |
| `internal/tui/stream.go` | Agent/stream pump + msg types + tokens/budget | ~340 |
| `internal/tui/session_ctl.go` | resume/fork/rewind/compact/session pickers | ~360 |
| `internal/tui/services.go` | `AppServices` DI over todos/bgtask/sandbox/skills/personas | ~70 |

| # | Work item | DoD | Status |
|---|-----------|-----|--------|
| Q2.1 | Extract slash handlers without behavior change | `model.go` &lt; 2.5k LOC | ✅ **~638** |
| Q2.2 | Extract key routing | Focused tests for Esc stack / Shift+Tab | ✅ `keys_test.go` |
| Q2.3 | Extract stream/agent message handling | Pump logic unit-tested | ✅ `stream_test.go` |
| Q2.4 | Dependency injection for globals used by TUI | Optional `AppServices` | ✅ `services.go` + helpers |
| Q2.5 | `model.go` target **&lt; 1.2k LOC** | Measured | ✅ **~638 LOC** |

**Exit:** Contributors can add a slash command in `slash.go` without editing a 4k file.

---

### Phase Q3 — Config, secrets, bootstrap (1–1.5 weeks) **P1 security**

| # | Work item | Detail | DoD |
|---|-----------|--------|-----|
| Q3.1 | **Non-destructive config write** | Load full file → merge → write; preserve unknown keys/comments if possible (or yaml node merge) | Round-trip test |
| Q3.2 | **Key storage policy** | Prefer env; config key optional; document risk; chmod 0600 on config | File mode test |
| Q3.3 | **Optional OS keyring** (Could) | Store API keys in keyring when available | Feature flag |
| Q3.4 | **Bootstrap options** | `SkipIndex` default true for headless unless needed; faster agent | Bench or timing test |
| Q3.5 | **Config schema validation** | Reject unknown sandbox profiles / invalid modes early | Clear error |
| Q3.6 | **app package tests** | Bootstrap with temp HOME + fake keys registration | No network |

**Exit:** Config safe to edit programmatically; headless boots faster by default.

---

### Phase Q4 — Session durability & recovery (1–2 weeks) **P1**

| # | Work item | DoD |
|---|-----------|-----|
| Q4.1 | Crash-resume scenario test (kill mid-save simulation) | Deterministic test |
| Q4.2 | `/resume` UX: list last session for cwd with preview | Manual + unit |
| Q4.3 | Checkpoint coverage for YOLO + BUILD apply | Tests |
| Q4.4 | Compact quality: preserve tool outcomes summary | Snapshot test |
| Q4.5 | Session export includes permissions mode + model | Round-trip |
| Q4.6 | Field dogfood Batch B complete (PROGRAM days 4–5) | SCORECARD update |

**Exit:** User trust for long sessions; Batch B field green.

---

### Phase Q5 — TUI polish & first-run excellence (1.5–2 weeks) **P1/P2**

| # | Work item | DoD |
|---|-----------|-----|
| Q5.1 | First-run layout: ASCII brand + **single** status card (no message flood) | Screenshot / golden string |
| Q5.2 | Empty-state prompts when no key / no project files | Copy reviewed |
| Q5.3 | “Retry last turn” after provider error | Keybinding or prompt chip |
| Q5.4 | Review overlay / `@` attach interactive dogfood | PROGRAM days 1–3 |
| Q5.5 | Consistent English product voice (or full i18n later) | Lint for mixed help strings |
| Q5.6 | Settings panel tests | Basic model tests |
| Q5.7 | Terminal matrix field pass on 5/8 real envs | BATCH_F filled |

**Exit:** First 3 minutes feel intentional; Batch A field ≥80%.

---

### Phase Q6 — ACP / IDE hardening (1–2 weeks) **P1**

| # | Work item | DoD |
|---|-----------|-----|
| Q6.1 | WebSocket serve unit/integration tests | Coverage &gt; 50% on websocket.go |
| Q6.2 | Per-session runtime isolation (no cross-session tool authorizer bleed) | Concurrent test |
| Q6.3 | Document `codeforge/error` payload for IDE clients | ACP.md section |
| Q6.4 | Multi-turn ACP fixture (initialize → session/new → prompt → tool) | Golden JSON-RPC script in CI |
| Q6.5 | Cancel / interrupt semantics | Test |

**Exit:** `agent stdio` and `agent serve` are CI-guarded.

---

### Phase Q7 — Performance & footprint (1 week) **P2**

| # | Work item | DoD |
|---|-----------|-----|
| Q7.1 | Lazy index build; progress UI | Headless default skip remains |
| Q7.2 | Scrollback virtualization audit (already partial) | Long session benchmark |
| Q7.3 | Reduce default binary (build tags / smaller deps review) | Size budget tracked in CI |
| Q7.4 | Stream tool-capable providers where API allows | Design note + optional path |
| Q7.5 | Token/cost accuracy pass | Document formulas |

**Exit:** Noticeably snappier cold start on Termux/SSH.

---

### Phase Q8 — Security hardening (1 week) **P1/P2**

| # | Work item | DoD |
|---|-----------|-----|
| Q8.1 | Default sandbox profile recommendation `workspace` for interactive (opt-out) | Doc + config default discussion |
| Q8.2 | Permission prompt UX shows full command + risk badge | UI test |
| Q8.3 | Audit `run_command` env injection / cwd | Tests |
| Q8.4 | Secret patterns expansion (xai-, huggingface, etc.) | Redact tests |
| Q8.5 | Session dir permissions 0700 | Test |
| Q8.6 | Supply chain: pin actions SHAs; dependabot | Repo settings |

**Exit:** Threat model doc + mitigations listed.

---

### Phase Q9 — Ecosystem & packaging (1 week) **P2**

| # | Work item | DoD |
|---|-----------|-----|
| Q9.1 | Create or drop homebrew-tap (don’t leave half-path) | Either live tap or docs only in-repo Formula |
| Q9.2 | Termux package PR or tracked fork | One-command install doc true |
| Q9.3 | Windows install path polish (scoop/winget Could) | Doc |
| Q9.4 | Go version policy (support N-1) | CI matrix 1.24+1.25 if feasible |
| Q9.5 | Release notes automation from CHANGELOG | Tag workflow posts body |

**Exit:** Install paths match README matrix on 3 platforms.

---

### Phase Q10 — Field dogfood gate → claim upgrade (2 calendar weeks, low eng hours/day) **P1 product**

| # | Work item | DoD |
|---|-----------|-----|
| Q10.1 | Execute PROGRAM.md days 1–10 | Daily logs present |
| Q10.2 | Side-by-side Grok days 3/7/10 | SCORECARD filled |
| Q10.3 | Zero P0 bugs open from dogfood | Issue tracker clean |
| Q10.4 | Update README claim language only if gates pass | Honest marketing |
| Q10.5 | Tag **v1.10.0** or **v2.0.0** after Q1–Q2+Q10 | Release |

**Exit:** “Daily driver” claim is evidence-backed.

---

## 7. Suggested sequencing (12 weeks)

```text
Week 1        Q0 stabilize CI (race, coverage floor, dogfood offline)
Week 1–2      Q1 agent/permission tests + rate-limit retry
Week 2         Q2 split model.go ✅ (same-package files; ~638 LOC shell)
Week 3–4      Q3 config/secrets + bootstrap
Week 4–5      Q4 sessions durability
Week 5–7      Q5 TUI polish + field Batch A/F
Week 6–7      Q6 ACP harden
Week 7–8      Q7 performance
Week 8–9      Q8 security
Week 9–10     Q9 packaging
Week 1–12     Q10 dogfood (parallel human track all along)
Week 12       Claim review + version bump
```

**Parallelism:** Q10 (human) runs continuously; Q2 is the largest eng investment; do not start large features until Q1+Q2 foundations land.

---

## 8. Definition of “maximal quality” (north star)

CodeForge is “done enough” for a strong public claim when:

1. **Coverage ≥ 55%** overall; **agent/permission/session/provider ≥ 75%**.  
2. **`model.go` &lt; 1.2k LOC** with slash modules. ✅ (~638; keys/slash/stream/session_ctl).
3. **Race-clean** critical packages.  
4. **Dogfood PROGRAM** complete; SCORECARD recommends daily use.  
5. **Keys** not required in plaintext (env or keyring).  
6. **ACP stdio + serve** multi-turn CI fixtures green.  
7. **Install matrix** verified on Linux/macOS/Termux after each tag.  
8. **No P0** open &gt; 48h on main.

---

## 9. Explicit non-goals (still)

- Grok.com OAuth / billing  
- Pixel-perfect proprietary Grok chrome  
- Supporting every LLM gateway quirk  
- Becoming a full IDE  

---

## 10. Immediate next action (if implementing next)

Recommended first engineering PR after this audit:

> **Q0.1–Q0.3 + Q1.1** — race job, coverage floor, offline dogfood in CI, and `internal/agent` unit tests with a mock provider.

That yields the highest confidence per hour before any large refactor.

---

## 11. Audit method & limits

**Method:** full tree inventory, `go test`, cover profile, package no-test scan, largest-file analysis, architecture reading of TUI/agent/provider/config/ACP/release paths, cross-check dogfood RESULTS/SCORECARD.

**Limits:** No multi-day interactive TUI dogfood in this audit session; no production traffic metrics; coverage numbers are approximate from one cover run; security is design review not a penetration test.

---

*Maintainer: NanoMindExplorer · CodeForge v1.9.3 · Audit 2026-07-17*
