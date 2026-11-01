import { Api } from './api.js'
import { h, icon, toast, cleanErr, openModal, closeModal, confirmBox, combo } from './util.js'
import { ACTIONS, keyOf, setKey, resetKeys, isDefault, comboOf, capture } from './keys.js'
import { settings, saveSettings, FONTS, THEMES } from './settings.js'
import { TYPE_INFO, typeOf, availableTypes } from './sessiontypes.js'

const field = (label, el) => [h('label', {}, label), el]

// ------------------------------------------------------------------ sessão
const ENCODINGS = ['utf-8', 'windows-1252', 'iso-8859-2', 'iso-8859-15', 'windows-1251', 'koi8-r', 'gbk', 'gb18030', 'big5', 'shift_jis', 'euc-jp', 'euc-kr']
const TERMS = ['xterm-256color', 'xterm', 'vt100', 'vt220', 'linux', 'ansi']
const BAUDS = [1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600]

export function editSession(existing, onChange, preset) {
  const s = {
    type: 'ssh', name: '', folder: '', host: '', port: 0, user: '', authMethod: 'password', keyPath: '',
    startupCommand: '', notes: '', serialPort: '', baudRate: 9600, dataBits: 8, parity: 'none', stopBits: '1',
    shell: '', distro: '', proxy: '', proxyCommand: '', jumpHost: '', encoding: '', termType: '',
    startDir: '', env: '', x11: false, ...(existing || preset || {}),
  }
  let type = typeOf(s)

  const opt = (value, label, sel) => h('option', { value, selected: sel }, label)
  const f = {
    name: h('input', { type: 'text', value: s.name, placeholder: 'Automático' }),
    folder: h('input', { type: 'text', value: s.folder, placeholder: 'Sem pasta' }),
    host: h('input', { type: 'text', value: s.host, spellcheck: 'false' }),
    port: h('input', { type: 'number', value: s.port || TYPE_INFO[type].defPort || '', min: 1, max: 65535 }),
    user: h('input', { type: 'text', value: s.user, spellcheck: 'false' }),
    auth: h('select', {},
      opt('password', 'Senha', s.authMethod === 'password'),
      opt('key', 'Chave privada', s.authMethod === 'key'),
      opt('agent', 'Agente SSH', s.authMethod === 'agent')),
    keyPath: h('input', { type: 'text', value: s.keyPath, spellcheck: 'false' }),
    startup: h('textarea', { rows: 2, spellcheck: 'false' }, s.startupCommand),
    serialPort: h('input', { type: 'text', value: s.serialPort, spellcheck: 'false' }),
    baud: h('input', { type: 'number', value: s.baudRate || 9600, min: 50 }),
    dataBits: h('select', {}, [8, 7, 6, 5].map((n) => opt(String(n), String(n), (s.dataBits || 8) === n))),
    parity: h('select', {}, [['none', 'Nenhuma'], ['odd', 'Ímpar'], ['even', 'Par'], ['mark', 'Mark'], ['space', 'Space']]
      .map(([v, l]) => opt(v, l, (s.parity || 'none') === v))),
    stopBits: h('select', {}, [['1', '1'], ['1.5', '1,5'], ['2', '2']].map(([v, l]) => opt(v, l, (s.stopBits || '1') === v))),
    shell: h('input', { type: 'text', value: s.shell, spellcheck: 'false', placeholder: 'PowerShell' }),
    distro: h('input', { type: 'text', value: s.distro, spellcheck: 'false', placeholder: 'Padrão' }),
    proxySel: h('select', {},
      opt('', 'Proxy geral', !s.proxy),
      opt('none', 'Sem proxy', s.proxy === 'none'),
      opt('custom', 'Outro...', !!s.proxy && s.proxy !== 'none')),
    proxy: h('input', { type: 'text', value: s.proxy && s.proxy !== 'none' ? s.proxy : '', spellcheck: 'false', placeholder: 'socks5://host:1080' }),
    jump: h('select', {}, opt('', 'Nenhum', !s.jumpHost)),
    proxyCmd: h('input', { type: 'text', value: s.proxyCommand, spellcheck: 'false', placeholder: 'ssh -W %h:%p bastion' }),
    encoding: h('input', { type: 'text', value: s.encoding, spellcheck: 'false', placeholder: 'utf-8' }),
    termType: h('input', { type: 'text', value: s.termType, spellcheck: 'false', placeholder: 'xterm-256color' }),
    startDir: h('input', { type: 'text', value: s.startDir, spellcheck: 'false' }),
    env: h('textarea', { rows: 2, spellcheck: 'false', placeholder: 'NOME=valor' }, s.env),
    x11: h('input', { type: 'checkbox', checked: !!s.x11 }),
  }
  const cb = {
    serial: combo(f.serialPort, () => Api.SerialPorts()),
    baud: combo(f.baud, () => BAUDS),
    shell: combo(f.shell, () => Api.LocalShells()),
    distro: combo(f.distro, () => Api.WSLDistros()),
    encoding: combo(f.encoding, () => ENCODINGS),
    termType: combo(f.termType, () => TERMS),
  }
  f.proxySel.addEventListener('change', () => f.proxy.classList.toggle('hidden', f.proxySel.value !== 'custom'))
  f.proxy.classList.toggle('hidden', f.proxySel.value !== 'custom')
  // servidores de salto possíveis: as outras sessões SSH
  Api.ListSessions().then((list) => {
    for (const x of list) {
      if (x.id !== s.id && typeOf(x) === 'ssh') f.jump.append(opt(x.id, x.name, x.id === s.jumpHost))
    }
  }).catch(() => {})

  const keyRow = h('div', {},
    h('label', {}, 'Arquivo da chave privada'),
    h('div', { class: 'input-icon' }, f.keyPath,
      h('button', { class: 'in-btn', type: 'button', title: 'Procurar', onclick: async () => { const p = await Api.PickKeyFile(); if (p) f.keyPath.value = p } }, icon('file', 15))))
  const syncAuth = () => keyRow.classList.toggle('hidden', f.auth.value !== 'key')
  f.auth.addEventListener('change', syncAuth)
  syncAuth()

  const save = async () => {
    try {
      await Api.SaveSession({
        id: s.id || '', type, name: f.name.value, folder: f.folder.value, host: f.host.value,
        port: parseInt(f.port.value, 10) || 0, user: f.user.value, authMethod: f.auth.value,
        keyPath: f.keyPath.value, startupCommand: f.startup.value, notes: s.notes || '',
        proxy: f.proxySel.value === 'custom' ? f.proxy.value : f.proxySel.value, jumpHost: f.jump.value,
        proxyCommand: f.proxyCmd.value, encoding: f.encoding.value, termType: f.termType.value,
        startDir: f.startDir.value, env: f.env.value, x11: f.x11.checked,
        serialPort: f.serialPort.value, baudRate: parseInt(f.baud.value, 10) || 0,
        dataBits: parseInt(f.dataBits.value, 10) || 0, parity: f.parity.value, stopBits: f.stopBits.value,
        shell: f.shell.value, distro: f.distro.value,
      })
      closeModal()
      onChange()
    } catch (e) { toast(cleanErr(e), true) }
  }
  const remove = async () => {
    if (!(await confirmBox('Excluir sessão', `Excluir "${s.name}"? Isso não apaga nada no servidor.`))) return editSession(existing, onChange)
    try { await Api.DeleteSession(s.id); onChange() } catch (e) { toast(cleanErr(e), true) }
  }
  for (const el of [f.host, f.user, f.port, f.name, f.folder, f.serialPort, f.baud, f.shell, f.distro]) {
    el.addEventListener('keydown', (e) => { if (e.key === 'Enter') save() })
  }

  const grid = (cls, ...cells) => h('div', { class: cls }, cells.filter(Boolean).map(([label, el]) => h('div', {}, h('label', {}, label), el)))
  const fld = (label, el) => h('div', {}, h('label', {}, label), el)
  const startup = () => fld('Comandos ao conectar', f.startup)
  const sep = () => h('div', { class: 'sd-sep' })
  // nome e pasta ficam no fim de qualquer tipo: é tudo o que "favorito" precisava ter
  const book = () => grid('sd-grid-eq', ['Nome', f.name], ['Pasta', f.folder])
  const authRow = () => h('div', { class: 'sd-grid-eq' }, h('div', {}, h('label', {}, 'Autenticação'), f.auth), keyRow)

  // opções menos usadas ficam recolhidas, no mesmo painel
  const moreFor = (t) => {
    const net = t === 'ssh' || t === 'sftp' || t === 'telnet'
    const items = []
    if (net) {
      items.push(h('div', { class: 'sd-grid-eq' }, h('div', {}, h('label', {}, 'Servidor de salto'), f.jump), h('div', {}, h('label', {}, 'Proxy'), f.proxySel, f.proxy)))
      items.push(fld('ProxyCommand', f.proxyCmd))
    }
    if (t !== 'sftp') items.push(grid('sd-grid-eq', ['Codificação', cb.encoding], t === 'serial' ? null : ['Tipo de terminal', cb.termType]))
    if (t === 'ssh' || t === 'shell' || t === 'wsl') items.push(fld('Pasta inicial', f.startDir))
    if (t === 'ssh' || t === 'shell' || t === 'wsl') items.push(fld('Variáveis de ambiente', f.env))
    if (t === 'ssh') items.push(h('label', { class: 'check' }, f.x11, ' Encaminhar X11'))
    if (!items.length) return null
    const used = !!(s.proxy || s.proxyCommand || s.jumpHost || s.encoding || s.termType || s.startDir || s.env || s.x11)
    return h('details', { class: 'sd-more', open: used }, h('summary', {}, 'Mais opções'), ...items)
  }

  // um único painel por tipo de sessão, sem abas
  const panelFor = (t) => {
    switch (t) {
      case 'ssh':
      case 'sftp':
        return h('div', { class: 'sd-form' },
          grid('sd-grid3', ['Host remoto *', f.host], ['Usuário *', f.user], ['Porta', f.port]),
          authRow(),
          t === 'ssh' ? startup() : null,
          moreFor(t),
          sep(), book())
      case 'telnet':
        return h('div', { class: 'sd-form' },
          grid('sd-grid2', ['Host remoto *', f.host], ['Porta', f.port]),
          startup(),
          moreFor(t),
          sep(), book())
      case 'serial':
        return h('div', { class: 'sd-form' },
          grid('sd-grid2', ['Porta serial *', cb.serial], ['Velocidade (baud)', cb.baud]),
          grid('sd-grid3b', ['Bits de dados', f.dataBits], ['Paridade', f.parity], ['Bits de parada', f.stopBits]),
          startup(), moreFor(t), sep(), book())
      case 'shell':
        return h('div', { class: 'sd-form' }, fld('Programa do shell', cb.shell), startup(), moreFor(t), sep(), book())
      default: // wsl
        return h('div', { class: 'sd-form' }, fld('Distribuição Linux', cb.distro), startup(), moreFor(t), sep(), book())
    }
  }

  const typesRow = h('div', { class: 'sd-types' })
  const body = h('div', { class: 'sd-body' })
  const render = () => {
    typesRow.replaceChildren(...availableTypes().map(([k, info]) =>
      h('button', {
        class: 'sd-type' + (k === type ? ' active' : ''), style: `--c:${info.color}`, title: info.label,
        onclick: () => {
          const old = TYPE_INFO[type].defPort
          if (!f.port.value || parseInt(f.port.value, 10) === old) f.port.value = TYPE_INFO[k].defPort || ''
          type = k
          render()
        },
      }, h('span', { class: 'tb-ico' }, icon(info.icon, 26)), info.label)))
    body.replaceChildren(panelFor(type))
    const first = type === 'serial' ? f.serialPort : type === 'shell' ? f.shell : type === 'wsl' ? f.distro : f.host
    setTimeout(() => first.focus(), 0)
  }
  render()

  openModal([
    h('div', { class: 'sd-title' }, existing ? 'Editar sessão' : 'Configurações da sessão',
      h('button', { class: 'sd-x', title: 'Fechar', onclick: closeModal }, icon('close', 15))),
    typesRow, body,
    h('div', { class: 'sd-foot' },
      existing ? h('button', { class: 'danger', onclick: remove }, 'Excluir') : null,
      h('span', { class: 'spacer' }),
      h('button', { class: 'sd-ok', onclick: save }, icon('check', 17), ' OK'),
      h('button', { class: 'sd-cancel', onclick: closeModal }, icon('close', 17), ' Cancelar'),
      h('span', { class: 'spacer' })),
  ], 'sdlg')
}

