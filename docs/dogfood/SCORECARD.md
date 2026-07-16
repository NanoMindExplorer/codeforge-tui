# Parity scorecard — CodeForge vs Grok Build TUI

**Version under test:** v1.9.0  
**Date:** 2026-07-16  
**Tester:** automated baseline + maintainer harness (`make dogfood`)  
**Live provider:** Gemini 2.5 Flash  

Rate each area: **Better / Equal / Weaker / N/A / Unknown (needs field)**

| Area | Score | Notes |
|------|-------|-------|
| First-run / API setup | Equal* | `/setup`, wizard, key sources (*field UX TBD) |
| Chat + streaming | Unknown | Headless stream/think ok; TUI chrome field TBD |
| Agent multi-step edits | Equal* | Live headless read+write success |
| BUILD / DESIGN / YOLO | Equal* | Unit + dogfood tests green |
| Permissions & hooks | Better* | Default deny rm -rf even in always-approve |
| Sessions (resume/fork/rewind) | Equal* | API green; crash-resume field TBD |
| Skills / personas / subagents | Unknown | Registered; interactive path TBD |
| ACP / IDE | Equal* | initialize ok; multi-turn IDE TBD |
| Provider errors UX | Equal* | structured codes + FormatUserError |
| Theme / terminal matrix | Equal* | smoke-matrix green; visual TBD |
| Packaging / install | Equal | matrix + termux scripts |
| GitHub PR/CI loop | Unknown | not exercised this baseline |

\* = automated/live evidence only, not 10-day human dogfood.

## Top 5 strengths

1. Write-mode safety (BUILD stage / DESIGN gate / YOLO explicit)  
2. Dangerous shell deny that survives always-approve  
3. Headless JSON contract (`no_provider` exit 2) for CI  
4. Grok tool surface registered (spawn_subagent, web_*, …)  
5. Measurable dogfood harness (`make dogfood` → RESULTS.md)  

## Top 5 gaps (P0/P1)

| Pri | Gap | Owner | Status |
|-----|-----|-------|--------|
| P1 | Interactive TUI dogfood days not complete | maintainer | open — PROGRAM.md |
| P1 | Side-by-side Grok Build days 3/7/10 | maintainer | open |
| P1 | Crash mid-task `/resume` field proof | maintainer | open |
| P2 | ACP multi-turn from real IDE | maintainer | open |
| P2 | Termux on-device visual matrix | maintainer | open |

## Verdict

- [x] Engineering baseline ready (automated dogfood FAIL=0)  
- [ ] Ready for public **“1:1 Grok daily driver”** claim  
- [x] Needs **field program** (10 days) before that claim  
- [ ] Needs patch first  

**Would use CodeForge over Grok for daily coding?** Sometimes — headless/CI yes; full TUI parity still field-pending.

**Summary:**  
v1.9.0 has solid **mechanized** evidence for agent edits, safety, sessions API, ACP init, and terminal env smoke. That removes speculation for the engine. The **1:1 Grok TUI** claim remains **not proven** until PROGRAM.md days are filled with real interactive sessions and a completed scorecard side-by-side.
