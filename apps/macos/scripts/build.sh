#!/usr/bin/env bash
#
# Build the Buildermark macOS app.
#
# Prerequisites:
#   - Xcode CLI tools (xcodebuild)
#   - A valid Developer ID Application certificate in the keychain
#
# Usage:
#   ./scripts/build.sh
#
# Environment variables (override defaults):
#   TEAM_ID              - Apple Developer Team ID
#   DEVELOPER_ID         - Code signing identity (default: "Developer ID Application")
#   SCHEME               - Xcode scheme (default: "Buildermark")
#   CONFIGURATION        - Build configuration (default: "Release")

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_DIR="$PROJECT_DIR/build"
ARCHIVE_PATH="$BUILD_DIR/Buildermark.xcarchive"
EXPORT_DIR="$BUILD_DIR/export"

SCHEME="${SCHEME:-Buildermark}"
CONFIGURATION="${CONFIGURATION:-Release}"
DEVELOPER_ID="${DEVELOPER_ID:-Developer ID Application}"
TEAM_ID="${TEAM_ID:-}"

APP_NAME="Buildermark"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

step() {
    echo ""
    echo "==> $1"
    echo ""
}

check_tool() {
    if ! command -v "$1" &>/dev/null; then
        echo "Error: $1 is not installed."
        echo "  $2"
        exit 1
    fi
}

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------

step "Checking prerequisites"
check_tool xcodebuild "Install Xcode Command Line Tools: xcode-select --install"

# ---------------------------------------------------------------------------
# Clean
# ---------------------------------------------------------------------------

step "Cleaning previous build"
if [ -d "$BUILD_DIR" ]; then
    TRASH="$BUILD_DIR.$$"
    mv "$BUILD_DIR" "$TRASH"
    rm -rf "$TRASH" &
fi
mkdir -p "$BUILD_DIR"

# ---------------------------------------------------------------------------
# Resolve packages
# ---------------------------------------------------------------------------

step "Resolving Swift package dependencies"
xcodebuild -project "$PROJECT_DIR/Buildermark.xcodeproj" \
    -scheme "$SCHEME" \
    -resolvePackageDependencies \
    -clonedSourcePackagesDirPath "$BUILD_DIR/SourcePackages"

# ---------------------------------------------------------------------------
# Archive
# ---------------------------------------------------------------------------

step "Archiving $SCHEME ($CONFIGURATION)"

ARCHIVE_ARGS=(
    -project "$PROJECT_DIR/Buildermark.xcodeproj"
    -scheme "$SCHEME"
    -configuration "$CONFIGURATION"
    -archivePath "$ARCHIVE_PATH"
    -clonedSourcePackagesDirPath "$BUILD_DIR/SourcePackages"
    -destination "generic/platform=macOS"
    archive
)

if [ -n "$TEAM_ID" ]; then
    ARCHIVE_ARGS+=("DEVELOPMENT_TEAM=$TEAM_ID")
fi

xcodebuild "${ARCHIVE_ARGS[@]}"

# ---------------------------------------------------------------------------
# Export
# ---------------------------------------------------------------------------

step "Exporting app from archive"

EXPORT_OPTIONS_PLIST="$BUILD_DIR/ExportOptions.plist"
cat > "$EXPORT_OPTIONS_PLIST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>method</key>
    <string>developer-id</string>
    <key>signingStyle</key>
    <string>automatic</string>
    <key>destination</key>
    <string>export</string>
</dict>
</plist>
PLIST

xcodebuild -exportArchive \
    -archivePath "$ARCHIVE_PATH" \
    -exportPath "$EXPORT_DIR" \
    -exportOptionsPlist "$EXPORT_OPTIONS_PLIST"

APP_PATH="$EXPORT_DIR/$APP_NAME.app"

if [ ! -d "$APP_PATH" ]; then
    echo "Error: exported app not found at $APP_PATH"
    exit 1
fi

step "Build complete"
echo "  App: $APP_PATH"