// ------------------------------------------------------------------ macro
export function editMacro(existing, onChange) {
  const m = existing || { name: '', command: '', enter: true }
  const name = h('input', { type: 'text', value: m.name })
  const cmd = h('textarea', { rows: 5, spellcheck: 'false' }, m.command)
  const enter = h('input', { type: 'checkbox', checked: m.enter !== false })
  const save = async () => {
    try {
      await Api.SaveMacro({ id: m.id || '', name: name.value, command: cmd.value, enter: enter.checked })
      closeModal()
      onChange()
    } catch (e) { toast(cleanErr(e), true) }
  }
  const remove = async () => {
    if (!(await confirmBox('Excluir macro', `Excluir a macro "${m.name}"?`))) return editMacro(existing, onChange)
    try { await Api.DeleteMacro(m.id); onChange() } catch (e) { toast(cleanErr(e), true) }
  }
  openModal([
    h('h2', {}, existing ? 'Editar macro' : 'Nova macro'),
    field('Nome', name), field('Comandos', cmd),
    h('label', { class: 'check' }, enter, ' Enviar Enter no final (executa o comando)'),
    h('div', { class: 'btns' },
      existing ? h('button', { class: 'danger', onclick: remove }, 'Excluir') : null,
      h('span', { class: 'spacer' }),
      h('button', { onclick: closeModal }, 'Cancelar'),
      h('button', { class: 'primary', onclick: save }, 'Salvar')),
  ])
  name.focus()
}

