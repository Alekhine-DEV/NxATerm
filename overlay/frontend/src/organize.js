// Histórico de conexões, espaços de trabalho e senha de abertura.
import { Api } from './api.js'
import { h, icon, toast, cleanErr, openModal, closeModal, confirmBox, promptBox } from './util.js'
import { settings, saveSettings } from './settings.js'
import { TYPE_INFO, typeOf, sessionSummary } from './sessiontypes.js'

const field = (label, el) => [h('label', {}, label), el]

// ------------------------------------------------------------------ histórico
const histKey = (s) => [s.type || 'ssh', s.host, s.user, s.port, s.serialPort, s.shell, s.distro, s.name].join('|')

/** Registra uma conexão no histórico (as mais recentes primeiro, sem repetir). */
export function recordHistory(s) {
  const snap = { ...s }
  const key = histKey(snap)
  settings.history = [{ at: Date.now(), s: snap }, ...(settings.history || []).filter((e) => histKey(e.s) !== key)].slice(0, 50)
  saveSettings()
}

function fmtWhen(ms) {
  const d = new Date(ms)
  const pad = (n) => String(n).padStart(2, '0')
  const today = new Date()
  const same = d.toDateString() === today.toDateString()
  return (same ? 'Hoje' : `${pad(d.getDate())}/${pad(d.getMonth() + 1)}/${d.getFullYear()}`) + ` ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function historyDialog(openEntry) {
  const list = h('div', { class: 'org-list' })
  const render = () => {
    const items = settings.history || []
    if (!items.length) { list.replaceChildren(h('div', { class: 'sftp-msg' }, 'Nenhuma conexão ainda')); return }
    list.replaceChildren(...items.map((e) => {
      const info = TYPE_INFO[typeOf(e.s)] || TYPE_INFO.ssh
      return h('div', { class: 'org-row', onclick: () => { closeModal(); openEntry(e.s) } },
        h('span', { class: 'sess-ico', style: `color:${info.color}` }, icon(info.icon, 15)),
        h('span', { class: 'org-name' }, e.s.name || sessionSummary(e.s)),
        h('span', { class: 'org-sub' }, fmtWhen(e.at)))
    }))
  }
  render()
  openModal([
    h('h2', {}, 'Histórico de conexões'),
    list,
    h('div', { class: 'btns' },
      h('button', {
        class: 'danger',
        onclick: () => { settings.history = []; saveSettings(); render() },
      }, 'Limpar'),
      h('span', { class: 'spacer' }),
      h('button', { onclick: closeModal }, 'Fechar')),
  ], 'wide')
}

// ------------------------------------------------------------------ espaços de trabalho
/** getState() -> { items: [session...], layout }; apply({items, layout}) abre tudo. */
export function workspaceDialog(getState, apply) {
  const list = h('div', { class: 'org-list' })
  const names = () => Object.keys(settings.workspaces || {}).sort((a, b) => a.localeCompare(b, 'pt-BR'))
  const render = () => {
    const ns = names()
    if (!ns.length) { list.replaceChildren(h('div', { class: 'sftp-msg' }, 'Nenhum espaço de trabalho salvo')); return }
    list.replaceChildren(...ns.map((n) => {
      const w = settings.workspaces[n]
      return h('div', { class: 'org-row', onclick: () => { closeModal(); apply(w) } },
        h('span', { class: 'org-name' }, n),
        h('span', { class: 'org-sub' }, `${w.items.length} aba${w.items.length === 1 ? '' : 's'}`),
        h('button', {
          class: 'ib', title: 'Excluir',
          onclick: async (e) => {
            e.stopPropagation()
            delete settings.workspaces[n]
            saveSettings()
            render()
          },
        }, icon('trash', 14)))
    }))
  }
  render()
  const saveCurrent = async () => {
    const st = getState()
    if (!st.items.length) { toast('Não há abas abertas para salvar.', true); return }
    const name = await promptBox({ title: 'Salvar espaço de trabalho', label: 'Nome', ok: 'Salvar' })
    if (!name || !name.trim()) { workspaceDialog(getState, apply); return }
    settings.workspaces[name.trim()] = { items: st.items, layout: st.layout }
    saveSettings()
    toast(`Espaço "${name.trim()}" salvo`)
    workspaceDialog(getState, apply)
  }
  openModal([
    h('h2', {}, 'Espaços de trabalho'),
    list,
    h('div', { class: 'btns' },
      h('button', { onclick: saveCurrent }, 'Salvar abas atuais...'),
      h('span', { class: 'spacer' }),
      h('button', { onclick: closeModal }, 'Fechar')),
  ], 'wide')
}

// ------------------------------------------------------------------ senha de abertura
/** Se houver senha de abertura, só volta depois que ela for digitada. */
export async function lockGate() {
  let enabled = false
  try { enabled = await Api.HasLock() } catch { return }
  if (!enabled) return
  await new Promise((resolve) => {
    const input = h('input', { type: 'password', autocomplete: 'off', spellcheck: 'false', placeholder: 'Senha' })
    const err = h('div', { class: 'lock-err' })
    const btn = h('button', { class: 'primary' }, 'Desbloquear')
    const box = h('div', { id: 'lock-screen' },
      h('div', { class: 'lock-box' },
        h('div', { class: 'home-logo' }, icon('terminal', 30), h('span', {}, 'NxATerm')),
        input, err, btn))
    let busy = false
    const go = async () => {
      if (busy) return
      busy = true
      btn.disabled = true
      let ok = false
      try { ok = await Api.CheckLock(input.value) } catch (e) { err.textContent = cleanErr(e) }
      busy = false
      btn.disabled = false
      if (ok) { box.remove(); resolve(); return }
      if (!err.textContent) err.textContent = 'Senha incorreta'
      input.select()
    }
    btn.addEventListener('click', go)
    input.addEventListener('keydown', (e) => { if (e.key === 'Enter') go() })
    input.addEventListener('input', () => { err.textContent = '' })
    document.body.append(box)
    input.focus()
  })
}

export function lockDialog(onDone) {
  Api.HasLock().then((enabled) => {
    const cur = h('input', { type: 'password', autocomplete: 'off' })
    const a = h('input', { type: 'password', autocomplete: 'off' })
    const b = h('input', { type: 'password', autocomplete: 'off' })
    const save = async () => {
      if (a.value !== b.value) { toast('As senhas não conferem', true); return }
      if (!a.value) { toast('Digite a nova senha', true); return }
      try { await Api.SetLock(cur.value, a.value); toast('Senha de abertura definida'); closeModal(); onDone() } catch (e) { toast(cleanErr(e), true) }
    }
    const remove = async () => {
      try { await Api.SetLock(cur.value, ''); toast('Senha de abertura removida'); closeModal(); onDone() } catch (e) { toast(cleanErr(e), true) }
    }
    openModal([
      h('h2', {}, 'Senha de abertura'),
      enabled ? field('Senha atual', cur) : null,
      field('Nova senha', a),
      field('Repita a nova senha', b),
      h('div', { class: 'btns' },
        enabled ? h('button', { class: 'danger', onclick: remove }, 'Remover') : null,
        h('span', { class: 'spacer' }),
        h('button', { onclick: () => { closeModal(); onDone() } }, 'Cancelar'),
        h('button', { class: 'primary', onclick: save }, 'Salvar')),
    ])
    ;(enabled ? cur : a).focus()
  }).catch((e) => toast(cleanErr(e), true))
}
