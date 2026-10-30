# NxATerm

Gerenciador de sessões estilo MobaXterm: SSH, SFTP, Telnet, Serial, shell local e WSL (no Windows), com MultiExec, túneis e macros. Feito em Go + Wails v3 + xterm.js, por Alexandre Garcia (alekhine-dev).

## Baixar

Na página de **Releases** estão o instalador do Windows e os pacotes do Linux.

**Windows 10/11**: execute `NxATerm-<versão>-windows-setup.exe`. Instala só para o seu usuário, sem pedir administrador. Também há uma versão portátil.

**Linux** (precisa do WebKitGTK 4.1: Ubuntu 22.04+, Debian 12+, Fedora, openSUSE, Arch):

- Debian / Ubuntu / Mint: `sudo apt install ./nxaterm_<versão>_amd64.deb`
- Fedora / openSUSE / RHEL: `sudo dnf install ./nxaterm-<versão>-1.x86_64.rpm`
- Arch / Manjaro: `sudo pacman -U nxaterm-<versão>-1-x86_64.pkg.tar.zst`
- Qualquer distro: `chmod +x NxATerm-*.AppImage && ./NxATerm-*.AppImage`, ou extraia o `.tar.gz` e rode `./install.sh`
- Portas seriais: o usuário precisa estar no grupo `dialout` (ou `uucp` no Arch).

Os instaladores não são assinados digitalmente: o Windows pode avisar "editor desconhecido". Confira os arquivos com o `SHA256SUMS` da release.
