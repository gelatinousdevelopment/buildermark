#!/usr/bin/env bash
#
# Build a versioned release of Buildermark for all platforms.
#
# Runs on a macOS host. Uses the VM build scripts for Windows and Linux,
# the native macOS build script, and the browser extension build script.
#
# Validates that CHANGELOG.md has an entry for the given version with today's
# date before proceeding.
#
# All artifacts are collected into:
#   release/<version>/
#
# Usage:
#   ./scripts/build-release.sh <version>
#   ./scripts/build-release.sh 1.2.0
#
# Environment variables:
#   SKIP_MACOS             - set to "1" to skip macOS build
#   SKIP_WINDOWS           - set to "1" to skip Windows VM build
#   SKIP_LINUX             - set to "1" to skip Linux VM build
#   SKIP_BROWSER           - set to "1" to skip browser extension build
#

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPTS_DIR="$ROOT_DIR/scripts"
CHANGELOG="$ROOT_DIR/CHANGELOG.md"

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <version>" >&2
    echo "  e.g. $0 1.2.0" >&2
    exit 1
fi

VERSION="$1"

# Validate version looks like semver (loose check).
if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-.].+)?$ ]]; then
    echo "Error: version '$VERSION' does not look like a valid semver string." >&2
    exit 1
fi

RELEASE_DIR="$ROOT_DIR/release/$VERSION"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

step() {
    echo ""
    echo "==========================================="
    echo "  $1"
    echo "==========================================="
    echo ""
}

# ---------------------------------------------------------------------------
# Validate CHANGELOG.md
# ---------------------------------------------------------------------------

step "Validating CHANGELOG.md"

if [[ ! -f "$CHANGELOG" ]]; then
    echo "Error: CHANGELOG.md not found at $CHANGELOG" >&2
    exit 1
fi

TODAY="$(date +%Y-%m-%d)"

# Look for a heading like: ## [1.2.0] - 2026-03-20
if ! grep -qE "^## \[$VERSION\] - $TODAY" "$CHANGELOG"; then
    echo "Error: CHANGELOG.md does not contain an entry for version $VERSION with today's date ($TODAY)." >&2
    echo "" >&2
    echo "Expected a line matching:" >&2
    echo "  ## [$VERSION] - $TODAY" >&2
    echo "" >&2
    echo "Please update CHANGELOG.md before building a release." >&2
    exit 1
fi

echo "  Found changelog entry: ## [$VERSION] - $TODAY"

# ---------------------------------------------------------------------------
# Prepare release directory
# ---------------------------------------------------------------------------

step "Preparing release directory"

if [[ -d "$RELEASE_DIR" ]]; then
    echo "  Cleaning existing release directory: $RELEASE_DIR"
    rm -rf "$RELEASE_DIR"
fi

mkdir -p "$RELEASE_DIR"
echo "  Output: $RELEASE_DIR"

# ---------------------------------------------------------------------------
# macOS build
# ---------------------------------------------------------------------------

if [[ "${SKIP_MACOS:-}" != "1" ]]; then
    step "Building macOS app"
    "$SCRIPTS_DIR/build-macos.sh"

    MACOS_APP="$ROOT_DIR/apps/macos/build/export/Buildermark.app"
    if [[ -d "$MACOS_APP" ]]; then
        MACOS_ZIP="$RELEASE_DIR/Buildermark-$VERSION-macos.zip"
        (cd "$(dirname "$MACOS_APP")" && zip -r -q "$MACOS_ZIP" "$(basename "$MACOS_APP")")
        echo "  OK: $MACOS_ZIP"
    else
        echo "  Warning: macOS app not found at $MACOS_APP" >&2
    fi
else
    echo "Skipping macOS build (SKIP_MACOS=1)"
fi

# ---------------------------------------------------------------------------
# Windows VM build
# ---------------------------------------------------------------------------

if [[ "${SKIP_WINDOWS:-}" != "1" ]]; then
    step "Building Windows app (VM)"
    "$SCRIPTS_DIR/build-windows-vm.sh" --arch all

    WINDOWS_BUILD="$ROOT_DIR/apps/windows/build"
    for runtime_dir in "$WINDOWS_BUILD"/*/; do
        runtime="$(basename "$runtime_dir")"
        WINDOWS_ZIP="$RELEASE_DIR/Buildermark-$VERSION-windows-$runtime.zip"
        (cd "$runtime_dir" && zip -r -q "$WINDOWS_ZIP" .)
        echo "  OK: $WINDOWS_ZIP"
    done
else
    echo "Skipping Windows build (SKIP_WINDOWS=1)"
fi

# ---------------------------------------------------------------------------
# Linux VM build
# ---------------------------------------------------------------------------

if [[ "${SKIP_LINUX:-}" != "1" ]]; then
    step "Building Linux CLI (VM)"
    "$SCRIPTS_DIR/build-linux-vm.sh" --arch all --version "$VERSION"

    LINUX_BUILD="$ROOT_DIR/apps/linux-cli/build"
    for arch_dir in "$LINUX_BUILD"/*/; do
        arch="$(basename "$arch_dir")"
        LINUX_TAR="$RELEASE_DIR/buildermark-$VERSION-linux-$arch.tar.gz"
        tar -czf "$LINUX_TAR" -C "$arch_dir" .
        echo "  OK: $LINUX_TAR"
    done
else
    echo "Skipping Linux build (SKIP_LINUX=1)"
fi

# ---------------------------------------------------------------------------
# Browser extensions
# ---------------------------------------------------------------------------

if [[ "${SKIP_BROWSER:-}" != "1" ]]; then
    step "Building browser extensions"
    "$SCRIPTS_DIR/build-browser-extensions.sh"

    EXT_DIST="$ROOT_DIR/plugins/browser_extension/dist"
    if [[ -d "$EXT_DIST" ]]; then
        for ext_dir in "$EXT_DIST"/*/; do
            browser="$(basename "$ext_dir")"
            EXT_ZIP="$RELEASE_DIR/buildermark-$VERSION-browser-$browser.zip"
            (cd "$ext_dir" && zip -r -q "$EXT_ZIP" .)
            echo "  OK: $EXT_ZIP"
        done
    else
        echo "  Warning: browser extension dist not found at $EXT_DIST" >&2
    fi
else
    echo "Skipping browser extension build (SKIP_BROWSER=1)"
fi

# ---------------------------------------------------------------------------
# Copy changelog excerpt
# ---------------------------------------------------------------------------

step "Extracting release notes"

# Extract the section for this version from the changelog.
# Grab everything between "## [VERSION]" and the next "## [" heading.
RELEASE_NOTES="$RELEASE_DIR/RELEASE_NOTES.md"
awk -v ver="$VERSION" '
    /^## \[/ {
        if (found) exit
        if (index($0, "[" ver "]")) { found=1 }
    }
    found { print }
' "$CHANGELOG" > "$RELEASE_NOTES"

echo "  OK: $RELEASE_NOTES"

# ---------------------------------------------------------------------------
# Generate checksums
# ---------------------------------------------------------------------------

step "Generating checksums"

CHECKSUMS="$RELEASE_DIR/checksums-sha256.txt"
(cd "$RELEASE_DIR" && shasum -a 256 *.zip *.tar.gz 2>/dev/null | sort) > "$CHECKSUMS" || true
echo "  OK: $CHECKSUMS"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

step "Release build complete: v$VERSION"
echo "  Release directory: $RELEASE_DIR"
echo ""
echo "  Artifacts:"
ls -1 "$RELEASE_DIR" | sed 's/^/    /'
echo ""
echo "  To publish this release:"
echo "    ./scripts/publish-release.sh $VERSION"
