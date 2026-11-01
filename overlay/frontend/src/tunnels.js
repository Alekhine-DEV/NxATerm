// Janela "Túneis SSH": lista de túneis salvos (cada um com a sua própria conexão SSH) e o editor
// com o desenho do caminho dos dados.
import * as Runtime from '@wailsio/runtime'
import { Api } from './api.js'
import { h, icon, toast, cleanErr, openModal, closeModal, confirmBox, promptBox, combo } from './util.js'
import { memo, memoKey } from './memo.js'
import * as art from './tunnelart.js'

const KINDS = { local: 'Local', remote: 'Remoto', dynamic: 'Dinâmico (SOCKS)' }
const CANCELED = Symbol('cancelado')

let defs = []
const active = new Map() // id do túnel salvo -> túnel ativo
const busy = new Set() // conectando
let ctx = { sessions: () => [], current: () => null }

const uid = () => Math.random().toString(36).slice(2, 10) + Date.now().toString(36).slice(-4)
const portOk = (n) => Number.isInteger(n) && n >= 1 && n <= 65535
const visible = () => !!document.querySelector('.tnwin')

Runtime.Events.On('tunnel:closed', (ev) => {
  const id = ev && typeof ev === 'object' && 'data' in ev ? ev.data : ev
  const d = defs.find((x) => x.id === id)
  active.delete(id)
  if (d) toast(`Túnel "${d.name}" desconectado`, true)
  if (visible()) show()
})

async function load() {
  try {
    const js = await Api.LoadTunnelDefs()
    defs = js ? (JSON.parse(js).tunnels || []) : []
  } catch (e) { defs = []; toast(cleanErr(e), true) }
  await refreshActive()
}

async function refreshActive() {
  active.clear()
  try { for (const t of (await Api.TunnelList()) || []) active.set(t.defId, t) } catch { /* sem lista */ }
}

async function persist() {
  try { await Api.SaveTunnelDefs(JSON.stringify({ tunnels: defs })) } catch (e) { toast(cleanErr(e), true) }
}

/** Abre a janela. c.sessions() lista as sessões salvas; c.current() devolve a sessão SSH da aba ativa (ou null). */
export async function openTunnelWindow(c) {
  ctx = c
  await load()
  show()
}

// ------------------------------------------------------------------ lista
const destText = (d) => (d.kind === 'dynamic' ? 'SOCKS5' : `${d.destHost}:${d.destPort}`)
const sshText = (d) => `${d.user}@${d.host}${d.port && d.port !== 22 ? ':' + d.port : ''}`

function show() {
  const cell = (cls, ...kids) => h('div', { class: 'tn-c ' + cls }, ...kids)
  const head = h('div', { class: 'tn-row head' },
    cell('name', 'Nome'), cell('type', 'Tipo'), cell('run', 'Iniciar/parar'), cell('port', 'Porta'),
    cell('dest', 'Servidor de destino'), cell('ssh', 'Servidor SSH'), cell('cfg', 'Configurações'), cell('ord', 'Ordem'))

  const rows = defs.map((d, i) => {
    const on = active.has(d.id)
    const wait = busy.has(d.id)
    const run = h('button', {
      class: 'tn-run' + (on ? ' on' : ''), disabled: wait, title: on ? 'Parar' : 'Iniciar',
      onclick: () => (on ? stop(d) : start(d, new Map())),
    }, icon(wait ? 'clock' : on ? 'stop' : 'play', 16))
    return h('div', { class: 'tn-row' + (on ? ' on' : '') },
      cell('name', h('span', { class: 'tn-dot' }), d.name),
      cell('type', KINDS[d.kind] || d.kind),
      cell('run', run),
      cell('port', String(d.listenPort)),
      cell('dest', destText(d)),
      cell('ssh', sshText(d)),
      cell('cfg',
        h('button', { class: 'ib', title: 'Editar', onclick: () => editTunnel(d) }, icon('pencil', 15)),
        h('button', { class: 'ib', title: 'Excluir', onclick: () => remove(d) }, icon('trash', 15))),
      cell('ord',
        h('button', { class: 'ib', title: 'Subir', disabled: i === 0, onclick: () => move(i, -1) }, icon('up', 15)),
        h('button', { class: 'ib', title: 'Descer', disabled: i === defs.length - 1, onclick: () => move(i, 1) }, icon('chev', 15))))
  })

  const anyOn = active.size > 0
  openModal([
    h('div', { class: 'sd-title' }, h('span', { class: 'tn-ttl' }, icon('tunnel', 15), ' Túneis SSH'),
      h('button', { class: 'sd-x', title: 'Fechar', onclick: closeModal }, icon('close', 15))),
    h('div', { class: 'tn-welcome' }, 'Gerenciador gráfico de túneis SSH'),
    h('div', { class: 'tn-table' }, head, h('div', { class: 'tn-body' }, rows)),
    h('div', { class: 'sd-foot tn-foot' },
      h('button', { class: 'tn-btn', onclick: () => editTunnel(null) }, icon('plus', 17), ' Novo túnel SSH'),
      h('button', { class: 'tn-btn', disabled: !defs.length || defs.every((d) => active.has(d.id)), onclick: startAll }, icon('playall', 17), ' Iniciar todos'),
      h('button', { class: 'tn-btn', disabled: !anyOn, onclick: stopAll }, icon('stop', 17), ' Parar todos'),
      h('span', { class: 'spacer' }),
      h('button', { class: 'tn-btn', onclick: closeModal }, icon('close', 17), ' Fechar')),
  ], 'sdlg tnwin')
}

