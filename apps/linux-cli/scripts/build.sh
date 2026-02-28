#!/usr/bin/env bash
#
# Build the Buildermark Linux CLI binary.
#
#   1. Svelte frontend  (local/frontend)
#   2. CLI binary        (local/server/cmd/buildermark-cli) — embeds the frontend
#
# Usage:
#   ./apps/linux-cli/scripts/build.sh [VERSION]
#
# The VERSION argument is optional; defaults to "dev".

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/local/frontend"
SERVER_DIR="$ROOT_DIR/local/server"
VERSION="${1:-dev}"
OUTPUT="$ROOT_DIR/apps/linux-cli/buildermark"

step() {
    echo ""
    echo "==> $1"
    echo ""
}

# ---------------------------------------------------------------------------
# 1. Build Svelte frontend
# ---------------------------------------------------------------------------

step "Building Svelte frontend"
cd "$FRONTEND_DIR"
npm ci
npm run build

# Copy the full build output into the Go server's embed path.
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

step "Build complete"
echo "  Binary: $OUTPUT"
echo "  Version: $VERSION"
