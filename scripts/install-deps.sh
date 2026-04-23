#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"
mkdir -p "$GOCACHE"

echo "==> go mod download"
go mod download

echo "==> frontend dependencies"
cd "$ROOT/frontend"
npm install

echo "==> install-deps done"
