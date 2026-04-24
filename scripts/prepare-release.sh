#!/usr/bin/env bash
#
# Prepare a versioned release of Buildermark for all platforms.
#
# Updates version numbers, builds all apps and browser extensions, and generates
# release notes, Sparkle appcast, and the Linux update manifest.
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
#   SKIP_NOTARIZE          - set to "1" to skip macOS notarization (notarization is on by default)
#

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPTS_DIR="$ROOT_DIR/scripts"
CHANGELOG="$ROOT_DIR/CHANGELOG.md"
RELEASE_BASE_URL="https://buildermark.dev/release"

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

extract_release_notes() {
    local version="$1"
    local changelog_file="$2"

    awk -v ver="$version" '
        /^## \[/ {
            if (found) {
                exit
            }
            if ($0 ~ "^## \\[" ver "\\] - ") {
                found = 1
            }
        }
        found { print }
    ' "$changelog_file"
}

markdown_to_html() {
    awk '
        function escape_html(text) {
            gsub(/&/, "\\&amp;", text)
            gsub(/</, "\\&lt;", text)
            gsub(/>/, "\\&gt;", text)
            gsub(/"/, "\\&quot;", text)
            return text
        }

        function flush_paragraph() {
            if (paragraph != "") {
                print "<p>" paragraph "</p>"
                paragraph = ""
            }
        }

        function flush_list() {
            if (in_list) {
                print "</ul>"
                in_list = 0
            }
        }

        {
            line = $0
            sub(/\r$/, "", line)

            if (line ~ /^[[:space:]]*$/) {
                flush_paragraph()
                flush_list()
                next
            }

            if (line ~ /^### /) {
                flush_paragraph()
                flush_list()
                print "<h3>" escape_html(substr(line, 5)) "</h3>"
                next
            }

            if (line ~ /^## /) {
                flush_paragraph()
                flush_list()
                print "<h2>" escape_html(substr(line, 4)) "</h2>"
                next
            }

            if (line ~ /^- /) {
                flush_paragraph()
                if (!in_list) {
                    print "<ul>"
                    in_list = 1
                }
                print "  <li>" escape_html(substr(line, 3)) "</li>"
                next
            }

            flush_list()
            line = escape_html(line)
            if (paragraph == "") {
                paragraph = line
            } else {
                paragraph = paragraph " " line
            }
        }

        END {
            flush_paragraph()
            flush_list()
        }
    '
}

artifact_download_url() {
    local version="$1"
    local filename="$2"

    printf '%s/%s/%s' "$RELEASE_BASE_URL" "$version" "$filename"
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

        # --- DMG (for direct download and Sparkle auto-updates) ---
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

        # --- Notarize ---
        if [[ "${SKIP_NOTARIZE:-}" != "1" ]]; then
            echo "  Submitting DMG to Apple for notarization ($ARCH)..."
            xcrun notarytool submit "$MACOS_DMG" \
                --keychain-profile "$NOTARY_PROFILE" \
                --wait
            xcrun stapler staple "$MACOS_DMG"
            echo "  Notarized and stapled: $(basename "$MACOS_DMG")"
        else
            echo "  Skipping notarization (SKIP_NOTARIZE=1) — DMG will trigger Gatekeeper warnings on other Macs"
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
        WINDOWS_INSTALLER="$runtime_dir/installer/Buildermark-$VERSION-windows-$runtime-Setup.exe"
        if [[ ! -f "$WINDOWS_INSTALLER" ]]; then
            echo "  Error: Windows installer not found at $WINDOWS_INSTALLER" >&2
            exit 1
        fi

        cp "$WINDOWS_INSTALLER" "$RELEASE_DIR/"
        echo "  OK: $(basename "$WINDOWS_INSTALLER")"
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
            if [[ "$browser" == "safari" ]]; then
                continue
            fi
            EXT_ZIP="$RELEASE_DIR/buildermark-$VERSION-browser-$browser.zip"
            (cd "$ext_dir" && zip -r -q "$EXT_ZIP" .)
            echo "  OK: $(basename "$EXT_ZIP")"
        done

        SAFARI_APP_BUNDLE="$EXT_DIST/BuildermarkSafari.app"
        if [[ -d "$SAFARI_APP_BUNDLE" ]]; then
            SAFARI_ZIP="$RELEASE_DIR/buildermark-$VERSION-browser-safari.zip"
            rm -f "$SAFARI_ZIP"
            ditto -c -k --sequesterRsrc --keepParent "$SAFARI_APP_BUNDLE" "$SAFARI_ZIP"
            echo "  OK: $(basename "$SAFARI_ZIP")"
        else
            echo "  Warning: Safari app bundle not found at $SAFARI_APP_BUNDLE" >&2
        fi
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

cp "$CHANGELOG" "$RELEASE_DIR/CHANGELOG.md"
echo "  OK: CHANGELOG.md"

RELEASE_NOTES="$RELEASE_DIR/RELEASE_NOTES.md"
cp "$CHANGELOG" "$RELEASE_NOTES"
echo "  OK: RELEASE_NOTES.md"

APPCAST_RELEASE_NOTES="$RELEASE_DIR/APPCAST_RELEASE_NOTES.md"
extract_release_notes "$VERSION" "$CHANGELOG" > "$APPCAST_RELEASE_NOTES"

echo "  OK: APPCAST_RELEASE_NOTES.md"

# ---------------------------------------------------------------------------
# Generate checksums
# ---------------------------------------------------------------------------

step "Generating checksums"

CHECKSUMS="$RELEASE_DIR/checksums-sha256.txt"
(cd "$RELEASE_DIR" && shasum -a 256 *.zip *.tar.gz *.dmg *.exe 2>/dev/null | sort) > "$CHECKSUMS" || true
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
        ver_notes="$ver_dir/APPCAST_RELEASE_NOTES.md"

        # Read release notes if available, escape for CDATA.
        NOTES_CONTENT=""
        NOTES_HTML=""
        if [[ -f "$ver_notes" ]]; then
            NOTES_CONTENT="$(cat "$ver_notes")"
            NOTES_HTML="$(printf '%s\n' "$NOTES_CONTENT" | markdown_to_html)"
        fi

        # Get the date from the changelog entry for this version.
        VER_DATE="$(grep -oE "^## \[$ver\] - [0-9]{4}-[0-9]{2}-[0-9]{2}" "$CHANGELOG" | head -1 | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' || echo "$TODAY")"
        # Convert to RFC 2822 format for pubDate.
        PUB_DATE="$(date -jf "%Y-%m-%d" "$VER_DATE" "+%a, %d %b %Y 00:00:00 +0000" 2>/dev/null || echo "$VER_DATE")"

        # --- macOS items ---
        for arch in arm64 amd64; do
            dmg_file="$ver_dir/Buildermark-$ver-macos-$arch.dmg"
            if [[ -f "$dmg_file" ]]; then
                DMG_LENGTH="$(stat -f%z "$dmg_file" 2>/dev/null || stat -c%s "$dmg_file" 2>/dev/null || echo "0")"

                # Sign the DMG with Sparkle's EdDSA key.
                SIG_OUTPUT="$("$SIGN_UPDATE" "$dmg_file" 2>/dev/null || echo "")"
                EDDSA_SIG="$(echo "$SIG_OUTPUT" | grep -oE 'sparkle:edSignature="[^"]*"' | sed 's/sparkle:edSignature="//;s/"$//' || echo "")"

                # If sign_update outputs just the signature string:
                if [[ -z "$EDDSA_SIG" && -n "$SIG_OUTPUT" ]]; then
                    EDDSA_SIG="$(echo "$SIG_OUTPUT" | head -1 | awk '{print $1}')"
                fi

                DOWNLOAD_URL="$(artifact_download_url "$ver" "Buildermark-$ver-macos-$arch.dmg")"

                cat >> "$APPCAST" <<XMLITEM
    <item>
      <title>Version $ver ($arch)</title>
      <sparkle:version>$ver</sparkle:version>
      <sparkle:shortVersionString>$ver</sparkle:shortVersionString>
      <sparkle:minimumSystemVersion>13.0</sparkle:minimumSystemVersion>
      <pubDate>$PUB_DATE</pubDate>
      <description><![CDATA[$NOTES_HTML]]></description>
      <enclosure
        url="$DOWNLOAD_URL"
        length="$DMG_LENGTH"
        type="application/x-apple-diskimage"
        sparkle:edSignature="$EDDSA_SIG"
        sparkle:os="macos" />
    </item>
XMLITEM
                echo "  Signed: Buildermark-$ver-macos-$arch.dmg"
            fi
        done

        # --- Windows items ---
        for runtime in win-x64 win-arm64; do
            installer_file="$ver_dir/Buildermark-$ver-windows-$runtime-Setup.exe"
            if [[ -f "$installer_file" ]]; then
                INSTALLER_LENGTH="$(stat -f%z "$installer_file" 2>/dev/null || stat -c%s "$installer_file" 2>/dev/null || echo "0")"

                SIG_OUTPUT="$("$SIGN_UPDATE" "$installer_file" 2>/dev/null || echo "")"
                EDDSA_SIG="$(echo "$SIG_OUTPUT" | grep -oE 'sparkle:edSignature="[^"]*"' | sed 's/sparkle:edSignature="//;s/"$//' || echo "")"
                if [[ -z "$EDDSA_SIG" && -n "$SIG_OUTPUT" ]]; then
                    EDDSA_SIG="$(echo "$SIG_OUTPUT" | head -1 | awk '{print $1}')"
                fi

                DOWNLOAD_URL="$(artifact_download_url "$ver" "Buildermark-$ver-windows-$runtime-Setup.exe")"

                cat >> "$APPCAST" <<XMLITEM
    <item>
      <title>Version $ver ($runtime)</title>
      <sparkle:version>$ver</sparkle:version>
      <sparkle:shortVersionString>$ver</sparkle:shortVersionString>
      <pubDate>$PUB_DATE</pubDate>
      <description><![CDATA[$NOTES_HTML]]></description>
      <enclosure
        url="$DOWNLOAD_URL"
        length="$INSTALLER_LENGTH"
        type="application/octet-stream"
        sparkle:edSignature="$EDDSA_SIG"
        sparkle:os="windows" />
    </item>
XMLITEM
                echo "  Signed: Buildermark-$ver-windows-$runtime-Setup.exe"
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
      "downloadUrl": "$(artifact_download_url "$VERSION" "buildermark-$VERSION-linux-amd64.tar.gz")",
      "sha256": "$LINUX_AMD64_SHA"
    },
    "linux-arm64": {
      "downloadUrl": "$(artifact_download_url "$VERSION" "buildermark-$VERSION-linux-arm64.tar.gz")",
      "sha256": "$LINUX_ARM64_SHA"
    }
  }
}
MANIFEST

echo "  OK: linux-update-latest.json"

# ---------------------------------------------------------------------------
# Package Linux installer assets
# ---------------------------------------------------------------------------

step "Packaging Linux installer assets"

LINUX_INSTALLER="$RELEASE_DIR/buildermark-install.sh"
cp "$ROOT_DIR/apps/linux-cli/install.sh" "$LINUX_INSTALLER"
chmod +x "$LINUX_INSTALLER"
echo "  OK: $(basename "$LINUX_INSTALLER")"

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
