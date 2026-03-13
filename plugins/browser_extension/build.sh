#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
SOURCE_DIR="$ROOT_DIR/src"
MANIFEST_DIR="$ROOT_DIR/manifests"
DIST_DIR="$ROOT_DIR/dist"
APP_ICON_SOURCE="$ROOT_DIR/safari/buildermark-app-icon-256.png"
SAFARI_PROJECT_ROOT="$ROOT_DIR/safari"
SAFARI_XCODE_PROJECT_DIR="$SAFARI_PROJECT_ROOT/BuildermarkSafari"
SAFARI_XCODE_PROJECT="$SAFARI_XCODE_PROJECT_DIR/BuildermarkSafari.xcodeproj"
SAFARI_XCODE_SCHEME="BuildermarkSafari"
SAFARI_XCODE_CONFIGURATION="Debug"
SAFARI_DERIVED_DATA_PATH="$SAFARI_PROJECT_ROOT/.derived-data"
SAFARI_APP_BUNDLE_NAME="BuildermarkSafari.app"
SAFARI_APP_BUNDLE_PATH="$SAFARI_DERIVED_DATA_PATH/Build/Products/$SAFARI_XCODE_CONFIGURATION/$SAFARI_APP_BUNDLE_NAME"

TARGET="${1:-all}"

usage() {
    cat <<'EOF'
Usage:
  ./build.sh [chromium|firefox|safari|all]
EOF
}

require_magick() {
    if command -v magick >/dev/null 2>&1; then
        return
    fi

    printf "ImageMagick 'magick' command not found\n" >&2
    exit 1
}

generate_app_icons() {
    local output_dir="$1"
    local icon_output_dir="$output_dir/icons"

    if [[ ! -f "$APP_ICON_SOURCE" ]]; then
        printf 'Expected app icon source at %s\n' "$APP_ICON_SOURCE" >&2
        exit 1
    fi

    require_magick

    for size in 16 32 48 128; do
        magick "$APP_ICON_SOURCE" \
            -resize "${size}x${size}" \
            "$icon_output_dir/app_icon${size}.png"
    done
}

build_target() {
    local target="$1"
    local output_dir="$DIST_DIR/$target"

    rm -rf "$output_dir"
    mkdir -p "$output_dir"
    rsync -a --delete --exclude '.DS_Store' "$SOURCE_DIR/" "$output_dir/"
    generate_app_icons "$output_dir"

    node - "$MANIFEST_DIR/base.json" "$MANIFEST_DIR/$target.json" "$output_dir/manifest.json" <<'EOF'
const fs = require("fs");

const [basePath, overlayPath, outputPath] = process.argv.slice(2);
const base = JSON.parse(fs.readFileSync(basePath, "utf8"));
const overlay = JSON.parse(fs.readFileSync(overlayPath, "utf8"));

function merge(baseValue, overlayValue) {
  if (Array.isArray(baseValue) || Array.isArray(overlayValue)) {
    return overlayValue === undefined ? baseValue : overlayValue;
  }

  if (isObject(baseValue) && isObject(overlayValue)) {
    const result = { ...baseValue };
    for (const [key, value] of Object.entries(overlayValue)) {
      result[key] = merge(baseValue[key], value);
    }
    return result;
  }

  return overlayValue === undefined ? baseValue : overlayValue;
}

function isObject(value) {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

const manifest = merge(base, overlay);
fs.writeFileSync(outputPath, `${JSON.stringify(manifest, null, 2)}\n`);
EOF
}

require_safari_project() {
    if [[ -d "$SAFARI_XCODE_PROJECT" ]]; then
        return
    fi

    printf 'Expected committed Safari Xcode project at %s\n' "$SAFARI_XCODE_PROJECT" >&2
    exit 1
}

build_safari_app() {
    xcodebuild \
        -project "$SAFARI_XCODE_PROJECT" \
        -scheme "$SAFARI_XCODE_SCHEME" \
        -configuration "$SAFARI_XCODE_CONFIGURATION" \
        -derivedDataPath "$SAFARI_DERIVED_DATA_PATH" \
        build

    if [[ ! -d "$SAFARI_APP_BUNDLE_PATH" ]]; then
        printf 'Expected Safari app bundle was not found at %s\n' "$SAFARI_APP_BUNDLE_PATH" >&2
        exit 1
    fi

    printf 'Safari app bundle: %s\n' "$SAFARI_APP_BUNDLE_PATH"
}

build_all() {
    build_target chromium
    build_target firefox
    build_target safari
    require_safari_project
    build_safari_app
}

case "$TARGET" in
    chromium|firefox)
        build_target "$TARGET"
        ;;
    safari)
        build_target safari
        require_safari_project
        build_safari_app
        ;;
    all)
        build_all
        ;;
    *)
        usage
        exit 1
        ;;
esac
