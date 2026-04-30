#!/usr/bin/env bash
# Production Linux (ELF) build. Run on Ubuntu/Debian or another Linux with
# Wails/WebKit dev packages; CGO + WebKit2GTK must match your distro
# (see https://wails.io/docs/next/guides/linux ).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "Run this script on Linux (found: $(uname -s)). Cross-build from other OSes is not supported here." >&2
  exit 1
fi

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"
mkdir -p "$GOCACHE"

# Avoid accidentally reusing macOS linker flags from the environment.
if [[ -n "${CGO_LDFLAGS:-}" ]] && [[ "${CGO_LDFLAGS:-}" == *"-mmacosx-version-min"* || "${CGO_LDFLAGS:-}" == *"-framework"* ]]; then
  echo "CGO_LDFLAGS looks macOS-specific; unsetting for Linux build." >&2
  unset CGO_LDFLAGS
fi

USE_WEBKIT41=0
if [[ "${WEBKIT2_41:-}" == "1" || "${WEBKIT2_41:-}" == "true" ]]; then
  USE_WEBKIT41=1
fi
while [[ $# -gt 0 ]]; do
  case "$1" in
  --webkit2-41) USE_WEBKIT41=1 ;;
  -h | --help)
    echo "Usage: $0 [--webkit2-41]" >&2
    echo "  --webkit2-41   Use WebKit2GTK 4.1 (e.g. Ubuntu 24.04+). Or set WEBKIT2_41=1." >&2
    exit 0
    ;;
  *)
    echo "Unknown option: $1 (try --help)" >&2
    exit 1
    ;;
  esac
  shift
done

BUILD_TAGS="desktop,wv2runtime.download,production"
if [[ "$USE_WEBKIT41" -eq 1 ]]; then
  BUILD_TAGS="${BUILD_TAGS},webkit2_41"
  echo "==> Building with webkit2_41 (WebKit2GTK 4.1 pkg-config)"
else
  echo "==> Building with default WebKit2GTK 4.0; use --webkit2-41 on distros that only ship 4.1"
fi

"$ROOT/scripts/install-deps.sh"

echo "==> frontend build"
(cd "$ROOT/frontend" && npm run build)

echo "==> go build"
go build -buildvcs=false -trimpath -tags "$BUILD_TAGS" -ldflags "-w -s" -o "$ROOT/build/bin/leiAgent"

echo "==> stage config example"
mkdir -p "$ROOT/build/bin/config"
cp "$ROOT/config/config.example.yaml" "$ROOT/build/bin/config/config.example.yaml"

echo "==> Output: $ROOT/build/bin/leiAgent"
echo "    Ensure runtime deps are installed (GTK/WebKit2GTK) per Wails Linux docs."
