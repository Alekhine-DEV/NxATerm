// Sincronização: envia e baixa sessões, pastas, macros e configurações (GitHub Gist, WebDAV ou servidor próprio).
import { Api } from './api.js'
import { h, toast, cleanErr, openModal, closeModal, confirmBox } from './util.js'
import { settings, saveSettings, normalizeSettings } from './settings.js'
import { applyAppearance } from './uitheme.js'

// Só estas configurações viajam na cópia. Ficam de fora: bloqueio, bandeja, atalho global, proxy, editor,
// imagem de fundo e a própria sincronização — são do computador, não da conta.
const SYNC_KEYS = [
  'font', 'fontSize', 'theme', 'cursorStyle', 'cursorBlink', 'scrollback', 'copyOnSelect', 'rightClick',
  'autoSftp', 'monitoring', 'followFolder', 'termSync', 'uiTheme', 'uiThemes', 'customThemes', 'customCss',
  'termOpacity', 'unicode11', 'inlineImages', 'zmodem', 'trzsz', 'ligatures', 'highlight', 'keys', 'macroScope',
]

// Segredos: só na memória (e, se o usuário pedir, num arquivo local só dele).
const secrets = { token: '', password: '', passcode: '' }
let secretsLoaded = false

async function loadSecrets() {
  if (secretsLoaded) return
  secretsLoaded = true
  if (!settings.sync.remember) return
  try {
    const c = await Api.SyncLoadSecrets()
    secrets.token = c.token || ''
    secrets.password = c.password || ''
    secrets.passcode = c.passcode || ''
  } catch { /* sem segredos guardados */ }
}

function config() {
  const sy = settings.sync
  return {
    provider: sy.provider,
    token: secrets.token,
    gistId: sy.gistId,
    url: sy.url,
    user: sy.user,
    password: secrets.password,
    passcode: sy.encrypt ? secrets.passcode : '',
  }
}

