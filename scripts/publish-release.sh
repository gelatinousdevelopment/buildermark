#!/usr/bin/env bash
#
# Publish a prepared Buildermark release to GitHub using the GitHub CLI (gh).
#
# Reads artifacts from the release/<version>/ directory produced by
# prepare-release.sh and creates a GitHub release with all files attached.
#
# Usage:
#   ./scripts/publish-release.sh <version>
#   ./scripts/publish-release.sh 1.2.0
#   ./scripts/publish-release.sh 1.2.0 --prerelease
#
# By default, releases are created as drafts. Pass --publish to create a
# public release.
#
# Options:
#   --publish     Create a public (non-draft) release
#   --prerelease  Mark the release as a prerelease
#

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CHANGELOG="$ROOT_DIR/CHANGELOG.md"

DRAFT="--draft"
PRERELEASE=""

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <version> [--publish] [--prerelease]" >&2
    exit 1
fi

VERSION="$1"
shift

while [[ $# -gt 0 ]]; do
    case "$1" in
        --publish)    DRAFT=""; shift ;;
        --prerelease) PRERELEASE="--prerelease"; shift ;;
        *)
            echo "Unknown argument: $1" >&2
            exit 1
            ;;
    esac
done

TAG="v$VERSION"
RELEASE_DIR="$ROOT_DIR/release/$VERSION"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

step() {
    echo ""
    echo "==> $1"
    echo ""
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

# ---------------------------------------------------------------------------
# Validate prerequisites
# ---------------------------------------------------------------------------

step "Validating prerequisites"

if ! command -v gh >/dev/null 2>&1; then
    echo "Error: GitHub CLI (gh) is not installed." >&2
    echo "  Install: brew install gh" >&2
    exit 1
fi

if ! gh auth status >/dev/null 2>&1; then
    echo "Error: Not authenticated with GitHub CLI." >&2
    echo "  Run: gh auth login" >&2
    exit 1
fi

if [[ ! -d "$RELEASE_DIR" ]]; then
    echo "Error: Release directory not found: $RELEASE_DIR" >&2
    echo "  Run prepare-release.sh first: ./scripts/prepare-release.sh $VERSION" >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Read release notes
# ---------------------------------------------------------------------------

step "Preparing release notes"

GITHUB_RELEASE_NOTES="$RELEASE_DIR/GITHUB_RELEASE_NOTES.md"

if [[ ! -f "$CHANGELOG" ]]; then
    echo "Error: CHANGELOG.md not found at $CHANGELOG" >&2
    exit 1
fi

extract_release_notes "$VERSION" "$CHANGELOG" > "$GITHUB_RELEASE_NOTES"

if [[ ! -s "$GITHUB_RELEASE_NOTES" ]]; then
    echo "Error: No changelog entry found for version $VERSION in $CHANGELOG" >&2
    exit 1
fi

echo "  Generated release notes from: $CHANGELOG"
echo "  Output: $GITHUB_RELEASE_NOTES"

# ---------------------------------------------------------------------------
# Collect artifacts (exclude metadata files from upload)
# ---------------------------------------------------------------------------

step "Collecting artifacts"

ARTIFACTS=()
for f in "$RELEASE_DIR"/*; do
    case "$(basename "$f")" in
        APPCAST_RELEASE_NOTES.md) continue ;;
        CHANGELOG.md)             continue ;;
        GITHUB_RELEASE_NOTES.md)  continue ;;
        RELEASE_NOTES.md)         continue ;;
        checksums-sha256.txt)     continue ;;
        appcast.xml)              continue ;;
        linux-update-latest.json) continue ;;
        *) ARTIFACTS+=("$f") ;;
    esac
done

if [[ ${#ARTIFACTS[@]} -eq 0 ]]; then
    echo "Error: No artifacts found in $RELEASE_DIR" >&2
    exit 1
fi

echo "  Found ${#ARTIFACTS[@]} artifact(s):"
for f in "${ARTIFACTS[@]}"; do
    echo "    $(basename "$f")"
done

# ---------------------------------------------------------------------------
# Create GitHub release
# ---------------------------------------------------------------------------

if [[ -n "$DRAFT" ]]; then
    echo "  Mode: DRAFT (pass --publish to create a public release)"
else
    echo "  Mode: PUBLIC"
fi

step "Creating GitHub release: $TAG"

gh release create "$TAG" \
    --title "Buildermark $TAG" \
    --notes-file "$GITHUB_RELEASE_NOTES" \
    $DRAFT \
    $PRERELEASE \
    "${ARTIFACTS[@]}"

step "Release published"
echo "  Tag:     $TAG"
echo "  URL:     $(gh release view "$TAG" --json url --jq '.url')"
