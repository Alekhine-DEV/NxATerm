#!/bin/sh
# Instala o NxATerm para o usuário atual (~/.local) ou, com PREFIX, para todos:
#   ./install.sh                      instala em ~/.local
#   sudo PREFIX=/usr/local ./install.sh
#   ./install.sh --remover            desinstala (use o mesmo PREFIX)
set -e
DIR="$(cd "$(dirname "$0")" && pwd)"
PREFIX="${PREFIX:-$HOME/.local}"
BIN="$PREFIX/bin/nxaterm"
APP="$PREFIX/share/applications/nxaterm.desktop"
ICON="$PREFIX/share/icons/hicolor/512x512/apps/nxaterm.png"

if [ "$1" = "--remover" ]; then
  rm -f "$BIN" "$APP" "$ICON"
  echo "NxATerm removido de $PREFIX"
  exit 0
fi

. "$DIR/deps-check.sh"
check_webkit || echo "(a instalação continua; instale o WebKitGTK antes de abrir o NxATerm)" >&2

install -Dm755 "$DIR/nxaterm" "$BIN"
install -Dm644 "$DIR/nxaterm.png" "$ICON"
mkdir -p "$(dirname "$APP")"
sed "s|^Exec=.*|Exec=$BIN %U|" "$DIR/nxaterm.desktop" > "$APP"
chmod 644 "$APP"
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q "$PREFIX/share/applications" 2>/dev/null || true
command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q -t -f "$PREFIX/share/icons/hicolor" 2>/dev/null || true
echo "NxATerm instalado em $BIN"
case ":$PATH:" in *":$PREFIX/bin:"*) ;; *) echo "Adicione $PREFIX/bin ao PATH para abrir pelo terminal com: nxaterm" ;; esac
