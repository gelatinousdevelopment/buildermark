#!/usr/bin/env bash
#
# Build the shared browser extension outputs for all supported browsers.
#
# Usage:
#   ./scripts/build-browser-extensions.sh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
EXT_DIR="$ROOT_DIR/plugins/browser_extension"

"$EXT_DIR/build.sh" all
