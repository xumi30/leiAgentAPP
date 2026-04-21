#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"
mkdir -p "$GOCACHE"

echo "==> Checking for committed-looking secrets"
if git grep -nE '(sk-[A-Za-z0-9]{20,}|api_key:[[:space:]]*\"sk-[A-Za-z0-9]|apiKey:[[:space:]]*\"sk-[A-Za-z0-9])' -- \
  ':!package-lock.json' \
  ':!frontend/package-lock.json'; then
  echo "Potential secret found. Replace it with a placeholder before release." >&2
  exit 1
fi

echo "==> Checking Go formatting"
UNFORMATTED="$(gofmt -l $(find . -name '*.go' -not -path './build/bin/*' -not -path './frontend/node_modules/*' -not -path './node_modules/*'))"
if [[ -n "$UNFORMATTED" ]]; then
  echo "The following Go files are not gofmt-formatted yet:"
  echo "$UNFORMATTED"
  echo "Warning: run gofmt on the files above before a public release." >&2
fi

echo "==> Running Go tests"
go test ./...

echo "==> Building frontend"
(cd frontend && npm run build)

echo "Release checks passed."
