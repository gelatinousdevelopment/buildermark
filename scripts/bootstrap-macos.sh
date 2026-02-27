#!/usr/bin/env bash
#
# Bootstrap the Buildermark development environment on macOS.
# Installs prerequisites and project dependencies.
#
# Usage:
#   ./scripts/bootstrap-macos.sh
#

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/local/frontend"
SERVER_DIR="$ROOT_DIR/local/server"

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
# System prerequisites
# ---------------------------------------------------------------------------

step "Checking system prerequisites"

# Xcode CLI tools (provides git, clang for CGO, xcodebuild)
if ! xcode-select -p &>/dev/null; then
    echo "Installing Xcode Command Line Tools..."
    xcode-select --install
    echo "Re-run this script after the install completes."
    exit 0
fi
echo "  Xcode CLI tools: OK"

# Homebrew
check_tool brew "Install Homebrew: https://brew.sh"
echo "  Homebrew: OK"

# Node.js
if ! command -v node &>/dev/null; then
    step "Installing Node.js via Homebrew"
    brew install node
fi
echo "  Node.js: $(node --version)"

# Go
if ! command -v go &>/dev/null; then
    step "Installing Go via Homebrew"
    brew install go
fi
echo "  Go: $(go version | awk '{print $3}')"

# ---------------------------------------------------------------------------
# Frontend dependencies
# ---------------------------------------------------------------------------

step "Installing frontend dependencies"
cd "$FRONTEND_DIR"
npm install

# ---------------------------------------------------------------------------
# Go dependencies
# ---------------------------------------------------------------------------

step "Downloading Go modules"
cd "$SERVER_DIR"
go mod download

# Verify CGO builds (go-sqlite3 requires it)
step "Verifying Go build (CGO)"
CGO_ENABLED=1 go build ./...

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

step "Bootstrap complete"
echo "  Frontend deps: $FRONTEND_DIR/node_modules"
echo "  Go modules:    $(go env GOMODCACHE)"
echo ""
echo "  To build everything:  ./scripts/build.sh"
echo "  To run the server:    cd local/server && go run ./cmd/buildermark"
echo "  To run the frontend:  cd local/frontend && npm run dev"
