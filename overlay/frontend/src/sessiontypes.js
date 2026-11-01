// Tipos de sessão: rótulo, ícone e cor usados na janela de sessão, na lista e nas abas.
export const TYPE_INFO = {
  ssh: { label: 'SSH', icon: 'key', tabIcon: 'terminal', color: '#ffc94a', defPort: 22 },
  sftp: { label: 'SFTP', icon: 'folder', tabIcon: 'folder', color: '#f5a43a', defPort: 22 },
  ftp: { label: 'FTP', icon: 'ftp', tabIcon: 'ftp', color: '#4fd1d1', defPort: 21 },
  telnet: { label: 'Telnet', icon: 'telnet', tabIcon: 'telnet', color: '#b07cff', defPort: 23 },
  serial: { label: 'Serial', icon: 'serial', tabIcon: 'serial', color: 'var(--g-9aa3ad)', defPort: 0 },
  shell: { label: 'Shell', icon: 'shell', tabIcon: 'shell', color: 'var(--g-cccccc)', defPort: 0 },
  wsl: { label: 'WSL', icon: 'wsl', tabIcon: 'wsl', color: '#4aa8ff', defPort: 0 },
}

/** O WSL e o PuTTY existem só no Windows. */
export const isWindows = /Windows/i.test(navigator.userAgent)

/** Tipos que o computador atual consegue abrir. */
export const availableTypes = () => Object.entries(TYPE_INFO).filter(([k]) => isWindows || k !== 'wsl')

/** Perfis antigos não têm tipo: são SSH. */
export const typeOf = (s) => (s && TYPE_INFO[s.type] ? s.type : 'ssh')

/** Tipos que só mostram arquivos, sem terminal. */
export const FILE_ONLY = new Set(['sftp', 'ftp'])

/** Tipos que abrem um terminal (todos menos os de só arquivos). */
export const hasTerminal = (s) => !FILE_ONLY.has(typeOf(s))

/** Tipos que passam por uma conexão SSH (aceitam túneis, permissões e cópia no servidor). */
export const hasSSH = (s) => typeOf(s) === 'ssh' || typeOf(s) === 'sftp'

/** Tipos que têm navegador de arquivos. */
export const hasFiles = (s) => hasSSH(s) || typeOf(s) === 'ftp'

/** Texto curto que descreve para onde a sessão vai. */
export function sessionSummary(s) {
  switch (typeOf(s)) {
    case 'telnet': return `${s.host}:${s.port || 23}`
    case 'serial': return `${s.serialPort} · ${s.baudRate || 9600} baud`
    case 'shell': return s.shell || 'shell padrão'
    case 'wsl': return s.distro || 'distribuição padrão'
    case 'ftp': return `${s.ftpSecurity ? 'ftps' : 'ftp'}://${s.user ? s.user + '@' : ''}${s.host}${s.port && s.port !== 21 ? ':' + s.port : ''}`
    default: return `${s.user}@${s.host}${s.port && s.port !== 22 ? ':' + s.port : ''}`
  }
}
