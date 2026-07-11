#!/bin/sh
set -eu

REPO="duffy/glean"
INSTALL_DIR="${GLEAN_INSTALL_DIR:-${HOME}/.local/bin}"

uname_s=$(uname -s)
uname_m=$(uname -m)

case "$uname_s" in
    Linux)  os="linux" ;;
    Darwin) os="darwin" ;;
    *) echo "Unsupported OS: $uname_s"; exit 1 ;;
esac

case "$uname_m" in
    x86_64)  arch="amd64" ;;
    aarch64) arch="arm64" ;;
    arm64)   arch="arm64" ;;
    *) echo "Unsupported arch: $uname_m"; exit 1 ;;
esac

if command -v glean >/dev/null 2>&1; then
    current=$(glean --version 2>/dev/null || echo "unknown")
    echo "glean $current already installed at $(command -v glean)"
    echo "Reinstall? [y/N] \c"
    read -r answer
    case "$answer" in
        y|Y) ;;
        *) echo "Aborted."; exit 0 ;;
    esac
fi

if [ -w /usr/local/bin ]; then
    INSTALL_DIR="/usr/local/bin"
fi

mkdir -p "$INSTALL_DIR"

latest=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$latest" ]; then
    echo "Could not determine latest release. Check https://github.com/${REPO}/releases"
    exit 1
fi

url="https://github.com/${REPO}/releases/download/${latest}/glean-${os}-${arch}"

echo "Downloading glean ${latest} (${os}/${arch})..."
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

curl -fsSL -o "$tmp" "$url"
chmod +x "$tmp"

dest="${INSTALL_DIR}/glean"
mv "$tmp" "$dest"
trap - EXIT

echo "Installed to ${dest}"

case ":$PATH:" in
    *":${INSTALL_DIR}:"*) ;;
    *) echo "WARNING: ${INSTALL_DIR} is not in your PATH. Add it:";;
esac

echo ""
echo "Run: glean --help"
echo "First run downloads the model (~400MB for fast, ~1.1GB for quality)."
