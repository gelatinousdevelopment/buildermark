#!/bin/bash
# Copies shared files into each browser extension directory.
# Run this after modifying anything in shared/.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SHARED_DIR="$SCRIPT_DIR/shared"

for browser in chrome firefox safari; do
  DEST="$SCRIPT_DIR/$browser/shared"
  rm -rf "$DEST"
  cp -r "$SHARED_DIR" "$DEST"
  echo "Copied shared/ -> $browser/shared/"
done

echo "Done."
