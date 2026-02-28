#!/usr/bin/env bash
#
# Bootstrap the Buildermark development environment on Linux.
# Installs project dependencies and verifies prerequisites.
#
# Usage:
#   ./scripts/bootstrap-linux.sh
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

check_tool gcc "Install GCC: sudo apt install build-essential (Debian/Ubuntu) or sudo dnf install gcc (Fedora)"
echo "  GCC: $(gcc --version | head -1)"

check_tool node "Install Node.js: https://nodejs.org/"
echo "  Node.js: $(node --version)"

check_tool npm "Included with Node.js: https://nodejs.org/"
echo "  npm: $(npm --version)"

check_tool go "Install Go: https://go.dev/dl/"
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
# Cross-compilation (optional)
# ---------------------------------------------------------------------------

step "Checking cross-compilation toolchains (optional)"
HOST_ARCH="$(go env GOARCH)"
if [[ "$HOST_ARCH" == "amd64" ]]; then
    CROSS_CC="aarch64-linux-gnu-gcc"
    CROSS_PKG="gcc-aarch64-linux-gnu"
else
    CROSS_CC="x86_64-linux-gnu-gcc"
    CROSS_PKG="gcc-x86-64-linux-gnu"
fi

if command -v "$CROSS_CC" &>/dev/null; then
    echo "  $CROSS_CC: OK"
else
    echo "  $CROSS_CC: not found (install $CROSS_PKG to cross-compile)"
fi

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

step "Bootstrap complete"
echo "  Frontend deps: $FRONTEND_DIR/node_modules"
echo "  Go modules:    $(go env GOMODCACHE)"
echo ""
echo "  To build everything:  ./scripts/build-linux.sh"
echo "  To build both archs:  ./scripts/build-linux.sh --arch all"
echo "  To run the server:    cd local/server && go run ./cmd/buildermark"
echo "  To run the frontend:  cd local/frontend && npm run dev"
