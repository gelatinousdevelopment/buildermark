#!/usr/bin/env bash
#
# Build the Buildermark Linux CLI binary.
#
#   1. Svelte frontend  (local/frontend)
#   2. CLI binary        (local/server/cmd/buildermark-cli) — embeds the frontend
#
# Usage:
#   ./apps/linux-cli/scripts/build.sh                        # host architecture
#   ./apps/linux-cli/scripts/build.sh --arch amd64
#   ./apps/linux-cli/scripts/build.sh --arch arm64
#   ./apps/linux-cli/scripts/build.sh --arch all
#   ./apps/linux-cli/scripts/build.sh --arch all --version 1.0.0
#
# Environment variables:
#   ARCH    - "amd64", "arm64", or "all" (default: host architecture)
#   VERSION - version string baked into the binary (default: "dev")

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/local/frontend"
SERVER_DIR="$ROOT_DIR/local/server"
BUILD_DIR="$ROOT_DIR/apps/linux-cli/build"

ARCH="${ARCH:-}"
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch)    ARCH="$2";    shift 2 ;;
        --version) VERSION="$2"; shift 2 ;;
        *)         VERSION="$1"; shift ;;
    esac
done

if [[ -z "$ARCH" ]]; then
    ARCH="$(go env GOARCH)"
fi

step() {
    echo ""
    echo "==> $1"
    echo ""
}

HOST_ARCH="$(go env GOARCH)"

cc_for_arch() {
    local target="$1"
    if [[ "$target" == "$HOST_ARCH" ]]; then
        echo "gcc"
        return
    fi
    case "$target" in
        amd64) echo "x86_64-linux-gnu-gcc" ;;
        arm64) echo "aarch64-linux-gnu-gcc" ;;
        *)     echo "gcc" ;;
    esac
}

if [[ "$ARCH" == "all" ]]; then
    ARCHES=(amd64 arm64)
else
    ARCHES=("$ARCH")
fi

# ---------------------------------------------------------------------------
# 1. Build Svelte frontend
# ---------------------------------------------------------------------------

step "Building Svelte frontend"
cd "$FRONTEND_DIR"
npm ci
npm run build

rm -rf "$SERVER_DIR/internal/handler/frontend"
cp -R "$FRONTEND_DIR/build" "$SERVER_DIR/internal/handler/frontend"

# ---------------------------------------------------------------------------
# 2. Build CLI binary for each architecture
# ---------------------------------------------------------------------------

for arch in "${ARCHES[@]}"; do
    cc="$(cc_for_arch "$arch")"
    output="$BUILD_DIR/$arch/buildermark"

    step "Building Linux CLI binary ($arch, version: $VERSION)"

    mkdir -p "$BUILD_DIR/$arch"
    cd "$SERVER_DIR"
    GOOS=linux GOARCH="$arch" CGO_ENABLED=1 CC="$cc" go build \
        -tags cli \
        -ldflags "-X main.version=$VERSION" \
        -o "$output" \
        ./cmd/buildermark-cli

    echo "  OK: $output"
done

step "Build complete"
for arch in "${ARCHES[@]}"; do
    echo "  $arch : $BUILD_DIR/$arch/buildermark"
done
