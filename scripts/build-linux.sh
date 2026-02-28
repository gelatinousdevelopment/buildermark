#!/usr/bin/env bash
#
# Orchestrate building the full Buildermark stack for Linux:
#   1. Svelte frontend  (local/frontend)
#   2. Go CLI binary     (local/server/cmd/buildermark-cli) — embeds the frontend
#
# Usage:
#   ./scripts/build-linux.sh [VERSION]
#
# The VERSION argument is optional; defaults to "dev".

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/local/frontend"
SERVER_DIR="$ROOT_DIR/local/server"
VERSION="${1:-dev}"
OUTPUT="$ROOT_DIR/apps/linux-cli/buildermark"

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

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------

step "Checking prerequisites"
check_tool node "Install Node.js: https://nodejs.org/"
check_tool npm  "Install Node.js: https://nodejs.org/"
check_tool go   "Install Go: https://go.dev/dl/"
check_tool gcc  "Install GCC: sudo apt install build-essential"

# ---------------------------------------------------------------------------
# 1. Build Svelte frontend
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
# 2. Build CLI binary
# ---------------------------------------------------------------------------

step "Building Linux CLI binary (version: $VERSION)"
cd "$SERVER_DIR"
CGO_ENABLED=1 go build \
    -tags cli \
    -ldflags "-X main.version=$VERSION" \
    -o "$OUTPUT" \
    ./cmd/buildermark-cli

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

step "Full build complete"
echo "  Binary:  $OUTPUT"
echo "  Version: $VERSION"
echo ""
echo "  Install: cp $OUTPUT ~/.local/bin/buildermark"
echo "  Service: buildermark service install"
