#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"
mkdir -p "$GOCACHE"

echo "==> Checking for committed-looking secrets"
if git grep -nE '(sk-[A-Za-z0-9]{20,}|lb_[A-Za-z0-9_-]{20,}|api_key:[[:space:]]*\"?(sk-|lb_)[A-Za-z0-9_-]{20,}|apiKey:[[:space:]]*\"?(sk-|lb_)[A-Za-z0-9_-]{20,}|Authorization:[[:space:]]*\"?Bearer[[:space:]]+[A-Za-z0-9._-]{20,})' -- \
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

echo "==> Building Wails app (generates bindings before frontend build)"
if ! command -v wails >/dev/null 2>&1; then
  echo "wails CLI is required for release checks. Install with: go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0" >&2
  exit 1
fi
if [[ -n "${WAILS_BUILD_TAGS:-}" ]]; then
  wails build -clean -tags "$WAILS_BUILD_TAGS"
else
  wails build -clean
fi

echo "Release checks passed."
