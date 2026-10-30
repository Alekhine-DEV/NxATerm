# NxATerm

Gerenciador de sessões SSH para desktop, em **Go + Wails v3 + xterm.js**, com visual inspirado em gerenciadores
clássicos de SSH: barra de menus, toolbar de ícones, abas laterais, navegador SFTP à esquerda e barra de status.

## Recursos

- Sessões ilimitadas, com pastas, busca e conexão rápida (`usuario@host:porta`); importação do `~/.ssh/config`
- Abas de terminal (xterm.js) com busca, links clicáveis, copiar ao selecionar, zoom com Ctrl+roda
- **MultiExec**: envia um comando a várias abas de uma vez, ou "ao vivo" (o que você digita vai para todas)
- **Divisão de tela**: uma janela, 2 colunas, 2 linhas ou grade 2×2
- **Navegador SFTP** lateral: enviar, baixar, renomear, excluir, nova pasta; "seguir pasta do terminal" (bash)
- **Túneis SSH**: local, remoto e dinâmico (proxy SOCKS5), sempre só em 127.0.0.1
- **Macros**: comandos salvos, executados na aba ativa ou em todas
- **Monitoramento remoto** na barra de status: CPU, memória, rede, uptime, usuário e disco (servidores Linux)
- **Gerador de chaves** Ed25519 (formato OpenSSH)
- Temas de cores, fonte, cursor, botão direito (colar ou menu)

## Segurança

- Senhas **nunca** vão para o disco (ficam só na memória da aba, para reconectar/duplicar).
- Dados em `%AppData%\NxATerm` (Windows) ou `~/.config/NxATerm` (Linux): `sessions.json`, `macros.json`, `settings.json`, `known_hosts`.
- Chave do servidor estilo `accept-new`: host novo é gravado; chave alterada é **recusada**.
- Túneis escutam somente em 127.0.0.1 e terminam junto com a aba.

## Estrutura

```
main.go             janela e registro do serviço (Wails v3)
termservice.go      serviço: sessões, ssh_config, seletor de chave
ssh.go              conexão SSH, terminal (PTY), known_hosts, keepalive, eventos
sftp.go             operações SFTP e diálogos de arquivo
monitor.go          estatísticas do servidor (barra de status)
tunnels.go          túneis SSH (local, remoto, SOCKS5)
extras.go           macros, configurações, gerador de chaves, sair
internal/store      sessões, macros e parser do ssh_config (com testes)
frontend/           index.html e src/{main,dialogs,settings,util,icons,api}.js + style.css
```

Testes: `go test ./internal/...` (e `go test .` no pacote principal, depois de gerar o projeto).
