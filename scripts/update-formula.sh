#!/usr/bin/env bash
# update-formula.sh — fill Formula/codeforge.rb sha256 from a release checksums.txt
# Usage:
#   scripts/update-formula.sh v1.8.2
#   scripts/update-formula.sh v1.8.2 ./dist/checksums.txt
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TAG="${1:-}"
CHECKSUMS="${2:-}"
if [[ -z "$TAG" ]]; then
  TAG="v$(tr -d '[:space:]' < VERSION)"
fi
VER="${TAG#v}"
FORMULA="Formula/codeforge.rb"

if [[ ! -f "$FORMULA" ]]; then
  echo "ERROR: $FORMULA missing"
  exit 1
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

if [[ -z "$CHECKSUMS" ]]; then
  url="https://github.com/NanoMindExplorer/codeforge/releases/download/${TAG}/checksums.txt"
  echo "Fetching $url"
  if ! curl -fsSL "$url" -o "$tmpdir/checksums.txt"; then
    echo "ERROR: could not download checksums (release may not exist yet)"
    exit 1
  fi
  CHECKSUMS="$tmpdir/checksums.txt"
fi

sha_for() {
  local arch="$1"
  # name_template: codeforge_VERSION_OS_ARCH.tar.gz
  local line
  line="$(grep -E "codeforge_${VER}_${arch}\\.tar\\.gz\$" "$CHECKSUMS" | head -1 || true)"
  if [[ -z "$line" ]]; then
    line="$(grep -E "codeforge_.*_${arch}\\.tar\\.gz\$" "$CHECKSUMS" | head -1 || true)"
  fi
  if [[ -z "$line" ]]; then
    echo ""
    return
  fi
  # sha256 is first field
  echo "$line" | awk '{print $1}'
}

declare -A MAP=(
  [darwin_arm64]="$(sha_for darwin_arm64)"
  [darwin_amd64]="$(sha_for darwin_amd64)"
  [linux_arm64]="$(sha_for linux_arm64)"
  [linux_amd64]="$(sha_for linux_amd64)"
)

# Rewrite formula with version + sha256 lines
python3 - "$FORMULA" "$VER" \
  "${MAP[darwin_arm64]:-}" "${MAP[darwin_amd64]:-}" \
  "${MAP[linux_arm64]:-}" "${MAP[linux_amd64]:-}" <<'PY'
import sys
path, ver, darm, damd, larm, lamd = sys.argv[1:7]
text = open(path).read()
import re
text = re.sub(r'version "[^"]+"', f'version "{ver}"', text)

def inject_sha(text, arch_token, sha):
    # After url line containing arch_token, set sha256
    lines = text.splitlines(True)
    out = []
    i = 0
    while i < len(lines):
        out.append(lines[i])
        if arch_token in lines[i] and "url " in lines[i]:
            # skip existing sha256 or comment lines until next non-sha
            j = i + 1
            while j < len(lines) and ("sha256" in lines[j] or lines[j].strip().startswith("# sha256") or lines[j].strip() == ""):
                if "sha256" in lines[j] or lines[j].strip().startswith("# sha256"):
                    j += 1
                    continue
                break
            if sha:
                indent = "      "
                out.append(f'{indent}sha256 "{sha}"\n')
            else:
                out.append('      # sha256 "REPLACE_ON_RELEASE"\n')
            i = j
            continue
        i += 1
    return "".join(out)

for token, sha in [
    ("darwin_arm64", darm),
    ("darwin_amd64", damd),
    ("linux_arm64", larm),
    ("linux_amd64", lamd),
]:
    text = inject_sha(text, token, sha)

open(path, "w").write(text)
print(f"Updated {path} for version {ver}")
for k, v in [("darwin_arm64", darm), ("darwin_amd64", damd), ("linux_arm64", larm), ("linux_amd64", lamd)]:
    print(f"  {k}: {v or '(missing)'}")
PY

echo "Done. Review $FORMULA and commit."
