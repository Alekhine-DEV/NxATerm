// Dicionário (parte 5): mensagens de erro e textos vindos do backend (Go). português ¦ inglês ¦ espanhol.
// {0}, {1}... marcam os trechos variáveis (nomes, números, erros internos).
export const DICT_E = `
o ssh-agent não está em execução (SSH_AUTH_SOCK vazio)¦the ssh-agent is not running (SSH_AUTH_SOCK is empty)¦el ssh-agent no está en ejecución (SSH_AUTH_SOCK vacío)
arquivo grande demais¦file too large¦archivo demasiado grande
a imagem passa de 5 MB¦the image is larger than 5 MB¦la imagen supera los 5 MB
formato de imagem não suportado (use PNG, JPG, GIF, WebP, BMP ou SVG)¦unsupported image format (use PNG, JPG, GIF, WebP, BMP or SVG)¦formato de imagen no compatible (use PNG, JPG, GIF, WebP, BMP o SVG)
comando vazio¦empty command¦comando vacío
programa não encontrado: {0}¦program not found: {0}¦programa no encontrado: {0}
não foi possível criar o pseudo-terminal: {0}¦could not create the pseudo-terminal: {0}¦no se pudo crear el pseudoterminal: {0}
não foi possível iniciar {0}: {1}¦could not start {0}: {1}¦no se pudo iniciar {0}: {1}
a pasta inicial não existe: {0}¦the starting folder does not exist: {0}¦la carpeta inicial no existe: {0}
o WSL só existe no Windows¦WSL only exists on Windows¦WSL solo existe en Windows
conexão cancelada¦connection canceled¦conexión cancelada
tempo esgotado esperando a resposta¦timed out waiting for the answer¦tiempo agotado esperando la respuesta
cadeia de servidores de salto longa demais¦jump host chain is too long¦cadena de servidores de salto demasiado larga
o servidor de salto escolhido não existe mais¦the chosen jump host no longer exists¦el servidor de salto elegido ya no existe
o servidor de salto precisa ser uma sessão SSH¦the jump host must be an SSH session¦el servidor de salto debe ser una sesión SSH
servidor de salto "{0}": {1}¦jump host "{0}": {1}¦servidor de salto "{0}": {1}
o servidor de salto não chegou em {0}: {1}¦the jump host could not reach {0}: {1}¦el servidor de salto no llegó a {0}: {1}
não consegui ler a chave {0}: {1}¦could not read the key {0}: {1}¦no pude leer la clave {0}: {1}
chave inválida ou senha da chave incorreta: {0}¦invalid key or wrong key passphrase: {0}¦clave no válida o contraseña de la clave incorrecta: {0}
falha ao conectar em {0}: {1}¦failed to connect to {0}: {1}¦error al conectar con {0}: {1}
codificação não suportada: "{0}"¦unsupported encoding: "{0}"¦codificación no compatible: "{0}"
não foi possível abrir {0}: {1}¦could not open {0}: {1}¦no se pudo abrir {0}: {1}
selecione um arquivo¦select a file¦seleccione un archivo
arquivo grande demais para editar (limite de 50 MB)¦file too large to edit (50 MB limit)¦archivo demasiado grande para editar (límite de 50 MB)
configurações grandes demais¦settings too large¦configuración demasiado grande
configurações inválidas¦invalid settings¦configuración no válida
{0} já existe; escolha outro nome para não sobrescrever uma chave¦{0} already exists; choose another name so a key is not overwritten¦{0} ya existe; elija otro nombre para no sobrescribir una clave
conexão encerrada¦connection closed¦conexión cerrada
login recusado: {0}¦login refused: {0}¦inicio de sesión rechazado: {0}
conexão FTP perdida: {0}¦FTP connection lost: {0}¦conexión FTP perdida: {0}
o atalho global só está disponível no Windows¦the global shortcut is only available on Windows¦el atajo global solo está disponible en Windows
o Windows recusou o atalho "{0}" (já está em uso?): {1}¦Windows refused the shortcut "{0}" (is it already in use?): {1}¦Windows rechazó el atajo "{0}" (¿ya está en uso?): {1}
endereço da API inválido (use http:// ou https://)¦invalid API address (use http:// or https://)¦dirección de la API no válida (use http:// o https://)
não foi possível falar com a API: {0}¦could not reach the API: {0}¦no se pudo comunicar con la API: {0}
resposta inesperada da API¦unexpected API response¦respuesta inesperada de la API
escolha o modelo¦choose the model¦elija el modelo
conversa vazia¦empty conversation¦conversación vacía
a API não devolveu resposta¦the API returned no answer¦la API no devolvió respuesta
arquivo da senha de abertura corrompido¦the startup password file is corrupted¦el archivo de la contraseña de inicio está dañado
senha incorreta¦incorrect password¦contraseña incorrecta
sem senha de abertura¦no startup password¦sin contraseña de inicio
senha atual incorreta¦current password is incorrect¦la contraseña actual es incorrecta
use pelo menos 4 caracteres¦use at least 4 characters¦use al menos 4 caracteres
atalho vazio¦empty shortcut¦atajo vacío
atalho inválido: "{0}"¦invalid shortcut: "{0}"¦atajo no válido: "{0}"
mais de uma tecla em "{0}"¦more than one key in "{0}"¦más de una tecla en "{0}"
falta a tecla em "{0}"¦missing key in "{0}"¦falta la tecla en "{0}"
use Ctrl, Alt ou Win junto com a tecla¦use Ctrl, Alt or Win together with the key¦use Ctrl, Alt o Win junto con la tecla
tecla desconhecida: "{0}"¦unknown key: "{0}"¦tecla desconocida: "{0}"
link inválido: {0}¦invalid link: {0}¦enlace no válido: {0}
tipo de link não suportado: "{0}"¦unsupported link type: "{0}"¦tipo de enlace no compatible: "{0}"
o link não tem host: "{0}"¦the link has no host: "{0}"¦el enlace no tiene host: "{0}"
porta inválida no link: "{0}"¦invalid port in the link: "{0}"¦puerto no válido en el enlace: "{0}"
o link precisa do usuário ({0}://usuario@host)¦the link needs a user ({0}://user@host)¦el enlace necesita el usuario ({0}://usuario@host)
porta inválida (use de 1024 a 65535)¦invalid port (use 1024 to 65535)¦puerto no válido (use de 1024 a 65535)
token curto demais¦token too short¦token demasiado corto
não foi possível abrir a porta {0}: {1}¦could not open port {0}: {1}¦no se pudo abrir el puerto {0}: {1}
endereço de proxy inválido: {0}¦invalid proxy address: {0}¦dirección de proxy no válida: {0}
tipo de proxy não suportado: "{0}" (use socks5 ou http)¦unsupported proxy type: "{0}" (use socks5 or http)¦tipo de proxy no compatible: "{0}" (use socks5 o http)
proxy sem endereço¦proxy has no address¦proxy sin dirección
porta de proxy inválida: "{0}"¦invalid proxy port: "{0}"¦puerto de proxy no válido: "{0}"
não consegui falar com o proxy {0}: {1}¦could not talk to the proxy {0}: {1}¦no pude comunicarme con el proxy {0}: {1}
proxy SOCKS5 não respondeu: {0}¦the SOCKS5 proxy did not answer: {0}¦el proxy SOCKS5 no respondió: {0}
o endereço não é de um proxy SOCKS5¦the address is not a SOCKS5 proxy¦la dirección no es de un proxy SOCKS5
usuário ou senha do proxy longos demais¦proxy user or password too long¦usuario o contraseña del proxy demasiado largos
o proxy recusou o usuário ou a senha¦the proxy refused the user or password¦el proxy rechazó el usuario o la contraseña
o proxy SOCKS5 exige uma autenticação que o NxATerm não suporta¦the SOCKS5 proxy requires an authentication that NxATerm does not support¦el proxy SOCKS5 exige una autenticación que NxATerm no admite
nome de host longo demais¦host name too long¦nombre de host demasiado largo
o proxy não conseguiu chegar em {0} ({1})¦the proxy could not reach {0} ({1})¦el proxy no pudo llegar a {0} ({1})
resposta inválida do proxy SOCKS5¦invalid SOCKS5 proxy response¦respuesta no válida del proxy SOCKS5
proxy HTTP não respondeu: {0}¦the HTTP proxy did not answer: {0}¦el proxy HTTP no respondió: {0}
o endereço não é de um proxy HTTP¦the address is not an HTTP proxy¦la dirección no es de un proxy HTTP
o proxy HTTP recusou o usuário ou a senha¦the HTTP proxy refused the user or password¦el proxy HTTP rechazó el usuario o la contraseña
o proxy HTTP recusou a conexão: {0}¦the HTTP proxy refused the connection: {0}¦el proxy HTTP rechazó la conexión: {0}
o proxy HTTP enviou dados antes da hora¦the HTTP proxy sent data too early¦el proxy HTTP envió datos antes de tiempo
ProxyCommand vazio¦empty ProxyCommand¦ProxyCommand vacío
não consegui executar o ProxyCommand: {0}¦could not run the ProxyCommand: {0}¦no pude ejecutar el ProxyCommand: {0}
DISPLAY inválido: "{0}"¦invalid DISPLAY: "{0}"¦DISPLAY no válido: "{0}"
pacote X11 inválido¦invalid X11 packet¦paquete X11 no válido
cookie X11 recusado¦X11 cookie refused¦cookie X11 rechazada
dados protegidos: digite a senha de abertura¦protected data: enter the startup password¦datos protegidos: escriba la contraseña de inicio
arquivo não está criptografado¦the file is not encrypted¦el archivo no está cifrado
arquivo criptografado corrompido¦encrypted file corrupted¦archivo cifrado dañado
não foi possível abrir o arquivo (senha errada ou arquivo corrompido)¦could not open the file (wrong password or corrupted file)¦no se pudo abrir el archivo (contraseña incorrecta o archivo dañado)
chave inválida¦invalid key¦clave no válida
arquivo de pastas corrompido ({0}): {1}¦folders file corrupted ({0}): {1}¦archivo de carpetas dañado ({0}): {1}
informe o nome da pasta¦enter the folder name¦indique el nombre de la carpeta
não dá para mover uma pasta para dentro dela mesma¦a folder cannot be moved into itself¦no se puede mover una carpeta dentro de sí misma
informe a pasta¦enter the folder¦indique la carpeta
arquivo inválido: {0}¦invalid file: {0}¦archivo no válido: {0}
arquivo de macros corrompido ({0}): {1}¦macros file corrupted ({0}): {1}¦archivo de macros dañado ({0}): {1}
dê um nome à macro¦give the macro a name¦ponga un nombre a la macro
a macro precisa de um comando¦the macro needs a command¦la macro necesita un comando
macro não encontrada¦macro not found¦macro no encontrada
arquivo de sessões corrompido ({0}): {1}¦sessions file corrupted ({0}): {1}¦archivo de sesiones dañado ({0}): {1}
método de autenticação inválido: "{0}"¦invalid authentication method: "{0}"¦método de autenticación no válido: "{0}"
informe o caminho da chave privada¦enter the path of the private key¦indique la ruta de la clave privada
o servidor de salto não pode ser a própria sessão¦the jump host cannot be the session itself¦el servidor de salto no puede ser la propia sesión
segurança de FTP inválida: "{0}"¦invalid FTP security: "{0}"¦seguridad FTP no válida: "{0}"
escolha a porta serial (ex: COM3)¦choose the serial port (e.g. COM3)¦elija el puerto serie (p. ej.: COM3)
velocidade (baud rate) inválida: {0}¦invalid speed (baud rate): {0}¦velocidad (baud rate) no válida: {0}
bits de dados inválido: {0} (use 5 a 8)¦invalid data bits: {0} (use 5 to 8)¦bits de datos no válidos: {0} (use de 5 a 8)
paridade inválida: "{0}"¦invalid parity: "{0}"¦paridad no válida: "{0}"
bits de parada inválido: "{0}"¦invalid stop bits: "{0}"¦bits de parada no válidos: "{0}"
tipo de sessão inválido: "{0}"¦invalid session type: "{0}"¦tipo de sesión no válido: "{0}"
tipo de terminal inválido: "{0}"¦invalid terminal type: "{0}"¦tipo de terminal no válido: "{0}"
variável inválida: "{0}" (use NOME=valor)¦invalid variable: "{0}" (use NAME=value)¦variable no válida: "{0}" (use NOMBRE=valor)
o host é obrigatório¦the host is required¦el host es obligatorio
o usuário é obrigatório¦the user is required¦el usuario es obligatorio
porta inválida: {0}¦invalid port: {0}¦puerto no válido: {0}
sessão "{0}" não encontrada¦session "{0}" not found¦sesión "{0}" no encontrada
informe o endereço da página¦enter the page address¦indique la dirección de la página
endereço inválido¦invalid address¦dirección no válida
use um endereço http:// ou https://¦use an http:// or https:// address¦use una dirección http:// o https://
não coloque usuário e senha no endereço¦do not put a user and password in the address¦no ponga usuario y contraseña en la dirección
ainda não há cópia no destino¦there is no copy at the destination yet¦aún no hay copia en el destino
a cópia está criptografada: informe a senha da criptografia¦the copy is encrypted: enter the encryption password¦la copia está cifrada: indique la contraseña de cifrado
criptografia da cópia não reconhecida¦unrecognized copy encryption¦cifrado de la copia no reconocido
cópia corrompida¦corrupted copy¦copia dañada
senha da criptografia incorreta¦incorrect encryption password¦contraseña de cifrado incorrecta
destino desconhecido: "{0}"¦unknown destination: "{0}"¦destino desconocido: "{0}"
o arquivo no destino não é uma cópia do NxATerm¦the file at the destination is not an NxATerm copy¦el archivo del destino no es una copia de NxATerm
falha de conexão: {0}¦connection failure: {0}¦fallo de conexión: {0}
resposta grande demais¦response too large¦respuesta demasiado grande
acesso negado ({0}): confira o token/usuário e a senha. {1}¦access denied ({0}): check the token/user and password. {1}¦acceso denegado ({0}): compruebe el token/usuario y la contraseña. {1}
o servidor respondeu {0} {1}¦the server answered {0} {1}¦el servidor respondió {0} {1}
informe o token do GitHub (com permissão de gist)¦enter the GitHub token (with gist permission)¦indique el token de GitHub (con permiso de gist)
resposta inesperada do GitHub¦unexpected GitHub response¦respuesta inesperada de GitHub
endereço do arquivo inesperado¦unexpected file address¦dirección del archivo inesperada
endereço inválido (use http:// ou https://)¦invalid address (use http:// or https://)¦dirección no válida (use http:// o https://)
registro de exclusões corrompido¦deletion log corrupted¦registro de eliminaciones dañado
registro de exclusões da cópia inválido¦invalid deletion log in the copy¦registro de eliminaciones de la copia no válido
quadro sem máscara¦frame without mask¦trama sin máscara
quadro grande demais¦frame too large¦trama demasiado grande
transferência cancelada¦transfer canceled¦transferencia cancelada
{0} item(ns) com erro; o primeiro: {1}¦{0} item(s) with errors; the first: {1}¦{0} elemento(s) con error; el primero: {1}
pastas aninhadas demais em {0}¦folders nested too deep in {0}¦carpetas anidadas en exceso en {0}
{0}: já existe um arquivo com o nome da pasta¦{0}: a file with the folder's name already exists¦{0}: ya existe un archivo con el nombre de la carpeta
já existe uma pasta com o nome do arquivo¦a folder with the file's name already exists¦ya existe una carpeta con el nombre del archivo
ação desconhecida: "{0}"¦unknown action: "{0}"¦acción desconocida: "{0}"
não foi possível achar um nome livre¦could not find a free name¦no se pudo encontrar un nombre libre
{0} já existe e não é uma pasta¦{0} already exists and is not a folder¦{0} ya existe y no es una carpeta
não existe a sessão "{0}"¦there is no session "{0}"¦no existe la sesión "{0}"
não consegui descobrir o caminho do programa¦could not find the program path¦no pude averiguar la ruta del programa
já existe um item com esse nome¦an item with that name already exists¦ya existe un elemento con ese nombre
não é possível apagar a raiz¦the root cannot be deleted¦no se puede eliminar la raíz
caminho vazio¦empty path¦ruta vacía
sem fonte de números aleatórios¦no source of random numbers¦sin fuente de números aleatorios
o monitoramento só funciona em sessões SSH¦monitoring only works on SSH sessions¦la monitorización solo funciona en sesiones SSH
o servidor demorou para responder¦the server took too long to answer¦el servidor tardó en responder
só existe no Windows¦only exists on Windows¦solo existe en Windows
o comando xdg-mime não foi encontrado¦the xdg-mime command was not found¦no se encontró el comando xdg-mime
tipo de sessão sem suporte aqui: "{0}"¦session type not supported here: "{0}"¦tipo de sesión no admitido aquí: "{0}"
a sessão não é VNC¦the session is not VNC¦la sesión no es VNC
o aplicativo não está pronto¦the application is not ready¦la aplicación no está lista
não foi possível abrir o mstsc: {0}¦could not open mstsc: {0}¦no se pudo abrir mstsc: {0}
nenhum cliente RDP encontrado: instale o FreeRDP (xfreerdp)¦no RDP client found: install FreeRDP (xfreerdp)¦no se encontró ningún cliente RDP: instale FreeRDP (xfreerdp)
nenhum cliente Spice encontrado: instale o virt-viewer (remote-viewer)¦no Spice client found: install virt-viewer (remote-viewer)¦no se encontró ningún cliente Spice: instale virt-viewer (remote-viewer)
arquivo grande demais para ser uma lista de sessões¦file too large to be a session list¦archivo demasiado grande para ser una lista de sesiones
não reconheci o formato do arquivo (esperava JSON do NxATerm, MobaXterm ou ~/.ssh/config)¦unrecognized file format (expected NxATerm JSON, MobaXterm or ~/.ssh/config)¦no reconocí el formato del archivo (esperaba JSON de NxATerm, MobaXterm o ~/.ssh/config)
a importação do PuTTY só funciona no Windows¦PuTTY import only works on Windows¦la importación de PuTTY solo funciona en Windows
não encontrei sessões do PuTTY neste usuário¦no PuTTY sessions found for this user¦no encontré sesiones de PuTTY en este usuario
o PuTTY não tem sessões SSH ou Telnet salvas¦PuTTY has no saved SSH or Telnet sessions¦PuTTY no tiene sesiones SSH ni Telnet guardadas
já existe "{0}" na pasta de destino¦"{0}" already exists in the destination folder¦ya existe "{0}" en la carpeta de destino
permissão inválida¦invalid permission¦permiso no válido
esta operação só existe em conexões SSH/SFTP¦this operation only exists on SSH/SFTP connections¦esta operación solo existe en conexiones SSH/SFTP
não foi possível abrir o SFTP: {0}¦could not open SFTP: {0}¦no se pudo abrir SFTP: {0}
A CHAVE DO HOST {0} MUDOU (possível ataque man-in-the-middle). Conexão recusada. Se a mudança é legítima, remova a linha correspondente de {1}¦THE KEY OF HOST {0} HAS CHANGED (possible man-in-the-middle attack). Connection refused. If the change is legitimate, remove the matching line from {1}¦LA CLAVE DEL HOST {0} HA CAMBIADO (posible ataque man-in-the-middle). Conexión rechazada. Si el cambio es legítimo, elimine la línea correspondiente de {1}
ID de aba vazio¦empty tab ID¦ID de pestaña vacío
já existe uma aba com o ID "{0}"¦a tab with ID "{0}" already exists¦ya existe una pestaña con el ID "{0}"
tipo de sessão sem suporte: "{0}"¦unsupported session type: "{0}"¦tipo de sesión no admitido: "{0}"
esta aba não tem terminal¦this tab has no terminal¦esta pestaña no tiene terminal
terminal "{0}" não está conectado¦terminal "{0}" is not connected¦el terminal "{0}" no está conectado
modo de sincronização inválido¦invalid sync mode¦modo de sincronización no válido
sessões da cópia inválidas¦invalid sessions in the copy¦sesiones de la copia no válidas
macros da cópia inválidas¦invalid macros in the copy¦macros de la copia no válidas
pastas da cópia inválidas¦invalid folders in the copy¦carpetas de la copia no válidas
codificação desconhecida: "{0}"¦unknown encoding: "{0}"¦codificación desconocida: "{0}"
transferência não encontrada¦transfer not found¦transferencia no encontrada
ação inválida: "{0}"¦invalid action: "{0}"¦acción no válida: "{0}"
cancele a transferência antes de remover¦cancel the transfer before removing it¦cancele la transferencia antes de quitarla
a pasta de destino não existe¦the destination folder does not exist¦la carpeta de destino no existe
túneis só funcionam em sessões SSH e SFTP¦tunnels only work on SSH and SFTP sessions¦los túneles solo funcionan en sesiones SSH y SFTP
porta de escuta inválida (1-65535)¦invalid listening port (1-65535)¦puerto de escucha no válido (1-65535)
informe o host de destino¦enter the destination host¦indique el host de destino
porta de destino inválida (1-65535)¦invalid destination port (1-65535)¦puerto de destino no válido (1-65535)
tipo de túnel desconhecido¦unknown tunnel type¦tipo de túnel desconocido
não foi possível abrir o túnel: {0}¦could not open the tunnel: {0}¦no se pudo abrir el túnel: {0}
túnel não encontrado¦tunnel not found¦túnel no encontrado
não foi possível consultar as versões: {0}¦could not check the versions: {0}¦no se pudieron consultar las versiones: {0}
o GitHub respondeu {0}¦GitHub answered {0}¦GitHub respondió {0}
resposta sem versão¦response without a version¦respuesta sin versión
arquivo Zmodem não está aberto¦the Zmodem file is not open¦el archivo Zmodem no está abierto
pasta de destino inválida¦invalid destination folder¦carpeta de destino no válida
não foi possível escolher um nome livre¦could not choose a free name¦no se pudo elegir un nombre libre
tempo esgotado ({0} s)¦timed out ({0} s)¦tiempo agotado ({0} s)
local 127.0.0.1:{0} → {1}¦local 127.0.0.1:{0} → {1}¦local 127.0.0.1:{0} → {1}
remoto (servidor) 127.0.0.1:{0} → {1}¦remote (server) 127.0.0.1:{0} → {1}¦remoto (servidor) 127.0.0.1:{0} → {1}
Senha do proxy {0}¦Proxy password {0}¦Contraseña del proxy {0}
Senha da chave privada de {0}¦Private key passphrase for {0}¦Contraseña de la clave privada de {0}
Chave protegida¦Protected key¦Clave protegida
Verificação em duas etapas¦Two-step verification¦Verificación en dos pasos
no Wayland o sistema não permite atalhos globais; crie um atalho nas configurações do sistema que execute o comando nxaterm (ele traz a janela para a frente)¦on Wayland the system does not allow global shortcuts; create a shortcut in your system settings that runs the nxaterm command (it brings the window to the front)¦en Wayland el sistema no permite atajos globales; cree un atajo en la configuración del sistema que ejecute el comando nxaterm (trae la ventana al frente)
sem servidor gráfico X11 (DISPLAY vazio)¦no X11 graphical server (DISPLAY is empty)¦sin servidor gráfico X11 (DISPLAY vacío)
não foi possível conectar ao servidor X11: {0}¦could not connect to the X11 server: {0}¦no se pudo conectar al servidor X11: {0}
não foi possível ler o teclado: {0}¦could not read the keyboard: {0}¦no se pudo leer el teclado: {0}
a tecla "{0}" não existe no teclado atual¦the key "{0}" does not exist on the current keyboard¦la tecla "{0}" no existe en el teclado actual
o atalho "{0}" já está em uso por outro programa ({1})¦the shortcut "{0}" is already in use by another program ({1})¦el atajo "{0}" ya está en uso por otro programa ({1})
o macOS recusou o atalho "{0}" (já está em uso?): código {1}¦macOS refused the shortcut "{0}" (is it already in use?): code {1}¦macOS rechazó el atajo "{0}" (¿ya está en uso?): código {1}
o atalho global não está disponível neste sistema¦the global shortcut is not available on this system¦el atajo global no está disponible en este sistema
Editar túnel SSH¦Edit SSH tunnel¦Editar túnel SSH
Novo túnel SSH¦New SSH tunnel¦Nuevo túnel SSH
Iniciar todos¦Start all¦Iniciar todos
Parar todos¦Stop all¦Detener todos
Gerenciador gráfico de túneis SSH¦Graphical SSH tunnel manager¦Gestor gráfico de túneles SSH
Iniciar/parar¦Start/stop¦Iniciar/detener
Servidor de destino¦Destination server¦Servidor de destino
Ordem¦Order¦Orden
Subir¦Move up¦Subir
Descer¦Move down¦Bajar
Iniciar¦Start¦Iniciar
Dinâmico (SOCKS)¦Dynamic (SOCKS)¦Dinámico (SOCKS)
Excluir túnel¦Delete tunnel¦Eliminar túnel
Excluir o túnel "{0}"?¦Delete the tunnel "{0}"?¦¿Eliminar el túnel "{0}"?
Túnel "{0}" desconectado¦Tunnel "{0}" disconnected¦Túnel "{0}" desconectado
Túnel parado: inicie de novo para aplicar as mudanças¦Tunnel stopped: start it again to apply the changes¦Túnel detenido: inícielo de nuevo para aplicar los cambios
Senha de {0}¦Password for {0}¦Contraseña de {0}
Senha recusada¦Password rejected¦Contraseña rechazada
Redirecionamento de porta local¦Local port forwarding¦Redirección de puerto local
Redirecionamento de porta remota¦Remote port forwarding¦Redirección de puerto remoto
Redirecionamento dinâmico de porta (proxy SOCKS)¦Dynamic port forwarding (SOCKS proxy)¦Redirección dinámica de puertos (proxy SOCKS)
Conexão de aplicativos locais para o servidor remoto¦Connection from local applications to the remote server¦Conexión de aplicaciones locales al servidor remoto
Conexão de aplicativos locais para servidores remotos¦Connection from local applications to remote servers¦Conexión de aplicaciones locales a servidores remotos
Conexão de aplicativos remotos para este computador¦Connection from remote applications to this computer¦Conexión de aplicaciones remotas a este equipo
Clientes locais¦Local clients¦Clientes locales
Clientes remotos¦Remote clients¦Clientes remotos
Servidor local¦Local server¦Servidor local
Servidor remoto¦Remote server¦Servidor remoto
Servidor SSH¦SSH server¦Servidor SSH
Firewall¦Firewall¦Cortafuegos
Túnel SSH¦SSH tunnel¦Túnel SSH
Meu computador com NxATerm¦My computer running NxATerm¦Mi equipo con NxATerm
<Servidor remoto>¦<Remote server>¦<Servidor remoto>
<Servidor local>¦<Local server>¦<Servidor local>
<Porta remota>¦<Remote port>¦<Puerto remoto>
<Porta local>¦<Local port>¦<Puerto local>
<Porta encaminhada>¦<Forwarded port>¦<Puerto redirigido>
<Servidor SSH>¦<SSH server>¦<Servidor SSH>
<Login SSH>¦<SSH login>¦<Usuario SSH>
<Porta SSH>¦<SSH port>¦<Puerto SSH>
<Chave privada>¦<Private key>¦<Clave privada>
Informe a porta encaminhada (1-65535)¦Enter the forwarded port (1-65535)¦Indique el puerto redirigido (1-65535)
Informe o servidor remoto¦Enter the remote server¦Indique el servidor remoto
Informe o servidor local¦Enter the local server¦Indique el servidor local
Informe a porta remota (1-65535)¦Enter the remote port (1-65535)¦Indique el puerto remoto (1-65535)
Informe a porta local (1-65535)¦Enter the local port (1-65535)¦Indique el puerto local (1-65535)
Informe o servidor SSH¦Enter the SSH server¦Indique el servidor SSH
Porta SSH inválida (1-65535)¦Invalid SSH port (1-65535)¦Puerto SSH no válido (1-65535)
Informe o login SSH¦Enter the SSH login¦Indique el usuario SSH
Informe a chave privada¦Enter the private key¦Indique la clave privada
Nova versão {0} disponível (Sobre > Verificar atualizações)¦New version {0} available (About > Check for updates)¦Nueva versión {0} disponible (Acerca de > Buscar actualizaciones)
a porta {0} já está em uso¦port {0} is already in use¦el puerto {0} ya está en uso
endereço de escuta inválido¦invalid listen address¦dirección de escucha no válida
Túneis SSH ativos (clique para abrir)¦Active SSH tunnels (click to open)¦Túneles SSH activos (clic para abrir)
Iniciar ao abrir o NxATerm¦Start when NxATerm opens¦Iniciar al abrir NxATerm
Reconectar sozinho se a conexão cair¦Reconnect automatically if the connection drops¦Reconectar solo si la conexión se cae
Permitir conexões de outros computadores da rede¦Allow connections from other computers on the network¦Permitir conexiones de otros equipos de la red
Escuta em 0.0.0.0 em vez de só 127.0.0.1: qualquer máquina da rede poderá usar este túnel¦Listens on 0.0.0.0 instead of only 127.0.0.1: any machine on the network will be able to use this tunnel¦Escucha en 0.0.0.0 en lugar de solo 127.0.0.1: cualquier equipo de la red podrá usar este túnel
Tentando reconectar (clique para cancelar)¦Trying to reconnect (click to cancel)¦Intentando reconectar (clic para cancelar)
Túnel "{0}" caiu: reconectando...¦Tunnel "{0}" dropped: reconnecting...¦El túnel "{0}" se cayó: reconectando...
Túnel "{0}" reconectado¦Tunnel "{0}" reconnected¦Túnel "{0}" reconectado
Túnel "{0}": {1}¦Tunnel "{0}": {1}¦Túnel "{0}": {1}
A porta {0} já é usada pelo túnel "{1}"¦Port {0} is already used by the tunnel "{1}"¦El puerto {0} ya lo usa el túnel "{1}"
Todas as macros¦All macros¦Todas las macros
Macros de {0}¦Macros for {0}¦Macros de {0}
MultiExec: {0} sessões¦MultiExec: {0} sessions¦MultiExec: {0} sesiones
`
