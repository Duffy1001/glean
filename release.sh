#!/bin/sh
set -eu

variant=${1:-all}
case "$variant" in
    thin-fast|thin-high|full-fast|full-high|all) ;;
    *) echo "Usage: $0 [thin-fast|thin-high|full-fast|full-high|all]" >&2; exit 1 ;;
esac

case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) echo "Use the GitHub Actions release workflow for Windows builds." >&2; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

version=${VERSION:-$(git describe --tags --always --dirty)}
mkdir -p dist

build_variant() {
    package=$1
    model=$2
    if [ "$os" = linux ]; then
        make VERSION="$version" "static-${package}-${model}"
        cp "glean-${package}-${model}-static" "dist/glean-${package}-${model}-${os}-${arch}"
    else
        make VERSION="$version" "build-${package}-${model}"
        cp "glean-${package}-${model}" "dist/glean-${package}-${model}-${os}-${arch}"
    fi
}

case "$variant" in
    thin-fast) build_variant thin fast ;;
    thin-high) build_variant thin high ;;
    full-fast) build_variant full fast ;;
    full-high) build_variant full high ;;
    all) build_variant thin fast; build_variant thin high; build_variant full fast; build_variant full high ;;
esac

if command -v sha256sum >/dev/null 2>&1; then
    (cd dist && sha256sum glean-* > checksums.txt)
else
    (cd dist && shasum -a 256 glean-* > checksums.txt)
fi

ls -lh dist/glean-* dist/checksums.txt
