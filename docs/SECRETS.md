# Secrets & API key storage (Q3)

CodeForge **prefers environment variables** for API keys. Disk storage is optional convenience for local machines.

## Resolution order

When a provider key is needed:

1. **Environment** — `XAI_API_KEY` / `GROK_API_KEY`, `GEMINI_API_KEY`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`
2. **OS keyring** — if `secrets.backend` is `keyring` or `auto` and the platform keyring works
3. **File keystore** — `~/.config/codeforge/keys/<provider>.key` (mode **0600**)
4. **config.yaml** — `providers.<name>.api_key` (plaintext; discouraged)

Only **one** provider is active at a time; other keys remain available via `/provider`.

## Recommended setups

### CI / production (best)

```bash
export XAI_API_KEY=…
# or GEMINI_API_KEY / ANTHROPIC_API_KEY / OPENAI_API_KEY
codeforge agent --json "…"
```

Never commit keys. Prefer your platform’s secret store (GitHub Actions secrets, etc.).

### Local laptop

```yaml
# ~/.config/codeforge/config.yaml
secrets:
  backend: auto   # try OS keyring, else keys/*.key (0600)
```

Then `/setup grok xai-…` stores outside YAML when possible.

### Env only (strict)

```yaml
secrets:
  backend: env_only
```

`SaveProviderKey` / `/setup <provider> <key>` **refuses** to write secrets to disk. Export env vars instead.

## File permissions

| Path | Mode |
|------|------|
| `~/.config/codeforge/` | **0700** |
| `config.yaml` | **0600** |
| `keys/<provider>.key` | **0600** |

Writes use temp+rename and re-`chmod` after rename.

## Config writes are non-destructive

`SaveDefaultProvider` and `SaveProviderKey` **merge** into the existing YAML map. Unknown top-level keys (extensions, comments-as-siblings after round-trip) are preserved. Changing `default_provider` does **not** dump env-sourced keys into the file.

## Feature flags / env

| Variable | Effect |
|----------|--------|
| `CODEFORGE_SECRETS_BACKEND` | Override `secrets.backend`: `auto` \| `file` \| `keyring` \| `env_only` |
| `CODEFORGE_NO_KEYRING=1` | Disable OS keyring even when backend is auto/keyring |
| `CODEFORGE_CONFIG_DIR` | Override config directory (tests / portable installs) |
| `CODEFORGE_INDEX=1` | Force codebase index on headless/ACP boots that skip it |

## Risk note

Keys in **config.yaml** are **plaintext**. Mode 0600 reduces local multi-user risk but does not protect against malware running as your user. Prefer env or keyring.

See also: [ONBOARDING.md](./ONBOARDING.md), [ERRORS.md](./ERRORS.md).