// ------------------------------------------------------------------ túneis
export function tunnelDialog(getActive) {
  const act = getActive()
  const kind = h('select', {},
    h('option', { value: 'local' }, 'Local'),
    h('option', { value: 'remote' }, 'Remoto'),
    h('option', { value: 'dynamic' }, 'Dinâmico (SOCKS5)'))
  const lport = h('input', { type: 'number', min: 1, max: 65535 })
  const dhost = h('input', { type: 'text', value: 'localhost', spellcheck: 'false' })
  const dport = h('input', { type: 'number', min: 1, max: 65535 })
  const destRow = h('div', { class: 'row' },
    h('div', {}, h('label', {}, 'Host de destino'), dhost),
    h('div', { class: 'narrow' }, h('label', {}, 'Porta destino'), dport))
  const list = h('div', { class: 'tun-list' })
  const sync = () => {
    destRow.classList.toggle('hidden', kind.value === 'dynamic')
  }
  kind.addEventListener('change', sync)
  sync()

  const refresh = async () => {
    try {
      const all = (await Api.TunnelList()) || []
      list.replaceChildren(...(all.length
        ? all.map((t) => h('div', { class: 'tun-row' },
          h('span', { class: 'grow' }, t.desc),
          h('button', {
            class: 'danger', onclick: async () => {
              try { await Api.TunnelStop(t.id) } catch (e) { toast(cleanErr(e), true) }
              refresh()
            },
          }, 'Parar')))
        : [h('div', { class: 'muted' }, 'Nenhum túnel ativo.')]))
    } catch (e) { list.replaceChildren(h('div', { class: 'muted' }, cleanErr(e))) }
  }
  const start = async () => {
    const a = getActive()
    if (!a) return toast('Abra uma sessão SSH primeiro.', true)
    try {
      await Api.TunnelStart(a.id, kind.value, parseInt(lport.value, 10) || 0, dhost.value.trim(), parseInt(dport.value, 10) || 0)
      toast('Túnel ativo')
      refresh()
    } catch (e) { toast(cleanErr(e), true) }
  }

  openModal([
    h('h2', {}, 'Túneis SSH'),
    field('Tipo', kind),
    h('label', {}, 'Porta de escuta'), lport,
    destRow,
    h('div', { class: 'btns' }, h('button', { class: 'primary', onclick: start, disabled: !act }, 'Iniciar túnel')),
    h('h3', {}, 'Túneis ativos'), list,
    h('div', { class: 'btns' }, h('button', { onclick: closeModal }, 'Fechar')),
  ], 'wide')
  refresh()
}

