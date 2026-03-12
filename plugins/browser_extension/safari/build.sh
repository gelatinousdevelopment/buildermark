#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PROJECT_DIR="$ROOT_DIR/safari/BuildermarkSafari"

"$ROOT_DIR/build.sh" safari

xcodebuild \
    -project "$PROJECT_DIR/BuildermarkSafari.xcodeproj" \
    -scheme BuildermarkSafari \
    -configuration Debug \
    -derivedDataPath "$ROOT_DIR/safari/.derived-data" \
    build