function problem() {
  const sy = settings.sync
  if (sy.provider === 'gist' && !secrets.token) return 'Informe o token do GitHub.'
  if (sy.provider !== 'gist' && !/^https?:\/\//i.test(sy.url)) return 'Informe o endereço (http:// ou https://).'
  if (sy.provider === 'webdav' && !sy.user) return 'Informe o usuário.'
  if (sy.encrypt && !secrets.passcode) return 'Informe a senha da criptografia.'
  return ''
}

function pickSettings() {
  const o = {}
  for (const k of SYNC_KEYS) o[k] = settings[k]
  return JSON.stringify(o)
}

function applySettings(json) {
  let o
  try { o = JSON.parse(json) } catch { return false }
  if (!o || typeof o !== 'object' || Array.isArray(o)) return false
  for (const k of SYNC_KEYS) if (k in o) settings[k] = o[k]
  normalizeSettings()
  saveSettings()
  applyAppearance()
  return true
}

async function persistSecrets() {
  try {
    if (settings.sync.remember) await Api.SyncSaveSecrets({ ...config(), passcode: secrets.passcode })
    else await Api.SyncForgetSecrets()
  } catch (e) { toast('Não foi possível guardar os dados: ' + cleanErr(e), true) }
}

async function push() {
  const sum = await Api.SyncPush(config(), settings.sync.withSettings ? pickSettings() : '{}')
  if (sum.gistId && sum.gistId !== settings.sync.gistId) { settings.sync.gistId = sum.gistId }
  saveSettings()
  await persistSecrets()
  return `Enviado: ${sum.sessions} sessão(ões) e ${sum.macros} macro(s)`
}

async function pull(mode, withSettings, hooks) {
  const sum = await Api.SyncPull(config(), mode)
  if (sum.empty) return 'Ainda não há cópia nesse destino.'
  let extra = ''
  if (withSettings && sum.settings && sum.settings !== 'null' && applySettings(sum.settings)) extra = ' e configurações'
  await persistSecrets()
  await hooks.onData?.()
  if (extra) hooks.onSettings?.()
  if (sum.removed > 0) toast(`${sum.removed} item(ns) apagado(s) aqui porque foram apagados em outro computador`)
  return mode === 'merge'
    ? `Baixado: ${sum.sessions} sessão(ões) e ${sum.macros} macro(s) novas${extra}`
    : `Substituído: ${sum.sessions} sessão(ões) e ${sum.macros} macro(s)${extra}`
}

/** Ao abrir o app: junta o que há de novo na cópia (só com os dados guardados neste computador). */
export async function syncOnOpen(hooks) {
  const sy = settings.sync
  if (!sy.onOpen || !sy.remember) return
  await loadSecrets()
  if (problem()) return
  try {
    const msg = await pull('merge', false, hooks)
    if (!/Ainda não/.test(msg)) toast(msg)
  } catch (e) { toast('Sincronização: ' + cleanErr(e), true) }
}

const field = (label, el) => [h('label', {}, label), el]

export async function syncDialog(hooks) {
  await loadSecrets()
  const sy = settings.sync
  const box = h('div', { class: 'sync-fields' })
  const buttons = []
  let busy = false

  const text = (get, set, opts = {}) => {
    const el = h('input', { type: opts.secret ? 'password' : 'text', value: get(), spellcheck: 'false', autocomplete: 'off', placeholder: opts.ph || '' })
    el.addEventListener('input', () => set(el.value))
    return el
  }
  const check = (label, get, set) => {
    const c = h('input', { type: 'checkbox', checked: get() })
    c.addEventListener('change', () => { set(c.checked); draw() })
    return h('label', { class: 'check' }, c, label)
  }

  const draw = () => {
    const rows = []
    if (sy.provider === 'gist') {
      rows.push(...field('Token do GitHub', text(() => secrets.token, (v) => { secrets.token = v.trim() }, { secret: true })))
      rows.push(...field('ID do Gist', text(() => sy.gistId, (v) => { sy.gistId = v.trim() })))
    } else {
      rows.push(...field('Endereço', text(() => sy.url, (v) => { sy.url = v.trim() }, { ph: 'https://' })))
      if (sy.provider === 'webdav') {
        rows.push(...field('Usuário', text(() => sy.user, (v) => { sy.user = v })))
        rows.push(...field('Senha', text(() => secrets.password, (v) => { secrets.password = v }, { secret: true })))
      } else {
        rows.push(...field('Token', text(() => secrets.token, (v) => { secrets.token = v.trim() }, { secret: true })))
      }
    }
    rows.push(h('div', { class: 'sync-checks' },
      check('Criptografar a cópia com senha', () => sy.encrypt, (v) => { sy.encrypt = v }),
      sy.encrypt ? text(() => secrets.passcode, (v) => { secrets.passcode = v }, { secret: true, ph: 'Senha da criptografia' }) : null,
      check('Incluir configurações', () => sy.withSettings, (v) => { sy.withSettings = v }),
      check('Guardar estes dados neste computador', () => sy.remember, (v) => { sy.remember = v; if (!v) sy.onOpen = false }),
      sy.remember ? check('Mesclar ao abrir o NxATerm', () => sy.onOpen, (v) => { sy.onOpen = v }) : null))
    box.replaceChildren(...rows)
  }

  const provider = h('select', {},
    ...[['gist', 'GitHub Gist'], ['webdav', 'WebDAV'], ['custom', 'Servidor próprio']]
      .map(([v, l]) => h('option', { value: v, selected: v === sy.provider }, l)))
  provider.addEventListener('change', () => { sy.provider = provider.value; draw() })

  const run = (fn) => async () => {
    if (busy) return
    const p = problem()
    if (p) { toast(p, true); return }
    busy = true
    buttons.forEach((b) => { b.disabled = true })
    try { toast(await fn()) } catch (e) { toast(cleanErr(e), true) }
    busy = false
    buttons.forEach((b) => { b.disabled = false })
    draw()
    saveSettings()
  }
  const btn = (label, fn, cls = '') => {
    const b = h('button', { class: cls, onclick: fn }, label)
    buttons.push(b)
    return b
  }

  draw()
  openModal([
    h('h2', {}, 'Sincronização'),
    ...field('Destino', provider),
    box,
    h('div', { class: 'btns' },
      h('button', { onclick: () => { saveSettings(); persistSecrets(); closeModal() } }, 'Fechar'),
      h('span', { class: 'spacer' }),
      btn('Baixar e substituir', run(async () => {
        if (!(await confirmBox('Substituir tudo', 'As sessões e macros deste computador serão trocadas pelas da cópia.', 'Substituir'))) { syncDialog(hooks); return 'Cancelado' }
        return pull('replace', sy.withSettings, hooks)
      })),
      btn('Baixar e mesclar', run(() => pull('merge', sy.withSettings, hooks))),
      btn('Enviar', run(push), 'primary')),
  ], 'sync-modal')
}
