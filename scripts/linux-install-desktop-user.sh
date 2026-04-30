#!/usr/bin/env bash
# 将当前目录下的 Linux 发行包注册到当前用户菜单，并安装 hicolor 图标，
# 这样启动器/任务栏会显示应用图标，而不是可执行文件的默认齿轮图标。
# 用法：在 build/linux-dist 目录下执行 ./install-desktop-user.sh
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_SRC="$ROOT/bin/leiAgent"
ICON256="$ROOT/share/icons/hicolor/256x256/apps/leiagent.png"
ICON48="$ROOT/share/icons/hicolor/48x48/apps/leiagent.png"

if [[ ! -f "$BIN_SRC" ]]; then
	echo "找不到 $BIN_SRC（请在 linux-dist 根目录运行本脚本）" >&2
	exit 1
fi
if [[ ! -f "$ICON256" ]]; then
	echo "找不到 $ICON256" >&2
	exit 1
fi

DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
BIN_DST="$HOME/.local/bin"
ICON_DST256="$DATA_HOME/icons/hicolor/256x256/apps"
ICON_DST48="$DATA_HOME/icons/hicolor/48x48/apps"
APP_DST="$DATA_HOME/applications"

mkdir -p "$BIN_DST" "$ICON_DST256" "$ICON_DST48" "$APP_DST"
cp "$BIN_SRC" "$BIN_DST/leiAgent"
chmod +x "$BIN_DST/leiAgent"
cp "$ICON256" "$ICON_DST256/leiagent.png"
if [[ -f "$ICON48" ]]; then
	cp "$ICON48" "$ICON_DST48/leiagent.png"
else
	cp "$ICON256" "$ICON_DST48/leiagent.png"
fi

DESKTOP="$APP_DST/leiagent.desktop"
cat >"$DESKTOP" <<EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=leiAgent
Comment=leiAgent
Exec=$BIN_DST/leiAgent
Icon=leiagent
Terminal=false
Categories=Utility;
EOF
chmod 644 "$DESKTOP"

if command -v gtk-update-icon-cache >/dev/null 2>&1; then
	gtk-update-icon-cache -f -t "$DATA_HOME/icons/hicolor" 2>/dev/null || true
fi
if command -v update-desktop-database >/dev/null 2>&1; then
	update-desktop-database "$APP_DST" 2>/dev/null || true
fi

echo "已安装: $BIN_DST/leiAgent"
echo "已注册菜单项: $DESKTOP（图标名 leiagent）"
echo "若启动器仍显示旧图标，请注销重新登录或重启会话。"
