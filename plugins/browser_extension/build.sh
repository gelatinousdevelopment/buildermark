#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
SOURCE_DIR="$ROOT_DIR/src"
MANIFEST_DIR="$ROOT_DIR/manifests"
DIST_DIR="$ROOT_DIR/dist"
SAFARI_PROJECT_ROOT="$ROOT_DIR/safari"
SAFARI_VERSION_FILE="$SAFARI_PROJECT_ROOT/.bundle-version"
SAFARI_PBXPROJ="$SAFARI_PROJECT_ROOT/BuildermarkSafari/BuildermarkSafari.xcodeproj/project.pbxproj"

TARGET="${1:-all}"

usage() {
    cat <<'EOF'
Usage:
  ./build.sh [chromium|firefox|safari|all]
EOF
}

build_target() {
    local target="$1"
    local output_dir="$DIST_DIR/$target"

    rm -rf "$output_dir"
    mkdir -p "$output_dir"
    rsync -a --delete --exclude '.DS_Store' "$SOURCE_DIR/" "$output_dir/"

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

ensure_safari_project() {
    mkdir -p "$SAFARI_PROJECT_ROOT"

    (
        cd "$ROOT_DIR"
        xcrun safari-web-extension-converter \
            "dist/safari" \
            --project-location "$SAFARI_PROJECT_ROOT" \
            --app-name BuildermarkSafari \
            --bundle-identifier dev.buildermark.BuildermarkSafari \
            --swift \
            --macos-only \
            --force \
            --no-open \
            --no-prompt
    )

    bump_safari_bundle_version
}

bump_safari_bundle_version() {
    local current_version=0
    local version=1

    if [[ -f "$SAFARI_PBXPROJ" ]]; then
        current_version="$(sed -n 's/.*CURRENT_PROJECT_VERSION = \([0-9][0-9]*\);/\1/p' "$SAFARI_PBXPROJ" | head -n1)"
        if [[ ! "$current_version" =~ ^[0-9]+$ ]]; then
            current_version=0
        fi
    fi

    if [[ -f "$SAFARI_VERSION_FILE" ]]; then
        version="$(<"$SAFARI_VERSION_FILE")"
        if [[ ! "$version" =~ ^[0-9]+$ ]]; then
            version="$current_version"
        fi
    else
        version="$current_version"
    fi

    version=$((version + 1))

    printf '%s\n' "$version" > "$SAFARI_VERSION_FILE"

    node - "$SAFARI_PBXPROJ" "$version" <<'EOF'
const fs = require("fs");

const [projectPath, version] = process.argv.slice(2);
const project = fs.readFileSync(projectPath, "utf8");
const updated = project.replace(/CURRENT_PROJECT_VERSION = \d+;/g, `CURRENT_PROJECT_VERSION = ${version};`);

if (project === updated) {
  throw new Error(`Failed to stamp CURRENT_PROJECT_VERSION in ${projectPath}`);
}

fs.writeFileSync(projectPath, updated);
EOF
}

build_all() {
    build_target chromium
    build_target firefox
    build_target safari
    ensure_safari_project
}

case "$TARGET" in
    chromium|firefox)
        build_target "$TARGET"
        ;;
    safari)
        build_target safari
        ensure_safari_project
        ;;
    all)
        build_all
        ;;
    *)
        usage
        exit 1
        ;;
esac
