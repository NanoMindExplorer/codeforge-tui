# Release gate — v1.9.0 (W4)

Public-ready bar for CodeForge after W1–W3 foundations.

## Automated (must pass before tag)

Run:

```bash
make release-gate
# or: bash scripts/release-gate.sh
```

| # | Check | Tool |
|---|--------|------|
| G1 | Version SSOT consistent | `make check-version` |
| G2 | Unit/integration tests | `go test ./...` |
| G3 | Vet + build + `codeforge version` | `make ci` |
| G4 | Binary size &lt; 30 MiB | release-gate |
| G5 | Packaging files present (install, Formula, Termux, release.yml) | release-gate |
| G6 | CHANGELOG section for VERSION | release-gate |
| G7 | Headless `no_provider` exit 2 | release-gate |
| G8 | Terminal env smoke (color/flags) | `scripts/smoke-matrix.sh` |

## Human / field (dogfood)

| # | Check | Target | Tracker |
|---|--------|--------|---------|
| H0 | Automated dogfood suite | `make dogfood` FAIL=0 | [dogfood/RESULTS.md](./dogfood/RESULTS.md) |
| H1 | Dogfood **A** core loop | 100% field | [DOGFOOD.md](./DOGFOOD.md) · [PROGRAM.md](./dogfood/PROGRAM.md) |
| H2 | Dogfood **B–C** session + safety | 100% | [dogfood/BATCH_BC.md](./dogfood/BATCH_BC.md) |
| H3 | Dogfood **D–E** Grok surface + ACP | ≥ 80% | [dogfood/BATCH_DE.md](./dogfood/BATCH_DE.md) |
| H4 | Dogfood **F** terminal matrix | matrix filled | [dogfood/BATCH_F.md](./dogfood/BATCH_F.md) |
| H5 | Parity scorecard | 1 page filled | [dogfood/SCORECARD.md](./dogfood/SCORECARD.md) |
| H6 | First-run ≤3 min (spot-check) | ≥4/5 without README | manual |
| H7 | No raw JSON/stack for 401/429/model in TUI | manual | ProviderError W1–W2 |
| H8 | Homebrew/Termux install path once | 1 machine each | INSTALL.md |

## Publish runbook

```bash
# 1. Automated gate
make release-gate

# 2. Ensure VERSION + CHANGELOG match
cat VERSION   # e.g. 1.9.0

# 3. Tag (triggers .github/workflows/release.yml → GoReleaser)
git tag v1.9.0
git push origin v1.9.0

# 4. After assets publish, fill Formula sha256
bash scripts/update-formula.sh v1.9.0
# commit formula if in-repo

# 5. Smoke install
curl -fsSL …/install.sh | sh
codeforge version   # → 1.9.0
```

## Honest limitations (ship with release notes)

- OS sandbox process isolation is best-effort (Landlock/Seatbelt may be `none` in containers).
- Grok.com billing/OAuth is out of scope (API key only).
- Homebrew sha256 is filled **after** first release assets exist.
- Dogfood field scores (H1–H5) are maintainer-owned; automated gate does not fake them.

## Decision

| Outcome | Version |
|---------|---------|
| Automated green + H1–H3 green | **v1.9.0** public-ready |
| Automated green, field incomplete | ship as **v1.9.0-rc** or keep main untagged |
| P0 regressions | patch **v1.8.x** before tag |