async function move(i, delta) {
  const j = i + delta
  if (j < 0 || j >= defs.length) return
  ;[defs[i], defs[j]] = [defs[j], defs[i]]
  await persist()
  show()
}

async function remove(d) {
  if (!(await confirmBox('Excluir túnel', `Excluir o túnel "${d.name}"?`))) return show()
  const a = active.get(d.id)
  if (a) { try { await Api.TunnelStop(a.id) } catch { /* já parado */ } active.delete(d.id) }
  defs = defs.filter((x) => x.id !== d.id)
  await persist()
  show()
}

// ------------------------------------------------------------------ iniciar e parar
async function secretFor(d, kind, cache, force) {
  const key = memoKey(kind, { user: d.user, host: d.host, port: d.port || 22 })
  if (!force) {
    if (memo.has(key)) return memo.get(key)
    if (cache.has(key)) return cache.get(key)
  }
  const r = await promptBox({
    title: kind === 'pw' ? 'Senha' : 'Chave protegida',
    label: kind === 'pw' ? `Senha de ${d.user}@${d.host}` : 'Senha da chave privada',
    secret: true, ok: 'Conectar', remember: 'Memorizar até fechar o NxATerm',
  })
  if (r === null || r === undefined) throw CANCELED
  if (r.remember) memo.set(key, r.value)
  cache.set(key, r.value)
  return r.value
}

async function openOne(d, cache) {
  let secret = d.authMethod === 'password' ? await secretFor(d, 'pw', cache) : ''
  for (let attempt = 0; ; attempt++) {
    try {
      return await Api.TunnelOpen(d, secret)
    } catch (e) {
      const msg = String(e)
      if (attempt >= 2) throw e
      if (msg.includes('PASSPHRASE_REQUIRED')) {
        secret = await secretFor(d, 'pp', cache, attempt > 0)
      } else if (d.authMethod === 'password' && /unable to authenticate/.test(msg)) {
        const key = memoKey('pw', { user: d.user, host: d.host, port: d.port || 22 })
        memo.delete(key)
        cache.delete(key)
        toast('Senha recusada', true)
        secret = await secretFor(d, 'pw', cache, true)
      } else throw e
    }
  }
}

async function start(d, cache) {
  if (active.has(d.id) || busy.has(d.id)) return
  busy.add(d.id)
  show()
  try {
    await openOne(d, cache)
  } catch (e) {
    if (e !== CANCELED) toast(`${d.name}: ${cleanErr(e)}`, true)
  } finally {
    busy.delete(d.id)
  }
  await refreshActive()
  show()
}

async function stop(d) {
  const a = active.get(d.id)
  if (!a) return
  try { await Api.TunnelStop(a.id) } catch (e) { toast(cleanErr(e), true) }
  await refreshActive()
  show()
}

async function startAll() {
  const cache = new Map()
  for (const d of defs) if (!active.has(d.id)) await start(d, cache)
}

async function stopAll() {
  for (const a of [...active.values()]) { try { await Api.TunnelStop(a.id) } catch { /* já parado */ } }
  await refreshActive()
  show()
}

// ------------------------------------------------------------------ editor
const CAPTION = {
  local: 'Conexão de aplicativos locais para o servidor remoto',
  remote: 'Conexão de aplicativos remotos para este computador',
  dynamic: 'Conexão de aplicativos locais para servidores remotos',
}

