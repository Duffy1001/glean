#!/bin/sh
set -eu

model=${1:-fast}
asset_dir="$(CDPATH= cd -- "$(dirname -- "$0")/../assets" && pwd)"

prepare() {
    name=$1
    url=$2
    expected=$3
    output="${asset_dir}/${name}.zst"
    if [ -f "$output" ]; then
        return
    fi
    tmp=$(mktemp "${asset_dir}/${name}.XXXXXX")
    trap 'rm -f "$tmp"' EXIT HUP INT TERM
    curl -fL --retry 3 -o "$tmp" "$url"
    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$tmp" | cut -d ' ' -f 1)
    else
        actual=$(shasum -a 256 "$tmp" | cut -d ' ' -f 1)
    fi
    if [ "$actual" != "$expected" ]; then
        echo "Model checksum mismatch: got $actual, want $expected" >&2
        exit 1
    fi
    zstd -T0 -19 -f "$tmp" -o "$output"
    rm -f "$tmp"
    trap - EXIT HUP INT TERM
}

case "$model" in
    fast) prepare "qwen3-0.6b-q4_k_m.gguf" "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q4_K_M.gguf" "ac2d97712095a558e31573f62f466a3f9d93990898b0ec79d7c974c1780d524a" ;;
    *) echo "Usage: $0 [fast]" >&2; exit 1 ;;
esac
