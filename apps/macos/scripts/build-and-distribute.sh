#!/usr/bin/env bash
#
# Build, package, notarize, and staple the Buildermark Local macOS app.
#
# Prerequisites:
#   - Xcode CLI tools (xcodebuild)
#   - create-dmg: brew install create-dmg  (https://github.com/create-dmg/create-dmg)
#   - A valid Developer ID Application certificate in the keychain
#   - An app-specific password stored in the keychain for notarytool:
#       xcrun notarytool store-credentials "BuildermarkNotary" \
#           --apple-id "you@example.com" \
#           --team-id "YOURTEAMID" \
#           --password "xxxx-xxxx-xxxx-xxxx"
#
# Usage:
#   ./scripts/build-and-distribute.sh                    # build + DMG only
#   ./scripts/build-and-distribute.sh --notarize         # build + DMG + notarize + staple
#
# Environment variables (override defaults):
#   TEAM_ID              - Apple Developer Team ID
#   DEVELOPER_ID         - Code signing identity (default: "Developer ID Application")
#   NOTARY_PROFILE       - notarytool keychain profile (default: "BuildermarkNotary")
#   SCHEME               - Xcode scheme (default: "BuildermarkLocal")
#   CONFIGURATION        - Build configuration (default: "Release")

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_DIR="$PROJECT_DIR/build"
ARCHIVE_PATH="$BUILD_DIR/BuildermarkLocal.xcarchive"
EXPORT_DIR="$BUILD_DIR/export"
DMG_DIR="$BUILD_DIR/dmg"
DMG_OUTPUT="$BUILD_DIR/BuildermarkLocal.dmg"

SCHEME="${SCHEME:-BuildermarkLocal}"
CONFIGURATION="${CONFIGURATION:-Release}"
DEVELOPER_ID="${DEVELOPER_ID:-Developer ID Application}"
NOTARY_PROFILE="${NOTARY_PROFILE:-BuildermarkNotary}"
TEAM_ID="${TEAM_ID:-}"

NOTARIZE=false
for arg in "$@"; do
    case "$arg" in
        --notarize) NOTARIZE=true ;;
        *) echo "Unknown argument: $arg"; exit 1 ;;
    esac
done

APP_NAME="Buildermark Local"

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
check_tool create-dmg "Install via Homebrew: brew install create-dmg"

if $NOTARIZE; then
    check_tool xcrun "Install Xcode Command Line Tools: xcode-select --install"
fi

# ---------------------------------------------------------------------------
# Clean
# ---------------------------------------------------------------------------

step "Cleaning previous build"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# ---------------------------------------------------------------------------
# Resolve packages
# ---------------------------------------------------------------------------

step "Resolving Swift package dependencies"
xcodebuild -project "$PROJECT_DIR/BuildermarkLocal.xcodeproj" \
    -scheme "$SCHEME" \
    -resolvePackageDependencies \
    -clonedSourcePackagesDirPath "$BUILD_DIR/SourcePackages"

# ---------------------------------------------------------------------------
# Archive
# ---------------------------------------------------------------------------

step "Archiving $SCHEME ($CONFIGURATION)"

ARCHIVE_ARGS=(
    -project "$PROJECT_DIR/BuildermarkLocal.xcodeproj"
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

# ---------------------------------------------------------------------------
# Create DMG
# ---------------------------------------------------------------------------

step "Creating DMG"
mkdir -p "$DMG_DIR"

# Remove any previous DMG to avoid create-dmg complaining.
rm -f "$DMG_OUTPUT"

create-dmg \
    --volname "$APP_NAME" \
    --volicon "$APP_PATH/Contents/Resources/AppIcon.icns" \
    --window-pos 200 120 \
    --window-size 600 400 \
    --icon-size 100 \
    --icon "$APP_NAME.app" 150 190 \
    --hide-extension "$APP_NAME.app" \
    --app-drop-link 450 190 \
    "$DMG_OUTPUT" \
    "$APP_PATH" \
    || true  # create-dmg exits 2 when it skips the background; that's fine

if [ ! -f "$DMG_OUTPUT" ]; then
    echo "Error: DMG was not created."
    exit 1
fi

echo "DMG created at: $DMG_OUTPUT"

# ---------------------------------------------------------------------------
# Notarize
# ---------------------------------------------------------------------------

if $NOTARIZE; then
    step "Submitting DMG to Apple for notarization"
    xcrun notarytool submit "$DMG_OUTPUT" \
        --keychain-profile "$NOTARY_PROFILE" \
        --wait

    step "Stapling notarization ticket to DMG"
    xcrun stapler staple "$DMG_OUTPUT"

    echo ""
    echo "Notarized and stapled: $DMG_OUTPUT"
fi

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

step "Build complete"
echo "  App:  $APP_PATH"
echo "  DMG:  $DMG_OUTPUT"
if $NOTARIZE; then
    echo "  Status: Notarized and ready for distribution"
else
    echo "  Status: Built (not notarized — pass --notarize to submit to Apple)"
fi
