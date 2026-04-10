#!/usr/bin/env bash
#
# Build the shared browser extension outputs for all supported browsers.
#
# Usage:
#   ./scripts/build-browser-extensions.sh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
EXT_DIR="$ROOT_DIR/plugins/browser_extension"
DIST_DIR="$EXT_DIR/dist"
SAFARI_APP_BUNDLE_PATH="$EXT_DIR/safari/.derived-data/Build/Products/Debug/BuildermarkSafari.app"

"$EXT_DIR/build.sh" all

package_zip() {
    local target="$1"
    local source_dir="$DIST_DIR/$target"
    local zip_path="$DIST_DIR/$target.zip"

    if [[ ! -d "$source_dir" ]]; then
        printf 'Expected build output at %s\n' "$source_dir" >&2
        exit 1
    fi

    rm -f "$zip_path"
    (cd "$source_dir" && zip -rq "$zip_path" .)
    printf 'Created %s\n' "$zip_path"
}

package_zip chromium
package_zip firefox

if [[ ! -d "$SAFARI_APP_BUNDLE_PATH" ]]; then
    printf 'Expected Safari app bundle at %s\n' "$SAFARI_APP_BUNDLE_PATH" >&2
    exit 1
fi

SAFARI_DEST="$DIST_DIR/$(basename "$SAFARI_APP_BUNDLE_PATH")"
rm -rf "$SAFARI_DEST"
cp -R "$SAFARI_APP_BUNDLE_PATH" "$SAFARI_DEST"
printf 'Copied %s\n' "$SAFARI_DEST"
