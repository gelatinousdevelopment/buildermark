#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <artifact-path> [<artifact-path> ...]"
  exit 1
fi

for file in "$@"; do
  if [[ ! -f "$file" ]]; then
    echo "Missing file: $file" >&2
    exit 1
  fi
  shasum -a 256 "$file"
done