// ------------------------------------------------------------------ gerador de chaves
export function keygenDialog() {
  const comment = h('input', { type: 'text', spellcheck: 'false' })
  const pass = h('input', { type: 'password', placeholder: 'Opcional' })
  const out = h('div', {})
  const go = async () => {
    try {
      const r = await Api.GenerateKey(comment.value.trim(), pass.value)
      if (!r || !r.path) return // cancelou
      const ta = h('textarea', { rows: 4, readonly: true, spellcheck: 'false' }, r.publicKey)
      out.replaceChildren(
        h('p', {}, 'Chave criada em: ' + r.path),
        h('label', {}, 'Chave pública'),
        ta,
        h('button', { onclick: () => navigator.clipboard.writeText(r.publicKey).then(() => toast('Copiado')).catch(() => {}) }, 'Copiar chave pública'))
    } catch (e) { toast(cleanErr(e), true) }
  }
  openModal([
    h('h2', {}, 'Gerador de chaves SSH'),
    field('Comentário', comment), field('Senha da chave', pass),
    h('div', { class: 'btns' },
      h('button', { onclick: closeModal }, 'Fechar'),
      h('button', { class: 'primary', onclick: go }, 'Gerar e salvar...')),
    out,
  ], 'wide')
  comment.focus()
}

