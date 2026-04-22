#!/usr/bin/env bash
set -euo pipefail

# Copies the built macOS .app into /Applications (requires write permission).

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_SRC="$ROOT/build/bin/leiAgent.app"
DEST="/Applications/leiAgent.app"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script is for macOS only." >&2
  exit 1
fi

if [[ ! -d "$APP_SRC" ]]; then
  echo "Missing $APP_SRC — run scripts/build-release.sh (or wails build) first." >&2
  exit 1
fi

echo "==> Installing $APP_SRC -> $DEST"
rm -rf "$DEST"
cp -R "$APP_SRC" "$DEST"
echo "==> Done. Open leiAgent from Applications."
