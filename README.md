# NxATerm

Gerenciador de sessões estilo MobaXterm: SSH, SFTP, FTP/FTPS, Telnet, Serial, shell local e WSL (no Windows), RDP, VNC, Spice e Web, com MultiExec, túneis, macros, sincronização e assistente de IA. Feito em Go + Wails v3 + xterm.js, por Alexandre Garcia (alekhine-dev).

## Baixar

Na página de **Releases** estão os instaladores e pacotes de cada sistema.

**Windows 10/11** (x64 e ARM64): execute `NxATerm-<versão>-windows-<arq>-setup.exe`. Instala só para o seu usuário, sem pedir administrador. Também há uma versão portátil.

**Linux** (x86_64 e aarch64; precisa do WebKitGTK 4.1: Ubuntu 22.04+, Debian 12+, Fedora, openSUSE, Arch):

- Debian / Ubuntu / Mint: `sudo apt install ./nxaterm_<versão>_amd64.deb`
- Fedora / openSUSE / RHEL: `sudo dnf install ./nxaterm-<versão>-1.x86_64.rpm`
- Arch / Manjaro: `sudo pacman -U nxaterm-<versão>-1-x86_64.pkg.tar.zst`
- Qualquer distro: `chmod +x NxATerm-*.AppImage && ./NxATerm-*.AppImage`, ou extraia o `.tar.gz` e rode `./install.sh`
- Portas seriais: o usuário precisa estar no grupo `dialout` (ou `uucp` no Arch).

**macOS** (Apple Silicon e Intel): baixe o `.zip` da sua arquitetura, descompacte e arraste o NxATerm para Aplicativos. O app não é assinado nem notarizado pela Apple: na primeira vez, clique com o botão direito e escolha Abrir, ou execute `xattr -cr /Applications/NxATerm.app`.

Os instaladores não são assinados digitalmente: o Windows pode avisar "editor desconhecido". Confira os arquivos com o `SHA256SUMS` da release.

## Recursos

**Conexão**
- SSH com senha, chave privada e agente SSH; proxy, jump host, X11, codificação de caracteres, variáveis, diretório inicial e links `ssh://`
- Telnet, Serial, shell local, WSL, FTP/FTPS e SFTP
- RDP e Spice pelo cliente do sistema; VNC dentro do app; sessões Web em janela própria
- Importação de `~/.ssh/config`
- Senhas SSH guardadas só em memória

**Terminal**
- Abas e painéis divididos, busca, links clicáveis, Unicode 11, aceleração WebGL e imagens inline (sixel)
- Zmodem (`rz`/`sz`) e Trzsz (`trz`/`tsz`) pelo terminal, com qualquer codificação e também no Telnet
- Ligaduras de fonte (Cascadia Code, Fira Code, JetBrains Mono e similares, quando instaladas)
- Temas do terminal e da interface, imagem de fundo, transparência, CSS próprio, realce de palavras e importação de temas do iTerm2 e do Windows Terminal

**Arquivos**
- Navegador de arquivos com painel local, seleção múltipla, arrastar e soltar, permissões e propriedades
- Fila e gaveta de transferências, SFTP recursivo, copiar/mover e edição remota

**Automação e organização**
- MultiExec e execução de comandos em lote em vários servidores
- Túneis SSH, macros e comandos rápidos (fixar, ordenar, vincular a sessões, importar e exportar)
- Cor, tags e descrição nas sessões, histórico e espaços de trabalho
- Senha de abertura do app: além de travar a interface, criptografa os dados guardados (AES-256-GCM); perder a senha significa perder esses dados
- Bandeja do sistema e atalho global (Windows, Linux em X11 e macOS; no Wayland use um atalho do sistema que execute `nxaterm`)

**Sincronização**
- Sessões, macros e configurações via GitHub Gist, WebDAV ou servidor próprio, com criptografia opcional por senha
- Exclusões também são propagadas entre computadores

**Assistente de IA**
- Chat com qualquer API compatível com OpenAI, com modo agente que pede aprovação a cada comando
- Servidor MCP local protegido por token

**Interface**
- Português, inglês e espanhol, com troca ao vivo em Configurações

## Créditos

- [xterm.js](https://github.com/xtermjs/xterm.js) (MIT)
- [zmodem.js](https://github.com/FGasper/zmodem.js) (Apache-2.0)
- [trzsz.js](https://github.com/trzsz/trzsz.js) (MIT)
- [noVNC](https://github.com/novnc/noVNC) (MPL-2.0)
- [Wails](https://github.com/wailsapp/wails) (MIT)
- O projeto [electerm](https://github.com/electerm/electerm) (MIT) serviu de referência para a lista de funcionalidades. Todo o código do NxATerm foi escrito do zero.
