#!/usr/bin/env bash
#
# Prepare a versioned release of Buildermark for all platforms.
#
# Updates version numbers, builds all apps and browser extensions, generates
# release notes, Sparkle appcast, Linux update manifest, and fills Homebrew
# SHA256 checksums.
#
# All artifacts are collected into:
#   release/<version>/
#
# Usage:
#   ./scripts/prepare-release.sh <version>
#   ./scripts/prepare-release.sh 1.2.0
#
# Environment variables:
#   SKIP_MACOS             - set to "1" to skip macOS build
#   SKIP_WINDOWS           - set to "1" to skip Windows VM build
#   SKIP_LINUX             - set to "1" to skip Linux VM build
#   SKIP_BROWSER           - set to "1" to skip browser extension build
#   NOTARIZE               - set to "1" to notarize macOS DMGs
#

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPTS_DIR="$ROOT_DIR/scripts"
CHANGELOG="$ROOT_DIR/CHANGELOG.md"
GITHUB_REPO="buildermark/buildermark"

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
TAG="v$VERSION"

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

check_tool() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "Error: $1 is not installed." >&2
        echo "  $2" >&2
        exit 1
    fi
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
    echo "Please update CHANGELOG.md before preparing a release." >&2
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
# Update version numbers
# ---------------------------------------------------------------------------

step "Updating version numbers to $VERSION"

# Browser extension manifest
sed -i '' "s/\"version\": \"[^\"]*\"/\"version\": \"$VERSION\"/" \
    "$ROOT_DIR/plugins/browser_extension/manifests/base.json"
echo "  Updated: plugins/browser_extension/manifests/base.json"

# macOS Xcode project (MARKETING_VERSION and CURRENT_PROJECT_VERSION)
sed -i '' "s/MARKETING_VERSION = [^;]*/MARKETING_VERSION = $VERSION/" \
    "$ROOT_DIR/apps/macos/Buildermark.xcodeproj/project.pbxproj"
sed -i '' "s/CURRENT_PROJECT_VERSION = [^;]*/CURRENT_PROJECT_VERSION = $VERSION/" \
    "$ROOT_DIR/apps/macos/Buildermark.xcodeproj/project.pbxproj"
echo "  Updated: apps/macos/Buildermark.xcodeproj/project.pbxproj"

# Windows .csproj
sed -i '' "s|<Version>[^<]*</Version>|<Version>$VERSION</Version>|" \
    "$ROOT_DIR/apps/windows/Buildermark/Buildermark.csproj"
sed -i '' "s|<AssemblyVersion>[^<]*</AssemblyVersion>|<AssemblyVersion>$VERSION.0</AssemblyVersion>|" \
    "$ROOT_DIR/apps/windows/Buildermark/Buildermark.csproj"
sed -i '' "s|<FileVersion>[^<]*</FileVersion>|<FileVersion>$VERSION.0</FileVersion>|" \
    "$ROOT_DIR/apps/windows/Buildermark/Buildermark.csproj"
echo "  Updated: apps/windows/Buildermark/Buildermark.csproj"

# Homebrew formulas and cask
for rb in \
    "$ROOT_DIR/apps/homebrew/Formula/buildermark.rb" \
    "$ROOT_DIR/apps/homebrew/Formula/buildermark-linux.rb" \
    "$ROOT_DIR/apps/homebrew/Casks/buildermark-app.rb"; do
    sed -i '' "s/version \"[^\"]*\"/version \"$VERSION\"/" "$rb"
    echo "  Updated: ${rb#$ROOT_DIR/}"
done

# Frontend package.json
sed -i '' "s/\"version\": \"[^\"]*\"/\"version\": \"$VERSION\"/" \
    "$ROOT_DIR/local/frontend/package.json"
echo "  Updated: local/frontend/package.json"

# ---------------------------------------------------------------------------
# macOS build (arm64 + amd64)
# ---------------------------------------------------------------------------

