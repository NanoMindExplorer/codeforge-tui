# Dogfood field results

**Generated:** 2026-07-16T14:50:22Z  
**CodeForge:** v1.9.0  
**Host:** Linux/aarch64  
**Runner:** scripts/dogfood-run.sh

## Score

| PASS | FAIL | SKIP/HUMAN |
|------|------|------------|
| 13 | 0 | 12 |

**Automated pass rate (PASS/(PASS+FAIL)):** 100%  

> Human TUI rows are marked HUMAN (not claimed as pass).  
> Live agent rows require API key (this run: has_key=1).

## Results

| ID | Status | Notes |
|----|--------|-------|
| `meta.version` | **PASS** | codeforge 1.9.0 |
| `meta.doctor` | **PASS** | doctor exit 0 |
| `auto.integration` | **PASS** | integration dogfood tests green |
| `auto.C.rm_rf` | **PASS** | internal/permission -run DenyRmRf\|AlwaysApproveStillDenies |
| `auto.C.hooks` | **PASS** | internal/hooks -run PreToolUseDeny |
| `auto.A.modes` | **PASS** | internal/tool -run StagedWriterPlanMode\|StagedWriterActMode\|DesignModeBlocks |
| `auto.B.session` | **PASS** | internal/session -run SaveLoad\|ForkAndRewind |
| `auto.F.theme` | **PASS** | internal/theme -run . |
| `F.smoke_matrix` | **PASS** | color/env variants |
| `E.no_provider` | **PASS** | exit 2 + code no_provider |
| `E.acp_initialize` | **PASS** | stdio initialize responded |
| `A.live_agent` | **PASS** | headless agent ok (workdir /tmp/tmp.umf9g7IMnL) |
| `A.live_ping` | **PASS** | model responded with DOGFOOD_PING |
| `A.tui_chat` | **HUMAN** | Open project type question (needs interactive TUI) |
| `A.at_attach` | **HUMAN** | @ attach file in TUI |
| `A.review_ui` | **HUMAN** | BUILD review overlay accept/reject in TUI |
| `A.git_push` | **HUMAN** | /commit+/push on real remote |
| `B.resume_crash` | **HUMAN** | Kill terminal mid-task then /resume |
| `B.double_esc_rewind` | **HUMAN** | 2x Esc rewind UX |
| `C.shell_modal` | **HUMAN** | Shell ask modal y/n/a interactive |
| `D.skills_ui` | **HUMAN** | /skills /personas interactive |
| `D.subagents_bg` | **HUMAN** | spawn_subagent background + /subagents UI |
| `D.pager_ui` | **HUMAN** | /pager or pager.toml visual |
| `F.termux_device` | **HUMAN** | Real Termux device |
| `F.ssh_slow` | **HUMAN** | Real SSH high-latency session |

## Mapping to DOGFOOD.md

| Checklist area | Evidence |
|----------------|----------|
| Core coding loop (modes/write) | auto.A.* + A.live_agent |
| Session lifecycle | auto.B.session |
| Permissions & safety | auto.C.* |
| Automation / ACP | E.no_provider, E.acp_initialize, A.live_* |
| Terminal matrix | F.smoke_matrix + auto.F.theme |
| Interactive TUI chrome | HUMAN rows (not auto) |

## Logs

- Live agent transcripts: `docs/dogfood/runs/`
- Doctor: /tmp/cf-dogfood-doctor.txt (ephemeral)

## Verdict

- Automated evidence: **GREEN** (13 passed).
- Interactive TUI / multi-day human dogfood: **still required** for full 1:1 claim.
- Next: fill daily logs from real coding sessions for 10 working days.
