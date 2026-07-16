#!/usr/bin/env sh
# CodeForge one-line installer
# curl -fsSL https://raw.githubusercontent.com/NanoMindExplorer/codeforge/main/install.sh | sh
set -e

REPO="NanoMindExplorer/codeforge"
BINARY="codeforge"
VERSION="${CODEFORGE_VERSION:-latest}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "unsupported arch: $arch"; exit 1 ;;
esac

case "$os" in
  linux|darwin) ;;
  mingw*|msys*|cygwin*) os="windows" ;;
  *) echo "unsupported os: $os"; exit 1 ;;
esac

if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
  if [ -z "$VERSION" ]; then
    echo "No GitHub release found — building from source..."
    if ! command -v go >/dev/null 2>&1; then
      echo "Go is required to build from source. Install Go or wait for a release."
      exit 1
    fi
    TMP=$(mktemp -d)
    git clone --depth 1 "https://github.com/${REPO}.git" "$TMP/codeforge"
    cd "$TMP/codeforge"
    CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BINARY" ./cmd/codeforge/
    DEST="${PREFIX:-/usr/local}/bin"
    if [ -n "$PREFIX" ] && [ -d "$PREFIX/bin" ]; then
      DEST="$PREFIX/bin"
    elif [ -n "$TERMUX_VERSION" ] || [ -d "$PREFIX" ]; then
      DEST="${PREFIX}/bin"
    fi
    mkdir -p "$DEST" 2>/dev/null || true
    if cp "$BINARY" "$DEST/$BINARY" 2>/dev/null; then
      echo "Installed to $DEST/$BINARY"
    else
      cp "$BINARY" "$HOME/$BINARY"
      echo "Installed to $HOME/$BINARY (add to PATH)"
    fi
    exit 0
  fi
fi

EXT="tar.gz"
[ "$os" = "windows" ] && EXT="zip"
ASSET="${BINARY}_${VERSION#v}_${os}_${arch}.${EXT}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"

TMP=$(mktemp -d)
cd "$TMP"
echo "Downloading $URL ..."
if ! curl -fsSL -o asset "$URL"; then
  echo "Download failed. Building from source..."
  exec sh -c "CODEFORGE_VERSION=source $(curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh)"
fi

if [ "$EXT" = "zip" ]; then
  unzip -q asset
else
  tar -xzf asset
fi

DEST="/usr/local/bin"
if [ -n "$PREFIX" ]; then
  DEST="$PREFIX/bin"
fi
mkdir -p "$DEST" 2>/dev/null || true
if install -m 755 "$BINARY" "$DEST/$BINARY" 2>/dev/null; then
  echo "✓ Installed $DEST/$BINARY ($VERSION)"
else
  install -m 755 "$BINARY" "$HOME/$BINARY"
  echo "✓ Installed $HOME/$BINARY — add to PATH"
fi
echo "Run: codeforge"
echo "Get free Gemini key: https://aistudio.google.com/apikey"
