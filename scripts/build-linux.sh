#!/usr/bin/env bash
#
# Orchestrate building the full Buildermark stack for Linux:
#   1. Svelte frontend  (local/frontend)
#   2. Go CLI binary     (local/server/cmd/buildermark-cli) — embeds the frontend
#
# Usage:
#   ./scripts/build-linux.sh                        # build for host architecture
#   ./scripts/build-linux.sh --arch amd64           # build for x86_64
#   ./scripts/build-linux.sh --arch arm64           # build for aarch64
#   ./scripts/build-linux.sh --arch all             # build both
#   ./scripts/build-linux.sh --arch amd64 --version 1.0.0
#
# Environment variables:
#   ARCH    - "amd64", "arm64", or "all" (default: host architecture)
#   VERSION - version string baked into the binary (default: "dev")

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/local/frontend"
SERVER_DIR="$ROOT_DIR/local/server"
BUILD_DIR="$ROOT_DIR/apps/linux-cli/build"

# Defaults from env, overridable by flags.
ARCH="${ARCH:-}"
VERSION="${VERSION:-dev}"

# ---------------------------------------------------------------------------
# Parse flags
# ---------------------------------------------------------------------------

while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch)    ARCH="$2";    shift 2 ;;
        --version) VERSION="$2"; shift 2 ;;
        *)
            # Positional fallback: first arg is version (backwards compat).
            VERSION="$1"; shift ;;
    esac
done

# Default to host architecture if not specified.
if [[ -z "$ARCH" ]]; then
    ARCH="$(go env GOARCH)"
fi

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

step() {
    echo ""
    echo "==> $1"
    echo ""
}

check_tool() {
    if ! command -v "$1" &>/dev/null; then
        echo "Error: $1 is not installed."
        echo "  $2"
        exit 1
    fi
}

HOST_ARCH="$(go env GOARCH)"

# Returns the CC compiler for the target architecture.
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

# ---------------------------------------------------------------------------
# Resolve target architectures
# ---------------------------------------------------------------------------

if [[ "$ARCH" == "all" ]]; then
    ARCHES=(amd64 arm64)
else
    ARCHES=("$ARCH")
fi

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------

step "Checking prerequisites"
check_tool node "Install Node.js: https://nodejs.org/"
check_tool npm  "Install Node.js: https://nodejs.org/"
check_tool go   "Install Go: https://go.dev/dl/"

for arch in "${ARCHES[@]}"; do
    cc="$(cc_for_arch "$arch")"
    check_tool "$cc" "Install cross-compiler: sudo apt install gcc-$(echo "$cc" | sed 's/-gcc$//')"
done

# ---------------------------------------------------------------------------
# 1. Build Svelte frontend (once — shared across architectures)
# ---------------------------------------------------------------------------

step "Building Svelte frontend"
cd "$FRONTEND_DIR"
npm ci
npm run build

# Copy the full build output into the Go server's embed path so it gets compiled
# into the binary (//go:embed frontend in dashboard.go).
rm -rf "$SERVER_DIR/internal/handler/frontend"
cp -R "$FRONTEND_DIR/build" "$SERVER_DIR/internal/handler/frontend"

# ---------------------------------------------------------------------------
# 2. Build CLI binary for each architecture
# ---------------------------------------------------------------------------

for arch in "${ARCHES[@]}"; do
    cc="$(cc_for_arch "$arch")"
    output="$BUILD_DIR/$arch/buildermark"

    step "Building Linux CLI binary ($arch, CC=$cc, version: $VERSION)"

    mkdir -p "$BUILD_DIR/$arch"
    cd "$SERVER_DIR"
    GOOS=linux GOARCH="$arch" CGO_ENABLED=1 CC="$cc" go build \
        -tags cli \
        -ldflags "-X main.version=$VERSION" \
        -o "$output" \
        ./cmd/buildermark-cli

    echo "  OK: $output"
done

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

step "Full build complete"
for arch in "${ARCHES[@]}"; do
    echo "  $arch : $BUILD_DIR/$arch/buildermark"
done
