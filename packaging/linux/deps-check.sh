# Avisa, em português, quando falta o WebKitGTK 4.1 (usado para desenhar a janela).
check_webkit() {
  if command -v ldconfig >/dev/null 2>&1 && ldconfig -p 2>/dev/null | grep -q 'libwebkit2gtk-4.1.so.0'; then
    return 0
  fi
  if [ -e /usr/lib/libwebkit2gtk-4.1.so.0 ] || ls /usr/lib/*/libwebkit2gtk-4.1.so.0 >/dev/null 2>&1 || ls /usr/lib64/libwebkit2gtk-4.1.so.0 >/dev/null 2>&1; then
    return 0
  fi
  cat >&2 <<'MSG'
O NxATerm precisa do WebKitGTK 4.1 e ele não está instalado. Instale com:
  Debian / Ubuntu / Mint:  sudo apt install libwebkit2gtk-4.1-0
  Fedora / RHEL:           sudo dnf install webkit2gtk4.1
  openSUSE:                sudo zypper install libwebkit2gtk-4_1-0
  Arch / Manjaro:          sudo pacman -S webkit2gtk-4.1
MSG
  return 1
}
