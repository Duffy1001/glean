#!/bin/sh
set -eu

variant=${1:-all}
case "$variant" in
    thin|full|all) ;;
    *) echo "Usage: $0 [thin|full|all]" >&2; exit 1 ;;
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

build_thin() {
    if [ "$os" = linux ]; then
        make VERSION="$version" static
        cp glean-static "dist/glean-thin-${os}-${arch}"
    else
        make VERSION="$version" build-go
        cp glean "dist/glean-thin-${os}-${arch}"
    fi
}

build_full() {
    if [ "$os" = linux ]; then
        make VERSION="$version" static-full
        cp glean-full-static "dist/glean-full-${os}-${arch}"
    else
        make VERSION="$version" build-full
        cp glean-full "dist/glean-full-${os}-${arch}"
    fi
}

case "$variant" in
    thin) build_thin ;;
    full) build_full ;;
    all) build_thin; build_full ;;
esac

if command -v sha256sum >/dev/null 2>&1; then
    (cd dist && sha256sum glean-* > checksums.txt)
else
    (cd dist && shasum -a 256 glean-* > checksums.txt)
fi

ls -lh dist/glean-* dist/checksums.txt