if [[ "${SKIP_MACOS:-}" != "1" ]]; then
    check_tool create-dmg "Install via: npm install -g create-dmg"

    NOTARY_PROFILE="${NOTARY_PROFILE:-BuildermarkNotary}"

    for ARCH in arm64 amd64; do
        step "Building macOS app ($ARCH)"
        "$SCRIPTS_DIR/build-macos.sh" --arch "$ARCH"

        MACOS_APP="$ROOT_DIR/apps/macos/build/export-$ARCH/Buildermark.app"
        if [[ ! -d "$MACOS_APP" ]]; then
            echo "  Error: macOS app not found at $MACOS_APP" >&2
            exit 1
        fi

        # --- ZIP (for Sparkle auto-updates) ---
        MACOS_ZIP="$RELEASE_DIR/Buildermark-$VERSION-macos-$ARCH.zip"
        (cd "$(dirname "$MACOS_APP")" && zip -r -q "$MACOS_ZIP" "$(basename "$MACOS_APP")")
        echo "  OK: $(basename "$MACOS_ZIP")"

        # --- DMG (for Homebrew / direct download) ---
        MACOS_DMG="$RELEASE_DIR/Buildermark-$VERSION-macos-$ARCH.dmg"
        create-dmg --overwrite "$MACOS_APP" "$RELEASE_DIR"

        # create-dmg names the output automatically; rename to our convention.
        DMG_GENERATED="$(ls "$RELEASE_DIR"/Buildermark*.dmg 2>/dev/null | grep -v "macos-" | head -1)"
        if [[ -n "$DMG_GENERATED" && "$DMG_GENERATED" != "$MACOS_DMG" ]]; then
            mv "$DMG_GENERATED" "$MACOS_DMG"
        fi

        if [[ ! -f "$MACOS_DMG" ]]; then
            echo "  Error: DMG was not created for $ARCH" >&2
            exit 1
        fi

        # --- Notarize (optional) ---
        if [[ "${NOTARIZE:-}" == "1" ]]; then
            echo "  Submitting DMG to Apple for notarization ($ARCH)..."
            xcrun notarytool submit "$MACOS_DMG" \
                --keychain-profile "$NOTARY_PROFILE" \
                --wait
            xcrun stapler staple "$MACOS_DMG"
            echo "  Notarized and stapled: $(basename "$MACOS_DMG")"
        fi

        echo "  OK: $(basename "$MACOS_DMG")"
    done
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
        echo "  OK: $(basename "$WINDOWS_ZIP")"
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
        echo "  OK: $(basename "$LINUX_TAR")"
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
            echo "  OK: $(basename "$EXT_ZIP")"
        done
    else
        echo "  Warning: browser extension dist not found at $EXT_DIST" >&2
    fi
else
    echo "Skipping browser extension build (SKIP_BROWSER=1)"
fi

# ---------------------------------------------------------------------------
# Extract release notes
# ---------------------------------------------------------------------------

step "Extracting release notes"

RELEASE_NOTES="$RELEASE_DIR/RELEASE_NOTES.md"
awk -v ver="$VERSION" '
    /^## \[/ {
        if (found) exit
        if (index($0, "[" ver "]")) { found=1 }
    }
    found { print }
' "$CHANGELOG" > "$RELEASE_NOTES"

echo "  OK: RELEASE_NOTES.md"

# ---------------------------------------------------------------------------
# Generate checksums
# ---------------------------------------------------------------------------

step "Generating checksums"

CHECKSUMS="$RELEASE_DIR/checksums-sha256.txt"
(cd "$RELEASE_DIR" && shasum -a 256 *.zip *.tar.gz *.dmg 2>/dev/null | sort) > "$CHECKSUMS" || true
echo "  OK: checksums-sha256.txt"

# ---------------------------------------------------------------------------
# Generate Sparkle appcast.xml (from all releases)
# ---------------------------------------------------------------------------

step "Generating Sparkle appcast.xml"

