# Sandbox (Phase G4)

Grok-compatible **OS shell sandbox** for CodeForge. Restricts what agent tools and shell commands can write/read, using:

| Backend | When | Enforcement |
|---------|------|-------------|
| **bwrap** | `bubblewrap` on PATH (Linux) | Kernel mount namespace for `run_command` / bg tasks |
| **soft** | No bwrap | Path policy on `read_file` / `write_file` / `search_replace` + optional `unshare -n` for network |
| **off** | Default | Unrestricted (project path sandbox only) |

In-process tools (`web_search`, LLM API) always keep network access — only **child shell** network is blocked when the profile says so.

## Profiles (match Grok)

| Profile | FS Read | FS Write | Child network | Use |
|---------|---------|----------|---------------|-----|
| `off` | unrestricted | unrestricted | allow | default |
| `workspace` | everywhere | CWD + `~/.codeforge` + `/tmp` | allow | daily coding |
| `read-only` | everywhere | `~/.codeforge` + tmp only | **block** | review / explore |
| `strict` | CWD + system paths | CWD + `~/.codeforge` + tmp | **block** | untrusted code |
| `devbox` | everywhere | almost all except `/data` | allow | disposable VMs |

## Quick start

```bash
# Recommended for normal work
codeforge --sandbox workspace

# Review-only
codeforge --sandbox read-only

# Headless CI with strict shell
codeforge agent --sandbox strict --always-approve "summarize README"

# Env (same as Grok GROK_SANDBOX)
export CODEFORGE_SANDBOX=workspace
# or
export GROK_SANDBOX=workspace
```

Inside TUI:

```text
/sandbox              # status + help
/sandbox workspace
/sandbox off
```

Footer badge when active: `SBX:ws` · `SBX:ro` · `SBX:strict`.

## Config

`~/.config/codeforge/config.yaml`:

```yaml
sandbox:
  profile: workspace
  deny:
    - "**/.env"
    - "**/*.pem"
```

`deny` is always applied on the soft path layer. With bubblewrap, exact (non-glob) deny paths are bind-overlaid (`/dev/null` or tmpfs).

## Resolution order

1. Explicit `--sandbox <profile>`
2. `CODEFORGE_SANDBOX` or `GROK_SANDBOX`
3. `sandbox.profile` in config
4. `off`

## Events

`~/.codeforge/sandbox-events.jsonl` logs activate/switch events for debugging.

## Process-wide kernel isolation

When a profile is active, CodeForge also tries **process-wide** isolation at startup:

| Platform | Mechanism | Notes |
|----------|-----------|--------|
| **Linux** | **Landlock** LSM | Write-only rules for `workspace`/`read-only`; read+write for `strict`. Requires kernel 5.13+ (and Landlock enabled). Best-effort: falls back silently if unavailable (e.g. some containers/PRoot). |
| **macOS** | **Seatbelt** via `sandbox-exec` | Writes a profile under `~/.codeforge/sandbox/` and re-execs once (`CODEFORGE_SEATBELT_APPLIED=1`). Set `CODEFORGE_SEATBELT=0` to disable re-exec. |

Status line example:

```text
sandbox: workspace (shell=bwrap, process=landlock, net=allow)
```

## Honest limits

- Soft backend does **not** stop a malicious `bash -c 'cat /etc/shadow'` under `workspace` (reads are open by design, like Grok workspace).
- Soft backend **does** block CodeForge file tools from writing outside allowed roots (and project writes in `read-only`).
- Landlock may be unavailable in restricted environments (PRoot, old kernels) — shell bwrap + soft path still apply.
- Network block on soft uses `unshare -n` and may fail without user namespaces.

Install bubblewrap for stronger **child shell** FS isolation on Linux:

```bash
# Debian/Ubuntu
sudo apt install bubblewrap
```

## Related

- Grok user guide: sandbox mode (`18-sandbox.md`)
- Permissions (Phase 6): allow/deny/ask rules still apply **before** tools run
- Session modes BUILD/DESIGN/YOLO are independent of sandbox profiles
