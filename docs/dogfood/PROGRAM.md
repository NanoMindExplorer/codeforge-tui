# Dogfood program — 10 working days

**Goal:** Turn “Grok-compatible” from marketing into **field-proven** evidence.  
**Baseline:** automated suite via `make dogfood` (see [RESULTS.md](./RESULTS.md)).  
**Human track:** daily coding **through CodeForge** for real tasks.

## Rules

1. Prefer CodeForge for agentic coding each day (≥30 min real work).
2. After each session: copy [TEMPLATE.md](./TEMPLATE.md) → `YYYY-MM-DD.md` and fill it.
3. Log bugs as GitHub issues with label `dogfood` (P0/P1/P2 in title).
4. Days 3, 7, 10: same task side-by-side with Grok Build TUI when available.
5. Re-run `make dogfood` at least twice per week (or after any main merge).

## Calendar

| Day | Batch | Focus | Exit |
|-----|-------|--------|------|
| 1 | A + auto | Core loop + run `make dogfood` | Auto green; 1 real `/act` or headless edit |
| 2 | A | BUILD review + YOLO + DESIGN | 8/8 core loop attempted |
| 3 | A side-by-side | Same task in Grok + CodeForge | Notes in scorecard |
| 4 | B | resume / fork / rewind | Session table filled |
| 5 | B | compact + crash mid-task | 5/5 session |
| 6 | C | permissions + hooks + sandbox | 0 false-allow |
| 7 | D side-by-side | skills / personas / subagents / reasoning | ≥1 real task each |
| 8 | D | pager + long session | |
| 9 | E | ACP stdio multi-turn | 1 IDE or script client session |
| 10 | F + scorecard | terminal matrix + final scorecard | H1–H5 decision |

## Commands

```bash
# Measurable evidence (CI-friendly)
make dogfood              # DOGFOOD_LIVE=auto (uses API key if present)
DOGFOOD_LIVE=0 make dogfood   # offline only

# Human session
codeforge --skip-wizard .
# …real work…
# then fill docs/dogfood/YYYY-MM-DD.md
```

## Definition of “lulus lapangan”

| Bar | Requirement |
|-----|-------------|
| **Minimum (engineering)** | `make dogfood` FAIL=0 on main; RESULTS.md committed |
| **Field (product claim)** | Days 1–10 logs present; Batch A–C marked pass; D–F ≥80%; SCORECARD verdict checked |
| **1:1 Grok claim** | Field bar + honest Coulds unchanged in README |

Automated-only green **does not** equal full 1:1. It **does** remove speculation for write modes, safety, sessions (API), ACP, headless, and color matrix.
