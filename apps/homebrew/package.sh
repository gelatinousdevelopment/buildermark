#!/usr/bin/env bash
#
# package.sh — Build and package Buildermark for Homebrew distribution.
#
# This script can be used in two modes:
#
#   1. Local build (no arguments):
#      Builds the frontend, compiles the Go binary, and outputs it to ./dist/
#
#   2. Release mode (with version argument):
#      Runs GoReleaser to create tagged release archives.
#
# Usage:
#   # Local build
#   bash apps/homebrew/package.sh
#
#   # GoReleaser release (requires goreleaser installed)
#   bash apps/homebrew/package.sh v0.1.0
#
# Prerequisites:
#   - Go 1.25+
#   - Node.js 20+ and pnpm (or npm)
#   - goreleaser (for release mode only)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VERSION="${1:-}"

cd "$REPO_ROOT"

# Step 1: Build the frontend
echo "==> Step 1: Building frontend..."
bash apps/homebrew/build-frontend.sh

if [[ -n "$VERSION" ]]; then
    # Release mode: use GoReleaser
    echo "==> Step 2: Running GoReleaser for $VERSION..."

    if ! command -v goreleaser &>/dev/null; then
        echo "Error: goreleaser is not installed." >&2
        echo "Install it with: brew install goreleaser" >&2
        exit 1
    fi

    # GoReleaser expects a git tag
    if ! git tag -l "$VERSION" | grep -q "$VERSION"; then
        echo "Warning: Tag $VERSION does not exist. Creating it now..."
        git tag -a "$VERSION" -m "Release $VERSION"
    fi

    goreleaser release \
        --config apps/homebrew/.goreleaser.yml \
        --clean

    echo "==> Release archives are in dist/"
else
    # Local build mode
    echo "==> Step 2: Building Go binary..."

    cd "$REPO_ROOT/local/server"

    COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
    DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

    CGO_ENABLED=1 go build \
        -tags embed_frontend \
        -ldflags "-s -w -X main.version=dev -X main.commit=$COMMIT -X main.date=$DATE" \
        -o "$REPO_ROOT/dist/buildermark" \
        ./cmd/buildermark

    echo "==> Binary built: $REPO_ROOT/dist/buildermark"
    file "$REPO_ROOT/dist/buildermark"
fi
