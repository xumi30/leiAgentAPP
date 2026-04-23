#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"
mkdir -p "$GOCACHE"
export CGO_LDFLAGS="${CGO_LDFLAGS:-} -framework UniformTypeIdentifiers -mmacosx-version-min=10.13"

"$ROOT/scripts/install-deps.sh"

echo "==> frontend build"
(cd "$ROOT/frontend" && npm run build)

echo "==> go build"
go build -buildvcs=false -trimpath -tags desktop,wv2runtime.download,production -ldflags "-w -s" -o "$ROOT/build/bin/leiAgent"

echo "==> stage config example"
mkdir -p "$ROOT/build/bin/config"
cp "$ROOT/config/config.example.yaml" "$ROOT/build/bin/config/config.example.yaml"

echo "==> package app bundle"
"$ROOT/scripts/package-app-macos.sh"

echo "==> Output under $ROOT/build/bin/"
