#!/usr/bin/env bash
#
# Orchestrate building the full Buildermark stack:
#   1. Svelte frontend  (local/frontend)
#   2. Go server binary  (local/server)  — embeds the frontend build
#   3. macOS app          (apps/macos)    — embeds the Go binary
#
# Usage:
#   ./scripts/build-macos.sh
#   ./scripts/build-macos.sh --arch arm64
#   ./scripts/build-macos.sh --arch amd64

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/local/frontend"
SERVER_DIR="$ROOT_DIR/local/server"
MACOS_DIR="$ROOT_DIR/apps/macos"

SERVER_BINARY="buildermark-server"

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------

ARCH=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch) ARCH="$2"; shift 2 ;;
        *) echo "Unknown argument: $1" >&2; exit 1 ;;
    esac
done

# Map arch for Go cross-compilation.
GOARCH_VAL=""
if [[ -n "$ARCH" ]]; then
    case "$ARCH" in
        arm64) GOARCH_VAL="arm64" ;;
        amd64) GOARCH_VAL="amd64" ;;
        *)
            echo "Error: unsupported architecture '$ARCH'. Use arm64 or amd64." >&2
            exit 1
            ;;
    esac
fi

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

ARCH_LABEL="${ARCH:-native}"
step "Building Go server ($ARCH_LABEL)"
cd "$SERVER_DIR"

GO_BUILD_ENV=(CGO_ENABLED=1)
if [[ -n "$GOARCH_VAL" ]]; then
    GO_BUILD_ENV+=(GOARCH="$GOARCH_VAL")
fi

env "${GO_BUILD_ENV[@]}" go build -o "$SERVER_BINARY" ./cmd/buildermark

# ---------------------------------------------------------------------------
# 3. Build macOS app
# ---------------------------------------------------------------------------

step "Building macOS app ($ARCH_LABEL)"

BUILD_ARGS=()
if [[ -n "$ARCH" ]]; then
    BUILD_ARGS+=(--arch "$ARCH")
fi

"$MACOS_DIR/scripts/build.sh" "${BUILD_ARGS[@]}"

# Copy the server binary into the exported app bundle so ServerManager.swift
# can find it via Bundle.main.url(forResource:).
if [[ -n "$ARCH" ]]; then
    APP_PATH="$MACOS_DIR/build/export-$ARCH/Buildermark.app"
else
    APP_PATH="$MACOS_DIR/build/export/Buildermark.app"
fi
cp "$SERVER_DIR/$SERVER_BINARY" "$APP_PATH/Contents/Resources/$SERVER_BINARY"

step "Full build complete ($ARCH_LABEL)"
echo "  App: $APP_PATH"
