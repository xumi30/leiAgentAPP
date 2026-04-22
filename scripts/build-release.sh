#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"
mkdir -p "$GOCACHE"

"$ROOT/scripts/install-deps.sh"

echo "==> wails build (production: strip via Wails; -trimpath removes host paths from binary)"
# Extra args (e.g. -platform windows/amd64) can be passed through.
wails build -trimpath "$@"

echo "==> Output under $ROOT/build/bin/"