SIGN_UPDATE="$ROOT_DIR/apps/macos/build/SourcePackages/artifacts/sparkle/Sparkle/bin/sign_update"

if [[ ! -x "$SIGN_UPDATE" ]]; then
    echo "  Warning: Sparkle sign_update tool not found at:" >&2
    echo "    $SIGN_UPDATE" >&2
    echo "  Build the macOS app first to fetch Sparkle, or run:" >&2
    echo "    xcodebuild -project apps/macos/Buildermark.xcodeproj -scheme Buildermark -resolvePackageDependencies" >&2
    echo "  Skipping appcast generation." >&2
else
    APPCAST="$RELEASE_DIR/appcast.xml"

    # Start the appcast XML.
    cat > "$APPCAST" <<'XMLHEADER'
<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>Buildermark Updates</title>
    <description>Buildermark release updates</description>
    <language>en</language>
XMLHEADER

    # Iterate over all release directories (sorted newest first by version).
    # List version basenames, sort by version descending, then reconstruct paths.
    for ver in $(ls "$ROOT_DIR/release" 2>/dev/null | sort -V -r); do
        ver_dir="$ROOT_DIR/release/$ver"
        [[ -d "$ver_dir" ]] || continue
        ver_notes="$ver_dir/RELEASE_NOTES.md"

        # Read release notes if available, escape for CDATA.
        NOTES_CONTENT=""
        if [[ -f "$ver_notes" ]]; then
            NOTES_CONTENT="$(cat "$ver_notes")"
        fi

        # Get the date from the changelog entry for this version.
        VER_DATE="$(grep -oE "^## \[$ver\] - [0-9]{4}-[0-9]{2}-[0-9]{2}" "$CHANGELOG" | head -1 | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' || echo "$TODAY")"
        # Convert to RFC 2822 format for pubDate.
        PUB_DATE="$(date -jf "%Y-%m-%d" "$VER_DATE" "+%a, %d %b %Y 00:00:00 +0000" 2>/dev/null || echo "$VER_DATE")"

        # --- macOS items ---
        for arch in arm64 amd64; do
            zip_file="$ver_dir/Buildermark-$ver-macos-$arch.zip"
            if [[ -f "$zip_file" ]]; then
                ZIP_LENGTH="$(stat -f%z "$zip_file" 2>/dev/null || stat -c%s "$zip_file" 2>/dev/null || echo "0")"

                # Sign the ZIP with Sparkle's EdDSA key.
                SIG_OUTPUT="$("$SIGN_UPDATE" "$zip_file" 2>/dev/null || echo "")"
                EDDSA_SIG="$(echo "$SIG_OUTPUT" | grep -oE 'sparkle:edSignature="[^"]*"' | sed 's/sparkle:edSignature="//;s/"$//' || echo "")"

                # If sign_update outputs just the signature string:
                if [[ -z "$EDDSA_SIG" && -n "$SIG_OUTPUT" ]]; then
                    EDDSA_SIG="$(echo "$SIG_OUTPUT" | head -1 | awk '{print $1}')"
                fi

                DOWNLOAD_URL="https://github.com/$GITHUB_REPO/releases/download/v$ver/Buildermark-$ver-macos-$arch.zip"

                cat >> "$APPCAST" <<XMLITEM
    <item>
      <title>Version $ver ($arch)</title>
      <sparkle:version>$ver</sparkle:version>
      <sparkle:shortVersionString>$ver</sparkle:shortVersionString>
      <sparkle:minimumSystemVersion>13.0</sparkle:minimumSystemVersion>
      <pubDate>$PUB_DATE</pubDate>
      <description><![CDATA[$NOTES_CONTENT]]></description>
      <enclosure
        url="$DOWNLOAD_URL"
        length="$ZIP_LENGTH"
        type="application/octet-stream"
        sparkle:edSignature="$EDDSA_SIG"
        sparkle:os="macos" />
    </item>
XMLITEM
                echo "  Signed: Buildermark-$ver-macos-$arch.zip"
            fi
        done

        # --- Windows items ---
        for runtime in win-x64 win-arm64; do
            zip_file="$ver_dir/Buildermark-$ver-windows-$runtime.zip"
            if [[ -f "$zip_file" ]]; then
                ZIP_LENGTH="$(stat -f%z "$zip_file" 2>/dev/null || stat -c%s "$zip_file" 2>/dev/null || echo "0")"

                SIG_OUTPUT="$("$SIGN_UPDATE" "$zip_file" 2>/dev/null || echo "")"
                EDDSA_SIG="$(echo "$SIG_OUTPUT" | grep -oE 'sparkle:edSignature="[^"]*"' | sed 's/sparkle:edSignature="//;s/"$//' || echo "")"
                if [[ -z "$EDDSA_SIG" && -n "$SIG_OUTPUT" ]]; then
                    EDDSA_SIG="$(echo "$SIG_OUTPUT" | head -1 | awk '{print $1}')"
                fi

                DOWNLOAD_URL="https://github.com/$GITHUB_REPO/releases/download/v$ver/Buildermark-$ver-windows-$runtime.zip"

                cat >> "$APPCAST" <<XMLITEM
    <item>
      <title>Version $ver ($runtime)</title>
      <sparkle:version>$ver</sparkle:version>
      <sparkle:shortVersionString>$ver</sparkle:shortVersionString>
      <pubDate>$PUB_DATE</pubDate>
      <description><![CDATA[$NOTES_CONTENT]]></description>
      <enclosure
        url="$DOWNLOAD_URL"
        length="$ZIP_LENGTH"
        type="application/octet-stream"
        sparkle:edSignature="$EDDSA_SIG"
        sparkle:os="windows" />
    </item>
XMLITEM
                echo "  Signed: Buildermark-$ver-windows-$runtime.zip"
            fi
        done
    done

    # Close the appcast XML.
    cat >> "$APPCAST" <<'XMLFOOTER'
  </channel>
</rss>
XMLFOOTER

    echo "  OK: appcast.xml"
fi

# ---------------------------------------------------------------------------
# Generate Linux update manifest
# ---------------------------------------------------------------------------

step "Generating Linux update manifest"

LINUX_MANIFEST="$RELEASE_DIR/linux-update-latest.json"

# Extract SHA256 values for Linux artifacts from the checksums file.
LINUX_AMD64_SHA=""
LINUX_ARM64_SHA=""
if [[ -f "$CHECKSUMS" ]]; then
    LINUX_AMD64_SHA="$(grep "buildermark-$VERSION-linux-amd64.tar.gz" "$CHECKSUMS" | awk '{print $1}' || echo "")"
    LINUX_ARM64_SHA="$(grep "buildermark-$VERSION-linux-arm64.tar.gz" "$CHECKSUMS" | awk '{print $1}' || echo "")"
fi

cat > "$LINUX_MANIFEST" <<MANIFEST
{
  "version": "$VERSION",
  "artifacts": {
    "linux-amd64": {
      "downloadUrl": "https://github.com/$GITHUB_REPO/releases/download/$TAG/buildermark-$VERSION-linux-amd64.tar.gz",
      "sha256": "$LINUX_AMD64_SHA"
    },
    "linux-arm64": {
      "downloadUrl": "https://github.com/$GITHUB_REPO/releases/download/$TAG/buildermark-$VERSION-linux-arm64.tar.gz",
      "sha256": "$LINUX_ARM64_SHA"
    }
  }
}
MANIFEST

echo "  OK: linux-update-latest.json"

# ---------------------------------------------------------------------------
# Fill Homebrew SHA256 checksums
# ---------------------------------------------------------------------------

step "Updating Homebrew SHA256 checksums"

if [[ -f "$CHECKSUMS" ]]; then
    # Extract checksums from the checksums file.
    SHA_MACOS_ARM64="$(grep "Buildermark-$VERSION-macos-arm64.dmg" "$CHECKSUMS" | awk '{print $1}' || echo "")"
    SHA_MACOS_AMD64="$(grep "Buildermark-$VERSION-macos-amd64.dmg" "$CHECKSUMS" | awk '{print $1}' || echo "")"
    SHA_LINUX_AMD64="$LINUX_AMD64_SHA"
    SHA_LINUX_ARM64="$LINUX_ARM64_SHA"

    BREW_FORMULA="$ROOT_DIR/apps/homebrew/Formula/buildermark.rb"
    BREW_LINUX="$ROOT_DIR/apps/homebrew/Formula/buildermark-linux.rb"
    BREW_CASK="$ROOT_DIR/apps/homebrew/Casks/buildermark-app.rb"

    # Replace REPLACE_WITH_* placeholders (or previous SHA256 hex values)
    # with the actual checksums from this build.
    brew_replace() {
        local file="$1" placeholder="$2" sha="$3"
        if [[ -z "$sha" ]]; then return; fi
        # Replace the named placeholder.
        sed -i '' "s/$placeholder/$sha/g" "$file"
        # Also handle re-runs where a previous real SHA is present:
        # the placeholder won't exist, but the version bump in step 2c
        # keeps the file structure stable.
    }

    # buildermark.rb — macOS DMGs + Linux tarballs
    brew_replace "$BREW_FORMULA" "REPLACE_WITH_ARM64_DMG_SHA256" "$SHA_MACOS_ARM64"
    brew_replace "$BREW_FORMULA" "REPLACE_WITH_AMD64_DMG_SHA256" "$SHA_MACOS_AMD64"
    brew_replace "$BREW_FORMULA" "REPLACE_WITH_ARM64_TAR_SHA256" "$SHA_LINUX_ARM64"
    brew_replace "$BREW_FORMULA" "REPLACE_WITH_AMD64_TAR_SHA256" "$SHA_LINUX_AMD64"
    echo "  Updated: apps/homebrew/Formula/buildermark.rb"

    # buildermark-linux.rb — Linux tarballs only
    brew_replace "$BREW_LINUX" "REPLACE_WITH_AMD64_TAR_SHA256" "$SHA_LINUX_AMD64"
    brew_replace "$BREW_LINUX" "REPLACE_WITH_ARM64_TAR_SHA256" "$SHA_LINUX_ARM64"
    echo "  Updated: apps/homebrew/Formula/buildermark-linux.rb"

    # buildermark-app.rb (cask) — macOS DMGs only
    brew_replace "$BREW_CASK" "REPLACE_WITH_ARM64_DMG_SHA256" "$SHA_MACOS_ARM64"
    brew_replace "$BREW_CASK" "REPLACE_WITH_AMD64_DMG_SHA256" "$SHA_MACOS_AMD64"
    echo "  Updated: apps/homebrew/Casks/buildermark-app.rb"

    # Copy the updated Homebrew files into the release directory.
    BREW_RELEASE_DIR="$RELEASE_DIR/homebrew"
    mkdir -p "$BREW_RELEASE_DIR/Formula" "$BREW_RELEASE_DIR/Casks"
    cp "$BREW_FORMULA" "$BREW_RELEASE_DIR/Formula/"
    cp "$BREW_LINUX"   "$BREW_RELEASE_DIR/Formula/"
    cp "$BREW_CASK"    "$BREW_RELEASE_DIR/Casks/"
    echo "  Copied Homebrew files to: release/$VERSION/homebrew/"
else
    echo "  Warning: checksums file not found, skipping Homebrew SHA256 updates" >&2
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

step "Release preparation complete: $TAG"
echo "  Release directory: $RELEASE_DIR"
echo ""
echo "  Artifacts:"
ls -1 "$RELEASE_DIR" | sed 's/^/    /'
echo ""
echo "  Version files updated — review with: git diff"
echo ""
echo "  To publish this release:"
echo "    ./scripts/publish-release.sh $VERSION"