// ------------------------------------------------------------------ configurações
export function settingsDialog(onApply) {
  const sel = (opts, cur) => h('select', {}, ...opts.map(([v, l]) => h('option', { value: v, selected: v === cur }, l)))
  const f = {
    font: sel(Object.keys(FONTS).map((k) => [k, k]), settings.font),
    size: h('input', { type: 'number', min: 8, max: 32, value: settings.fontSize }),
    theme: sel(Object.keys(THEMES).map((k) => [k, k]), settings.theme),
    cursor: sel([['block', 'Bloco'], ['bar', 'Barra'], ['underline', 'Sublinhado']], settings.cursorStyle),
    blink: h('input', { type: 'checkbox', checked: settings.cursorBlink }),
    scroll: h('input', { type: 'number', min: 500, max: 200000, step: 500, value: settings.scrollback }),
    copy: h('input', { type: 'checkbox', checked: settings.copyOnSelect }),
    right: sel([['paste', 'Colar'], ['menu', 'Abrir menu']], settings.rightClick),
    autoSftp: h('input', { type: 'checkbox', checked: settings.autoSftp }),
    monitoring: h('input', { type: 'checkbox', checked: settings.monitoring }),
    follow: h('input', { type: 'checkbox', checked: settings.followFolder }),
    proxy: h('input', { type: 'text', value: settings.proxy, spellcheck: 'false', placeholder: 'Sem proxy (socks5://host:1080 ou http://host:3128)' }),
    editor: h('input', { type: 'text', value: settings.editor, spellcheck: 'false', placeholder: 'Programa padrão do sistema' }),
    multi: h('input', { type: 'checkbox', checked: settings.multiInstance }),
  }
  const linkBtn = h('button', { onclick: async () => {
    try { await Api.RegisterLinkHandler(); toast('Links ssh:// e telnet:// agora abrem no NxATerm') } catch (e) { toast(cleanErr(e), true) }
  } }, 'Abrir links ssh:// e telnet:// com o NxATerm')
  const apply = () => {
    settings.font = f.font.value
    settings.fontSize = Math.min(32, Math.max(8, parseInt(f.size.value, 10) || 14))
    settings.theme = f.theme.value
    settings.cursorStyle = f.cursor.value
    settings.cursorBlink = f.blink.checked
    settings.scrollback = Math.min(200000, Math.max(500, parseInt(f.scroll.value, 10) || 10000))
    settings.copyOnSelect = f.copy.checked
    settings.rightClick = f.right.value
    settings.autoSftp = f.autoSftp.checked
    settings.monitoring = f.monitoring.checked
    settings.followFolder = f.follow.checked
    settings.proxy = f.proxy.value.trim()
    settings.editor = f.editor.value.trim()
    settings.multiInstance = f.multi.checked
    saveSettings()
    closeModal()
    onApply()
  }
  openModal([
    h('h2', {}, 'Configurações'),
    h('div', { class: 'row' },
      h('div', {}, h('label', {}, 'Fonte do terminal'), f.font),
      h('div', { class: 'narrow' }, h('label', {}, 'Tamanho'), f.size)),
    field('Tema de cores', f.theme),
    h('div', { class: 'row' },
      h('div', {}, h('label', {}, 'Cursor'), f.cursor),
      h('div', {}, h('label', {}, 'Linhas de histórico'), f.scroll)),
    h('label', { class: 'check' }, f.blink, ' Cursor piscando'),
    h('label', { class: 'check' }, f.copy, ' Copiar ao selecionar'),
    field('Botão direito no terminal', f.right),
    h('h3', {}, 'Servidores SSH'),
    h('label', { class: 'check' }, f.autoSftp, ' Abrir o navegador SFTP ao conectar'),
    h('label', { class: 'check' }, f.follow, ' Navegador SFTP acompanha o terminal'),
    h('label', { class: 'check' }, f.monitoring, ' Uso do servidor na barra de status'),
    field('Editor de arquivos remotos', f.editor),
    h('h3', {}, 'Conexão'),
    field('Proxy geral', f.proxy),
    h('h3', {}, 'Sistema'),
    h('label', { class: 'check' }, f.multi, ' Permitir várias janelas do NxATerm'),
    h('div', { class: 'row' }, linkBtn),
    h('div', { class: 'btns' },
      h('button', { onclick: closeModal }, 'Cancelar'),
      h('button', { class: 'primary', onclick: apply }, 'Aplicar')),
  ])
}

