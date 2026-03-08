#!/usr/bin/env bash
#
# Build all browser extensions:
#   1. Copy shared code into each browser extension directory
#
# Usage:
#   ./scripts/build-browser-extensions.sh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
EXT_DIR="$ROOT_DIR/apps/browser_extensions"

step() {
    echo ""
    echo "==> $1"
    echo ""
}

# ---------------------------------------------------------------------------
# 1. Copy shared code into each extension
# ---------------------------------------------------------------------------

step "Copying shared code"
"$EXT_DIR/build.sh"

step "Browser extensions build complete"
