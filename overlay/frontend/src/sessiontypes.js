// Tipos de sessão: rótulo, ícone e cor usados na janela de sessão, na lista e nas abas.
export const TYPE_INFO = {
  ssh: { label: 'SSH', icon: 'key', tabIcon: 'terminal', color: '#ffc94a', defPort: 22 },
  sftp: { label: 'SFTP', icon: 'folder', tabIcon: 'folder', color: '#f5a43a', defPort: 22 },
  telnet: { label: 'Telnet', icon: 'telnet', tabIcon: 'telnet', color: '#b07cff', defPort: 23 },
  serial: { label: 'Serial', icon: 'serial', tabIcon: 'serial', color: '#9aa7b3', defPort: 0 },
  shell: { label: 'Shell', icon: 'shell', tabIcon: 'shell', color: '#d0d0d0', defPort: 0 },
  wsl: { label: 'WSL', icon: 'wsl', tabIcon: 'wsl', color: '#4aa8ff', defPort: 0 },
}

/** O WSL e o PuTTY existem só no Windows. */
export const isWindows = /Windows/i.test(navigator.userAgent)

/** Tipos que o computador atual consegue abrir. */
export const availableTypes = () => Object.entries(TYPE_INFO).filter(([k]) => isWindows || k !== 'wsl')

/** Perfis antigos não têm tipo: são SSH. */
export const typeOf = (s) => (s && TYPE_INFO[s.type] ? s.type : 'ssh')

/** Tipos que abrem um terminal (todos menos o SFTP). */
export const hasTerminal = (s) => typeOf(s) !== 'sftp'

/** Tipos que passam por uma conexão SSH (e portanto têm arquivos). */
export const hasFiles = (s) => typeOf(s) === 'ssh' || typeOf(s) === 'sftp'

/** Texto curto que descreve para onde a sessão vai. */
export function sessionSummary(s) {
  switch (typeOf(s)) {
    case 'telnet': return `${s.host}:${s.port || 23}`
    case 'serial': return `${s.serialPort} · ${s.baudRate || 9600} baud`
    case 'shell': return s.shell || 'shell padrão'
    case 'wsl': return s.distro || 'distribuição padrão'
    default: return `${s.user}@${s.host}${s.port && s.port !== 22 ? ':' + s.port : ''}`
  }
}
