#!/bin/bash
set -euo pipefail

# Cross-compilation release script for jsonify
# Builds static binaries for multiple platforms.
#
# Usage:
#   ./release.sh                    # build for current platform
#   ./release.sh linux/amd64        # build for specific target
#   ./release.sh all                # build all targets
#
# Requirements for cross-compilation:
#   - Docker (for linux/arm64 cross-compile)
#   - Native build for darwin and windows requires running on that platform
#
# Output: dist/jsonify-<os>-<arch>[.exe]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="${SCRIPT_DIR}/dist"
mkdir -p "${DIST_DIR}"

TARGET="${1:-current}"

build_native() {
    local os="$1"
    local arch="$2"
    local suffix=""
    [ "$os" = "windows" ] && suffix=".exe"
    local out="${DIST_DIR}/jsonify-${os}-${arch}${suffix}"

    echo "=== Building jsonify-${os}-${arch} (native) ==="

    cd "${SCRIPT_DIR}"
    make clean-native
    make static

    if [ "$os" = "windows" ] && [ "$suffix" = ".exe" ]; then
        mv jsonify-static "$out"
    else
        cp jsonify-static "$out" 2>/dev/null || cp jsonify "$out"
    fi

    echo "Built: $out ($(ls -lh "$out" | awk '{print $5}'))"
}

build_cross_linux() {
    local arch="$1"
    local cc="$2"
    local cxx="$3"
    local cmake_toolchain="$4"
    local suffix=""
    local out="${DIST_DIR}/jsonify-linux-${arch}"

    echo "=== Building jsonify-linux-${arch} (cross-compile via Docker) ==="

    local docker_arch
    case "$arch" in
        amd64)  docker_arch="linux/amd64" ;;
        arm64)  docker_arch="linux/arm64" ;;
        *)      echo "Unsupported arch: $arch"; return 1 ;;
    esac

    docker run --rm --platform "$docker_arch" \
        -v "${SCRIPT_DIR}:/work" -w /work \
        ubuntu:22.04 bash -c '
            apt-get update -qq && apt-get install -y -qq \
                build-essential cmake git g++ > /dev/null 2>&1
            make clean
            make static
            mv jsonify-static dist/jsonify-linux-'"${arch}"'
        '

    echo "Built: $out ($(ls -lh "$out" 2>/dev/null | awk '{print $5}'))"
}

case "$TARGET" in
    current)
        os=$(uname -s | tr '[:upper:]' '[:lower:]')
        arch=$(uname -m)
        case "$arch" in
            x86_64)  arch="amd64" ;;
            aarch64) arch="arm64" ;;
        esac
        build_native "$os" "$arch"
        ;;
    linux/amd64)
        build_cross_linux "amd64" "x86_64-linux-gnu-gcc" "x86_64-linux-gnu-g++" ""
        ;;
    linux/arm64)
        build_cross_linux "arm64" "aarch64-linux-gnu-gcc" "aarch64-linux-gnu-g++" ""
        ;;
    all)
        build_cross_linux "amd64" "" "" ""
        build_cross_linux "arm64" "" "" ""
        echo ""
        echo "NOTE: darwin and windows binaries must be built natively on those platforms."
        echo "      Run 'make static' on macOS or Windows, then copy to dist/."
        ;;
    *)
        if [[ "$TARGET" =~ ^(.+)/(.+)$ ]]; then
            echo "Cross-compilation for ${TARGET} requires platform-specific setup."
            echo "Use 'docker run --platform linux/${TARGET}' or build natively."
            exit 1
        else
            echo "Unknown target: $TARGET"
            echo "Usage: $0 [current|linux/amd64|linux/arm64|all]"
            exit 1
        fi
        ;;
esac

echo ""
echo "Done. Binaries in ${DIST_DIR}/"
ls -lh "${DIST_DIR}"/ 2>/dev/null || echo "(empty)"
