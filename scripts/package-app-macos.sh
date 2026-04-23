#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$ROOT/build/bin"
APP_NAME="$(/usr/bin/plutil -extract name raw -o - "$ROOT/wails.json" 2>/dev/null || echo "leiAgent")"
OUTPUT_NAME="$(/usr/bin/plutil -extract outputfilename raw -o - "$ROOT/wails.json" 2>/dev/null || echo "$APP_NAME")"
APP_BUNDLE="$BIN_DIR/$APP_NAME.app"
APP_CONTENTS="$APP_BUNDLE/Contents"
APP_MACOS="$APP_CONTENTS/MacOS"
APP_RESOURCES="$APP_CONTENTS/Resources"
APP_BINARY="$BIN_DIR/$OUTPUT_NAME"
APP_BINARY_IN_BUNDLE="$APP_MACOS/$OUTPUT_NAME"
ICON_SOURCE="$ROOT/build/appicon.png"
ICON_DEST="$APP_RESOURCES/iconfile.icns"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script is for macOS only." >&2
  exit 1
fi

if [[ ! -f "$APP_BINARY" ]]; then
  echo "Missing compiled binary: $APP_BINARY" >&2
  exit 1
fi

rm -rf "$APP_BUNDLE"
mkdir -p "$APP_MACOS" "$APP_RESOURCES"

cp "$APP_BINARY" "$APP_BINARY_IN_BUNDLE"
chmod +x "$APP_BINARY_IN_BUNDLE"

# Ship the example config alongside the executable so runtime can show it in the UI
# even when the working directory is not the repository root.
if [[ -f "$ROOT/config/config.example.yaml" ]]; then
  mkdir -p "$APP_MACOS/config"
  cp "$ROOT/config/config.example.yaml" "$APP_MACOS/config/config.example.yaml"
fi

cat >"$APP_CONTENTS/Info.plist" <<PLIST
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>CFBundleExecutable</key>
    <string>$OUTPUT_NAME</string>
    <key>CFBundleGetInfoString</key>
    <string>Built using Wails (https://wails.io)</string>
    <key>CFBundleIconFile</key>
    <string>iconfile</string>
    <key>CFBundleIdentifier</key>
    <string>com.wails.$APP_NAME</string>
    <key>CFBundleName</key>
    <string>$APP_NAME</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0.0</string>
    <key>CFBundleVersion</key>
    <string>1.0.0</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.13.0</string>
    <key>NSHighResolutionCapable</key>
    <string>true</string>
    <key>NSHumanReadableCopyright</key>
    <string>Copyright.........</string>
  </dict>
</plist>
PLIST

if [[ -f "$ICON_SOURCE" ]]; then
  ICONSET_WORK="$(mktemp -d)"
  cleanup() {
    rm -rf "$ICONSET_WORK"
  }
  trap cleanup EXIT

  NORMALIZED_SOURCE="$ICONSET_WORK/source-1024.png"
  SQUARE_SOURCE="$ICONSET_WORK/source-square.png"
  /usr/bin/sips -Z 1024 "$ICON_SOURCE" --out "$NORMALIZED_SOURCE" >/dev/null
  /usr/bin/sips -p 1024 1024 "$NORMALIZED_SOURCE" --padColor FFFFFF --out "$SQUARE_SOURCE" >/dev/null
  if ! python3 - <<'PY' "$SQUARE_SOURCE" "$ICON_DEST"
import struct
import sys

png_path, out_path = sys.argv[1], sys.argv[2]
with open(png_path, "rb") as f:
    png = f.read()

block = b"ic10" + struct.pack(">I", len(png) + 8) + png
with open(out_path, "wb") as f:
    f.write(b"icns")
    f.write(struct.pack(">I", len(block) + 8))
    f.write(block)
PY
  then
    echo "Warning: failed to generate macOS .icns from $ICON_SOURCE; the app will use the default icon." >&2
  fi
  trap - EXIT
fi

/usr/bin/xattr -rc "$APP_BUNDLE" 2>/dev/null || true
/usr/bin/codesign --force --deep --sign - "$APP_BUNDLE"

echo "==> Packaged $APP_BUNDLE"
