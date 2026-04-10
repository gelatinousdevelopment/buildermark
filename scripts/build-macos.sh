#!/usr/bin/env bash
#
# Orchestrate building the full Buildermark stack:
#   1. Svelte frontend  (local/frontend)
#   2. Go server binary  (local/server)  — embeds the frontend build
#   3. macOS app          (apps/macos)    — embeds the Go binary
#
# Usage:
#   ./scripts/build-macos.sh
#   ./scripts/build-macos.sh --arch arm64
#   ./scripts/build-macos.sh --arch amd64

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/local/frontend"
SERVER_DIR="$ROOT_DIR/local/server"
MACOS_DIR="$ROOT_DIR/apps/macos"

SERVER_BINARY="buildermark-server"

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------

ARCH=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch) ARCH="$2"; shift 2 ;;
        *) echo "Unknown argument: $1" >&2; exit 1 ;;
    esac
done

# Map arch for Go cross-compilation.
GOARCH_VAL=""
if [[ -n "$ARCH" ]]; then
    case "$ARCH" in
        arm64) GOARCH_VAL="arm64" ;;
        amd64) GOARCH_VAL="amd64" ;;
        *)
            echo "Error: unsupported architecture '$ARCH'. Use arm64 or amd64." >&2
            exit 1
            ;;
    esac
fi

step() {
    echo ""
    echo "==> $1"
    echo ""
}

# ---------------------------------------------------------------------------
# 1. Build Svelte frontend
# ---------------------------------------------------------------------------

step "Building Svelte frontend"
cd "$FRONTEND_DIR"
npm ci
npm run build

# Copy the full build output into the Go server's embed path so it gets compiled
# into the binary (//go:embed frontend in dashboard.go).
rm -rf "$SERVER_DIR/internal/handler/frontend"
cp -R "$FRONTEND_DIR/build" "$SERVER_DIR/internal/handler/frontend"

# ---------------------------------------------------------------------------
# 2. Build Go server
# ---------------------------------------------------------------------------

ARCH_LABEL="${ARCH:-native}"
step "Building Go server ($ARCH_LABEL)"
cd "$SERVER_DIR"

GO_BUILD_ENV=(CGO_ENABLED=1)
if [[ -n "$GOARCH_VAL" ]]; then
    GO_BUILD_ENV+=(GOARCH="$GOARCH_VAL")
fi

env "${GO_BUILD_ENV[@]}" go build -o "$SERVER_BINARY" ./cmd/buildermark

# ---------------------------------------------------------------------------
# 3. Build macOS app
# ---------------------------------------------------------------------------

step "Building macOS app ($ARCH_LABEL)"

BUILD_ARGS=()
if [[ -n "$ARCH" ]]; then
    BUILD_ARGS+=(--arch "$ARCH")
fi

"$MACOS_DIR/scripts/build.sh" "${BUILD_ARGS[@]}"

# Copy the server binary into the exported app bundle so ServerManager.swift
# can find it via Bundle.main.url(forResource:).
if [[ -n "$ARCH" ]]; then
    APP_PATH="$MACOS_DIR/build/export-$ARCH/Buildermark.app"
else
    APP_PATH="$MACOS_DIR/build/export/Buildermark.app"
fi
cp "$SERVER_DIR/$SERVER_BINARY" "$APP_PATH/Contents/Resources/$SERVER_BINARY"

# ---------------------------------------------------------------------------
# 4. Re-sign the app to include the embedded server binary
# ---------------------------------------------------------------------------
#
# Adding the server binary after `xcodebuild -exportArchive` invalidates the
# bundle's code signature ("a sealed resource is missing or invalid"). On
# other Macs, Gatekeeper reports this as either:
#   - "Buildermark is damaged and can't be opened."
#   - "Apple could not verify Buildermark is free of malware..."
# Re-sign the helper binary first (with hardened runtime + timestamp, both
# required for notarization), then re-sign the outer .app bundle so its
# CodeResources is regenerated to include the new file. We deliberately do
# NOT pass --deep so the nested Sparkle.framework signatures Xcode produced
# are preserved.

step "Re-signing app to include embedded server binary"

# Loud failure handler — without this, any failed command in this section
# exits silently because of `set -euo pipefail`, which is exactly what
# happened the first time around.
resign_failed() {
    echo "" >&2
    echo "ERROR: re-signing failed at line $1." >&2
    echo "  App:           $APP_PATH" >&2
    echo "  Helper binary: $APP_PATH/Contents/Resources/$SERVER_BINARY" >&2
    echo "  Identity:      ${SIGN_IDENTITY:-<not yet detected>}" >&2
    echo "" >&2
    echo "  Most common causes:" >&2
    echo "    - Keychain locked / Developer ID private key not accessible" >&2
    echo "    - --timestamp could not reach Apple's timestamp server" >&2
    echo "    - DEVELOPER_ID does not match any cert in the keychain" >&2
    exit 1
}
trap 'resign_failed $LINENO' ERR

# Detect the signing identity Xcode used so re-signing stays consistent.
# Use a temp file instead of `codesign | awk ... exit` — that pipeline gives
# codesign SIGPIPE under `set -o pipefail` and aborts the script silently.
echo "  Reading existing signature from app bundle"
CODESIGN_INFO_TMP="$(mktemp -t buildermark-codesign)"
if ! codesign -dvvv "$APP_PATH" >"$CODESIGN_INFO_TMP" 2>&1; then
    echo "  Error: 'codesign -dvvv $APP_PATH' failed:" >&2
    sed 's/^/    /' "$CODESIGN_INFO_TMP" >&2
    rm -f "$CODESIGN_INFO_TMP"
    exit 1
fi
SIGN_IDENTITY="$(grep -m1 '^Authority=' "$CODESIGN_INFO_TMP" | sed 's/^Authority=//' || true)"
rm -f "$CODESIGN_INFO_TMP"

if [[ -z "$SIGN_IDENTITY" ]]; then
    SIGN_IDENTITY="${DEVELOPER_ID:-Developer ID Application}"
    echo "  No Authority found in existing signature; falling back to: $SIGN_IDENTITY"
else
    echo "  Detected signing identity: $SIGN_IDENTITY"
fi

ENTITLEMENTS="$MACOS_DIR/Buildermark/Buildermark.entitlements"
if [[ ! -f "$ENTITLEMENTS" ]]; then
    echo "  Error: entitlements file not found at $ENTITLEMENTS" >&2
    exit 1
fi

echo "  Signing helper binary (hardened runtime + timestamp)"
codesign --force \
    --sign "$SIGN_IDENTITY" \
    --options runtime \
    --timestamp \
    "$APP_PATH/Contents/Resources/$SERVER_BINARY"

echo "  Re-signing outer .app bundle"
codesign --force \
    --sign "$SIGN_IDENTITY" \
    --options runtime \
    --timestamp \
    --entitlements "$ENTITLEMENTS" \
    "$APP_PATH"

echo "  Verifying re-signed bundle"
codesign --verify --deep --strict --verbose=2 "$APP_PATH"

trap - ERR

step "Full build complete ($ARCH_LABEL)"
echo "  App: $APP_PATH"
