#!/usr/bin/env bash
#
# Orchestrate building the full Buildermark stack:
#   1. Svelte frontend  (local/frontend)
#   2. Go server binary  (local/server)  — embeds the frontend build
#   3. macOS app          (apps/macos)    — embeds the Go binary
#
# Usage:
#   ./scripts/build.sh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/local/frontend"
SERVER_DIR="$ROOT_DIR/local/server"
MACOS_DIR="$ROOT_DIR/apps/macos"

SERVER_BINARY="buildermark-server"

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

# Copy the full build output into the Go server's embed path so it gets compiled
# into the binary (//go:embed frontend in dashboard.go).
rm -rf "$SERVER_DIR/internal/handler/frontend"
cp -R "$FRONTEND_DIR/build" "$SERVER_DIR/internal/handler/frontend"

# ---------------------------------------------------------------------------
# 2. Build Go server
# ---------------------------------------------------------------------------

step "Building Go server"
cd "$SERVER_DIR"
CGO_ENABLED=1 go build -o "$SERVER_BINARY" ./cmd/buildermark

# ---------------------------------------------------------------------------
# 3. Build macOS app
# ---------------------------------------------------------------------------

step "Building macOS app"
"$MACOS_DIR/scripts/build.sh"

# Copy the server binary into the exported app bundle so ServerManager.swift
# can find it via Bundle.main.url(forResource:).
APP_PATH="$MACOS_DIR/build/export/Buildermark.app"
cp "$SERVER_DIR/$SERVER_BINARY" "$APP_PATH/Contents/Resources/$SERVER_BINARY"

step "Full build complete"
echo "  App: $APP_PATH"
