#!/bin/sh
set -eu

repo="duffy1001/glean"
variant="${GLEAN_VARIANT:-thin}"
model="${GLEAN_MODEL:-fast}"
force="${GLEAN_FORCE:-0}"

case "$variant" in
    thin|full) ;;
    *) echo "GLEAN_VARIANT must be thin or full" >&2; exit 1 ;;
esac
case "$model" in
    fast|high) ;;
    *) echo "GLEAN_MODEL must be fast or high" >&2; exit 1 ;;
esac

case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac
if [ "$os" = darwin ] && [ "$arch" = amd64 ]; then
    echo "Prebuilt macOS releases currently require Apple Silicon." >&2
    exit 1
fi

if [ -n "${GLEAN_INSTALL_DIR:-}" ]; then
    install_dir=$GLEAN_INSTALL_DIR
elif [ -w /usr/local/bin ]; then
    install_dir=/usr/local/bin
else
    install_dir="${HOME}/.local/bin"
fi
dest="${install_dir}/glean"

if [ -e "$dest" ] && [ "$force" != 1 ]; then
    current=$($dest --version 2>/dev/null || echo unknown)
    if [ -r /dev/tty ]; then
        printf '%s' "${current} is already installed at ${dest}. Replace it? [y/N] " >/dev/tty
        read -r answer </dev/tty
        case "$answer" in y|Y) ;; *) echo "Aborted."; exit 0 ;; esac
    else
        echo "${dest} already exists; set GLEAN_FORCE=1 to replace it" >&2
        exit 1
    fi
fi

latest_url=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${repo}/releases/latest")
version=${latest_url##*/}
case "$version" in
    v*) ;;
    *) echo "Could not determine latest release from ${latest_url}" >&2; exit 1 ;;
esac

asset="glean-${variant}-${model}-${os}-${arch}"
base="https://github.com/${repo}/releases/download/${version}"
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

echo "Downloading glean ${version} ${variant}-${model} (${os}/${arch})..." >&2
curl -fL --retry 3 -o "${tmp_dir}/${asset}" "${base}/${asset}"
curl -fL --retry 3 -o "${tmp_dir}/checksums.txt" "${base}/checksums.txt"

expected=$(awk -v name="$asset" '$2 == name { print $1 }' "${tmp_dir}/checksums.txt")
if [ -z "$expected" ]; then
    echo "No checksum published for ${asset}" >&2
    exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "${tmp_dir}/${asset}" | awk '{ print $1 }')
else
    actual=$(shasum -a 256 "${tmp_dir}/${asset}" | awk '{ print $1 }')
fi
if [ "$actual" != "$expected" ]; then
    echo "Checksum mismatch for ${asset}" >&2
    exit 1
fi

mkdir -p "$install_dir"
chmod 0755 "${tmp_dir}/${asset}"
mv "${tmp_dir}/${asset}" "$dest"
trap - EXIT
rm -rf "$tmp_dir"

echo "Installed $($dest --version) to ${dest}" >&2
case ":$PATH:" in
    *":${install_dir}:"*) ;;
    *) echo "Add ${install_dir} to PATH." >&2 ;;
esac
