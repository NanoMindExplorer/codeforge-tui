# Changelog

All notable changes to CodeForge are documented here.

Generate a release blurb: `make release-notes` or `bash scripts/release-notes.sh`.  
Automated readiness: `make release-gate` (see [docs/RELEASE_GATE.md](./docs/RELEASE_GATE.md)).

## [1.9.0] — 2026-07-16

### Public-ready gate (W4)

- `docs/RELEASE_GATE.md` — automated + human gate checklist for public release.
- `make release-gate` / `scripts/release-gate.sh` — version, tests, packaging, headless contract, smoke-matrix.
- `make smoke-matrix` — Batch F env/color smoke (`CODEFORGE_COLOR`, `NO_COLOR`, SSH tune, …).
- Dogfood **Batch F** + **parity scorecard** templates.
- **`codeforge doctor`** + **`/doctor`** — keys, model, color level, sandbox, hints (E7).
- Roadmap baseline + honest limitations updated for v1.9.0.

### Shipped foundations (W1–W3 recap)

- Release automation, ProviderError UX, onboarding wizard/`/setup`, packaging matrix, Termux metadata.

## [1.8.4] — 2026-07-16

### Packaging (W3 / R4–R6)

- Termux: `contrib/termux/build.sh` + `package.sh` (TERMUX_PKG_VERSION from `VERSION`).
- README / INSTALL **install matrix** (platform → command → `codeforge version`).
- `scripts/release-notes.sh` + `make release-notes` for CHANGELOG section + commits.
- `install.sh` embeds `ProjectVersion` from `VERSION` on source fallback; clearer post-install hints.
- `check-version` validates termux package metadata emits current VERSION.

### Onboarding docs (W3 / O6–O7)

- Provider priority matrix in README + INSTALL.
- Headless CI contract documented: exit 2 + `code: no_provider` JSON.

### Dogfood

- Batch D–E: `docs/dogfood/BATCH_DE.md` (Grok surface + ACP/IDE).

## [1.8.3] — 2026-07-16

### Onboarding (W2 / O1–O5)

- `~/.codeforge/onboarding.json` tracks completed/skipped first-run (no wizard spam).
- Wizard v2: pick provider → paste key (prefix detect) → ValidateConfig → default model → save config.
- Footer strip: `⚠ no API key · /setup` until a provider validates.
- `/setup` slash (re-run anytime): `/setup <provider> <key> [model]`.
- `/provider` lists key source: `env:XAI_API_KEY` / `config` / `missing`.

### Provider error UX (W2 / E4–E5)

- Reasoning unsupported → one automatic retry with `Reasoning=off` + system notice (agent + stream).
- Headless `--json`: structured `code` + `hint`; exit **2** for `no_provider` / `auth`.
- ACP surfaces `FormatUserError` and `codeforge/error` session updates.

### Dogfood

- Batch B–C checklist: `docs/dogfood/BATCH_BC.md`.

## [1.8.2] — 2026-07-16

### Release automation (W1 / R1–R3)

- `VERSION` is the single source of truth for the product version.
- `scripts/check-version.sh` + `make check-version` / `make ci` gate consistency across `main.go`, README, TUI about, MCP, ACP, and Homebrew Formula.
- `scripts/bump-version.sh` updates all version string locations in one shot.
- `scripts/update-formula.sh` fills Formula sha256 from a published release checksums file.
- CI (`.github/workflows/ci.yml`) runs the version gate before tests/build and smoke-checks `codeforge version`.
- Dedicated release workflow (`.github/workflows/release.yml`) on tag `v*`: tag must match `VERSION`, then GoReleaser publishes.

### Provider error UX (W1 / E1–E3)

- New `provider.ProviderError` with codes: `auth`, `rate_limit`, `quota`, `model`, `context`, `network`, `timeout`, `unsupported`.
- `Classify` / `HTTPError` / `FormatUserError` map HTTP and transport failures to short messages + actionable hints.
- Wired through OpenAI-compatible (incl. Grok), Gemini, Claude, and Ollama Complete/Stream paths.
- TUI stream, agent, and `errMsg` paths render `FormatUserError` instead of raw dump strings.

### Dogfood

- Daily log template at `docs/dogfood/TEMPLATE.md`.
- Checklist remains in `docs/DOGFOOD.md`.

## [1.8.1] — previous

- Permissions, subagent auth, provider config keys, ACP skills + toolCallId, clone parent registry for general subagents (see git history).