// ------------------------------------------------------------------ ajuda
export function aboutDialog() {
  openModal([
    h('div', { class: 'about' },
      h('div', { class: 'about-logo' }, icon('terminal', 30)),
      h('div', { class: 'about-name' }, 'NxATerm'),
      h('div', { class: 'about-by' }, 'Criado por ', h('b', {}, 'Alexandre Garcia'), ' (alekhine-dev)'),
      h('div', { class: 'about-stack' }, ['Go', 'Wails v3', 'xterm.js', 'Vite'].map((t) => h('span', { class: 'chip' }, t)))),
    h('div', { class: 'btns center' }, h('button', { class: 'primary', onclick: closeModal }, 'Fechar')),
  ], 'about-modal')
}

// ------------------------------------------------------------------ atalhos de teclado
export function shortcutsDialog() {
  const list = h('div', { class: 'keys-list' })
  const render = () => {
    list.replaceChildren(...ACTIONS.map((a) => {
      const k = keyOf(a.id)
      const btn = h('button', { class: 'key-btn' + (k ? '' : ' empty') }, k || 'Nenhum')
      btn.addEventListener('click', () => record(a, btn))
      return h('div', { class: 'key-row' },
        h('span', { class: 'key-name' }, a.label),
        btn,
        h('button', {
          class: 'ib key-reset', title: 'Restaurar padrão', style: isDefault(a.id) ? 'visibility:hidden' : '',
          onclick: () => { setKey(a.id, a.def); render() },
        }, icon('undo', 14)))
    }))
  }
  const record = (a, btn) => {
    if (capture.active) return
    capture.active = true
    btn.textContent = 'Pressione as teclas'
    btn.classList.add('recording')
    const stop = () => {
      capture.active = false
      document.removeEventListener('keydown', onKey, true)
      document.removeEventListener('mousedown', onAway, true)
      render()
    }
    const onKey = (e) => {
      e.preventDefault(); e.stopPropagation()
      if (e.key === 'Escape') return stop()
      if (e.key === 'Backspace' || e.key === 'Delete') { setKey(a.id, ''); return stop() }
      const c = comboOf(e)
      if (!c) return
      const taken = setKey(a.id, c)
      stop()
      if (taken) toast(`"${taken}" ficou sem atalho`)
    }
    const onAway = (e) => { if (e.target !== btn) stop() }
    document.addEventListener('keydown', onKey, true)
    document.addEventListener('mousedown', onAway, true)
  }
  render()
  openModal([
    h('h2', {}, 'Atalhos de teclado'),
    list,
    h('div', { class: 'btns' },
      h('button', { onclick: () => { resetKeys(); render() } }, 'Restaurar todos'),
      h('span', { class: 'spacer' }),
      h('button', { class: 'primary', onclick: closeModal }, 'Fechar')),
  ], 'keys-modal')
}

export { icon }
