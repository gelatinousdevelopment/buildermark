#!/usr/bin/env bash
#
# build-frontend.sh — Build the Svelte frontend SPA and stage it for Go embedding.
#
# Called by GoReleaser as a before-hook, or manually before `go build`.
# The built assets are placed into local/server/internal/handler/frontend_dist/
# so they can be picked up by //go:embed directives in the Go server.
#
# Usage:
#   bash apps/homebrew/build-frontend.sh
#
# Environment:
#   SKIP_FRONTEND_BUILD=1   Skip the frontend build (use existing dist)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
FRONTEND_DIR="$REPO_ROOT/local/frontend"
DIST_DIR="$REPO_ROOT/local/server/internal/handler/frontend_dist"

if [[ "${SKIP_FRONTEND_BUILD:-}" == "1" ]]; then
    echo "SKIP_FRONTEND_BUILD=1, skipping frontend build"
    if [[ ! -d "$DIST_DIR" ]]; then
        echo "Error: $DIST_DIR does not exist and SKIP_FRONTEND_BUILD=1" >&2
        exit 1
    fi
    exit 0
fi

echo "==> Building Svelte frontend SPA..."

cd "$FRONTEND_DIR"

# Install dependencies (prefer pnpm if available, fall back to npm)
if command -v pnpm &>/dev/null; then
    pnpm install --frozen-lockfile
else
    npm ci
fi

# Build the static SPA
npm run build

echo "==> Copying build output to Go embed directory..."

# Clean and recreate the embed target directory
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

# SvelteKit adapter-static outputs to build/ by default
cp -r "$FRONTEND_DIR/build/"* "$DIST_DIR/"

echo "==> Frontend build complete: $DIST_DIR"
ls -la "$DIST_DIR"
