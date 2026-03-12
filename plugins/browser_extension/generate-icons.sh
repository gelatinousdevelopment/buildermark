#!/usr/bin/env bash
#
# Generate extension icons from Buildermark SVG.
# - Default fill color: #666666 (icon*.png)
# - Alternate fill color: #0066cc (blue_icon*.png)
# - Margin controlled by INNER_SCALE_PERCENT (currently 100)
#
# Usage:
#   ./plugins/browser_extension/generate-icons.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
ICON_SVG_SRC="$ROOT_DIR/local/frontend/src/lib/icons/buildermarkIcon.svg"
DEFAULT_ICON_COLOR="#666666"
BLUE_ICON_COLOR="#0066cc"
INNER_SCALE_PERCENT=100

if ! command -v magick >/dev/null 2>&1; then
    echo "Error: ImageMagick 'magick' command not found."
    exit 1
fi

if [[ ! -f "$ICON_SVG_SRC" ]]; then
    echo "Error: Icon SVG source not found at $ICON_SVG_SRC"
    exit 1
fi

TMP_ICON_SVG="$(mktemp "${TMPDIR:-/tmp}/buildermark-icon-XXXXXX.svg")"
trap 'rm -f "$TMP_ICON_SVG"' EXIT

generate_set() {
    local color="$1"
    local filename_prefix="$2"

    # The source icon uses currentColor, so set a concrete fill color for rasterization.
    sed "s/currentcolor/$color/g; s/currentColor/$color/g" "$ICON_SVG_SRC" > "$TMP_ICON_SVG"

    out_dir="$SCRIPT_DIR/src/icons"
    mkdir -p "$out_dir"

    for size in 16 32 48 128; do
        inner_size=$(( (size * INNER_SCALE_PERCENT + 50) / 100 ))
        magick \
            -background none \
            "$TMP_ICON_SVG" \
            -resize "${inner_size}x${inner_size}" \
            -gravity center \
            -extent "${size}x${size}" \
            "$out_dir/${filename_prefix}icon${size}.png"
    done

    echo "Generated ${filename_prefix}icons"
}

generate_set "$DEFAULT_ICON_COLOR" ""
generate_set "$BLUE_ICON_COLOR" "blue_"

echo "Done."
