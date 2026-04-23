#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_PATH="${PROXY_LB_CONFIG:-$ROOT_DIR/config/config.yaml}"
EXAMPLE_PATH="$ROOT_DIR/config/config.example.yaml"
GO_BUILD_CACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"

if [[ ! -f "$CONFIG_PATH" ]]; then
  mkdir -p "$(dirname "$CONFIG_PATH")"
  cp "$EXAMPLE_PATH" "$CONFIG_PATH"
  echo "Created config at $CONFIG_PATH"
  echo "Fill in your model parameters and run again."
  exit 0
fi

cd "$ROOT_DIR"
mkdir -p "$GO_BUILD_CACHE"
echo "Sarted"
exec env PROXY_LB_CONFIG="$CONFIG_PATH" GOCACHE="$GO_BUILD_CACHE" go run ./cmd/proxy-lb
