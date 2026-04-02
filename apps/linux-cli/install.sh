#!/usr/bin/env bash

set -euo pipefail

RELEASES_BASE_URL="https://buildermark.dev/release"
INSTALL_DIR="${BUILDERMARK_INSTALL_DIR:-$HOME/.local/bin}"
BIN_PATH="$INSTALL_DIR/buildermark"
VERSION_OVERRIDE="${BUILDERMARK_VERSION:-}"
DOWNLOAD_URL_OVERRIDE="${BUILDERMARK_DOWNLOAD_URL:-}"

step() {
    echo ""
    echo "==> $1"
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

require_tool() {
    if ! command -v "$1" >/dev/null 2>&1; then
        fail "missing required tool: $1"
    fi
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            fail "unsupported architecture: $(uname -m)"
            ;;
    esac
}

normalize_version() {
    local version="$1"
    version="${version#v}"
    printf '%s\n' "$version"
}

main() {
    [[ "$(uname -s)" == "Linux" ]] || fail "this installer only supports Linux"

    require_tool curl
    require_tool tar
    require_tool mktemp
    require_tool install

    local arch
    arch="$(detect_arch)"

    local version_label="latest"
    local download_url
    if [[ -n "$DOWNLOAD_URL_OVERRIDE" ]]; then
        version_label="custom"
        download_url="$DOWNLOAD_URL_OVERRIDE"
    elif [[ -n "$VERSION_OVERRIDE" ]]; then
        local normalized_version
        normalized_version="$(normalize_version "$VERSION_OVERRIDE")"
        version_label="v$normalized_version"
        download_url="$RELEASES_BASE_URL/$normalized_version/buildermark-$normalized_version-linux-$arch.tar.gz"
    else
        download_url="$RELEASES_BASE_URL/latest/buildermark-linux-$arch.tar.gz"
    fi

    step "Resolving Buildermark release"

    echo "Release: $version_label"
    echo "Arch:    $arch"
    echo "Install: $BIN_PATH"

    local temp_dir
    temp_dir="$(mktemp -d)"
    trap "rm -rf \"$temp_dir\"" EXIT

    local archive_path="$temp_dir/buildermark.tar.gz"
    local extracted_path="$temp_dir/buildermark"

    step "Downloading archive"
    if ! curl -fL "$download_url" -o "$archive_path"; then
        fail "failed to download Buildermark from $download_url"
    fi

    step "Installing binary"
    mkdir -p "$INSTALL_DIR"
    tar -xzf "$archive_path" -C "$temp_dir"
    [[ -f "$extracted_path" ]] || fail "archive did not contain buildermark"
    install -m 0755 "$extracted_path" "$BIN_PATH"

    local installed_version
    installed_version="$("$BIN_PATH" version 2>/dev/null || true)"

    step "Installed"
    if [[ -n "$installed_version" ]]; then
        echo "$installed_version"
    else
        echo "Installed to $BIN_PATH"
    fi

    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        echo ""
        echo "Add Buildermark to your PATH if needed:"
        echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    fi

    local cli_cmd="buildermark"
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        cli_cmd="$BIN_PATH"
    fi

    echo ""
    echo "Next steps:"
    echo "  $cli_cmd version"
    echo "  $cli_cmd service install"
    echo "  $cli_cmd open"
    echo ""
    echo "Update commands:"
    echo "  $cli_cmd update check"
    echo "  $cli_cmd update apply"
}

main "$@"