function editTunnel(existing) {
  const t = {
    id: uid(), name: '', kind: 'local', listenPort: 0, destHost: '', destPort: 0, sessionId: '',
    host: '', port: 22, user: '', authMethod: 'password', keyPath: '', ...(existing || {}),
  }
  if (!existing) {
    const cur = ctx.current()
    if (cur) Object.assign(t, { sessionId: cur.id || '', host: cur.host, port: cur.port || 22, user: cur.user, authMethod: cur.authMethod || 'password', keyPath: cur.keyPath || '' })
  }
  const sshSessions = (ctx.sessions() || []).filter((s) => s.host && (s.type === 'ssh' || s.type === 'sftp' || !s.type))
  const label = (s) => `${s.name} (${s.user}@${s.host})`
  const byLabel = new Map(sshSessions.map((s) => [label(s), s]))

  const stage = h('div', { class: 'tn-stage' })
  const radios = h('div', { class: 'tn-kinds' })
  const name = h('input', { type: 'text', value: t.name, placeholder: 'Automático', spellcheck: 'false' })

  // ---- peças do desenho
  const put = (el, x, y, w, h2) => {
    el.style.left = x + 'px'
    el.style.top = y + 'px'
    if (w) el.style.width = w + 'px'
    if (h2) el.style.height = h2 + 'px'
    stage.append(el)
    return el
  }
  const picture = (fn, x, y, w, h2, text, textY) => {
    const el = put(h('div', { class: 'tn-art' }), x, y, w, h2)
    el.innerHTML = fn()
    if (text) put(h('div', { class: 'tn-lbl' }, text), x - 40, textY, w + 80)
  }
  const line = (x, y1, y2) => put(h('div', { class: 'tn-vline' }), x, y1, 2, y2 - y1)
  const arrow = (x, y, dir) => put(h('div', { class: 'tn-arrow' }, dir === 'l' ? '◀' : '▶'), x, y)
  const boxed = (x, y, w, h2) => put(h('div', { class: 'tn-box' }), x, y, w, h2)

  const text = (key, ph) => {
    const i = h('input', { type: 'text', class: 'tn-in', value: t[key] || '', placeholder: `<${ph}>`, spellcheck: 'false' })
    i.addEventListener('input', () => { t[key] = i.value })
    return i
  }
  const num = (key, ph) => {
    const i = h('input', { type: 'number', class: 'tn-in', value: t[key] || '', placeholder: `<${ph}>`, min: 1, max: 65535 })
    i.addEventListener('input', () => { t[key] = parseInt(i.value, 10) || 0 })
    return i
  }

  const sshBox = () => {
    boxed(430, 246, 188, 98)
    const host = put(text('host', 'Servidor SSH'), 438, 252, 172)
    combo(host, () => sshSessions.map(label))
    host.addEventListener('input', () => {
      const s = byLabel.get(host.value)
      if (s) {
        Object.assign(t, { sessionId: s.id, host: s.host, port: s.port || 22, user: s.user, authMethod: s.authMethod || 'password', keyPath: s.keyPath || '' })
        setTimeout(draw, 0)
      } else {
        t.host = host.value
        t.sessionId = ''
      }
    })
    const user = put(text('user', 'Login SSH'), 438, 280, 172)
    combo(user, () => [...new Set(sshSessions.map((s) => s.user).filter(Boolean))])
    put(num('port', 'Porta SSH'), 438, 308, 120)
    arrow(626, 288, 'r')

    // forma de entrar no servidor
    const auth = h('select', { class: 'tn-in' },
      h('option', { value: 'password', selected: t.authMethod === 'password' }, 'Senha'),
      h('option', { value: 'key', selected: t.authMethod === 'key' }, 'Chave privada'),
      h('option', { value: 'agent', selected: t.authMethod === 'agent' }, 'Agente SSH'))
    auth.addEventListener('change', () => { t.authMethod = auth.value; draw() })
    put(auth, 430, 356, 150)
    if (t.authMethod === 'key') {
      const key = h('input', { type: 'text', class: 'tn-in', value: t.keyPath, placeholder: '<Chave privada>', spellcheck: 'false' })
      key.addEventListener('input', () => { t.keyPath = key.value })
      put(key, 590, 356, 170)
      const pick = h('button', { class: 'tn-pick', type: 'button', title: 'Procurar', onclick: async () => { const p = await Api.PickKeyFile(); if (p) { t.keyPath = p; key.value = p } } }, icon('file', 14))
      put(pick, 736, 357)
    }
  }

  const draw = () => {
    stage.replaceChildren()
    put(h('div', { class: 'tn-caption' }, CAPTION[t.kind]), 120, 4, 540)

    if (t.kind === 'remote') {
      picture(art.server, 52, 40, 100, 100, 'Servidor local', 18)
      line(102, 144, 262)
      boxed(184, 50, 176, 70)
      put(text('destHost', 'Servidor local'), 192, 56, 160)
      put(num('destPort', 'Porta local'), 192, 84, 120)
      arrow(162, 74, 'l')
      picture(art.people, 590, 36, 140, 92, 'Clientes remotos', 14)
      line(660, 130, 262)
      boxed(402, 166, 184, 36)
      put(num('listenPort', 'Porta encaminhada'), 410, 172, 168)
      arrow(596, 174, 'r')
    } else {
      picture(art.people, 16, 36, 140, 92, 'Clientes locais', 14)
      line(86, 130, 262)
      boxed(104, 176, 184, 36)
      put(num('listenPort', 'Porta encaminhada'), 112, 182, 168)
      arrow(292, 184, 'l')
      picture(art.server, 622, 34, 104, 104, 'Servidor remoto', 14)
      line(674, 142, 262)
      if (t.kind === 'local') {
        boxed(436, 52, 172, 70)
        put(text('destHost', 'Servidor remoto'), 444, 58, 156)
        put(num('destPort', 'Porta remota'), 444, 86, 120)
        arrow(612, 76, 'r')
      }
    }

    // parte de baixo, igual nos três tipos (o tubo passa por trás do firewall e dos computadores)
    put(h('div', { class: 'tn-tube ' + t.kind }, h('span', {}, 'Túnel SSH')), 130, 318, 510, 20)
    picture(art.computer, 12, 262, 130, 100, '', 0)
    put(h('div', { class: 'tn-lbl strong' }, 'Meu computador com NxATerm'), -10, 366, 190)
    picture(art.firewall, 262, 252, 96, 92, 'Firewall', 350)
    picture(art.sshServer, 626, 262, 130, 100, '', 0)
    put(h('div', { class: 'tn-lbl strong' }, 'Servidor SSH'), 596, 366, 190)
    sshBox()
  }

  const kinds = ['local', 'remote', 'dynamic'].map((k) => {
    const r = h('input', { type: 'radio', name: 'tn-kind', value: k, checked: t.kind === k })
    r.addEventListener('change', () => { t.kind = k; draw() })
    return h('label', { class: 'tn-kind' }, r, ' ', ({ local: 'Redirecionamento de porta local', remote: 'Redirecionamento de porta remota', dynamic: 'Redirecionamento dinâmico de porta (proxy SOCKS)' })[k])
  })
  radios.append(...kinds)
  draw()

  const save = async () => {
    const e = check(t)
    if (e) return toast(e, true)
    t.name = name.value.trim() || `${KINDS[t.kind].split(' ')[0]} ${t.listenPort}`
    if (t.kind === 'dynamic') { t.destHost = ''; t.destPort = 0 }
    t.host = t.host.trim()
    t.user = t.user.trim()
    t.destHost = t.destHost.trim()
    const a = active.get(t.id)
    const i = defs.findIndex((x) => x.id === t.id)
    if (i >= 0) defs[i] = { ...t }
    else defs.push({ ...t })
    await persist()
    if (a) {
      try { await Api.TunnelStop(a.id) } catch { /* já parado */ }
      await refreshActive()
      toast('Túnel parado: inicie de novo para aplicar as mudanças')
    }
    show()
  }

  openModal([
    h('div', { class: 'sd-title' }, h('span', { class: 'tn-ttl' }, icon('tunnel', 15), existing ? ' Editar túnel SSH' : ' Novo túnel SSH'),
      h('button', { class: 'sd-x', title: 'Fechar', onclick: show }, icon('close', 15))),
    h('div', { class: 'tn-edit' }, radios, h('div', { class: 'tn-stagewrap' }, stage),
      h('div', { class: 'tn-name' }, h('label', {}, 'Nome'), name)),
    h('div', { class: 'sd-foot' },
      h('span', { class: 'spacer' }),
      h('button', { class: 'sd-ok', onclick: save }, icon('check', 17), ' Salvar'),
      h('button', { class: 'sd-cancel', onclick: show }, icon('close', 17), ' Cancelar'),
      h('span', { class: 'spacer' })),
  ], 'sdlg tnedit')
}

function check(t) {
  if (!portOk(t.listenPort)) return 'Informe a porta encaminhada (1-65535)'
  if (t.kind !== 'dynamic') {
    if (!t.destHost.trim()) return t.kind === 'local' ? 'Informe o servidor remoto' : 'Informe o servidor local'
    if (!portOk(t.destPort)) return t.kind === 'local' ? 'Informe a porta remota (1-65535)' : 'Informe a porta local (1-65535)'
  }
  if (!t.host.trim()) return 'Informe o servidor SSH'
  if (!portOk(t.port)) return 'Porta SSH inválida (1-65535)'
  if (!t.user.trim()) return 'Informe o login SSH'
  if (t.authMethod === 'key' && !t.keyPath.trim()) return 'Informe a chave privada'
  return ''
}
