import { icon } from './icons.js'

export const $ = (id) => document.getElementById(id)

/** Cria um elemento. Filhos string viram texto (nunca HTML). */
export function h(tag, attrs = {}, ...children) {
  const el = document.createElement(tag)
  for (const [k, v] of Object.entries(attrs)) {
    if (k === 'class') el.className = v
    else if (k.startsWith('on')) el.addEventListener(k.slice(2), v)
    else if (v === true) el.setAttribute(k, '')
    else if (v !== false && v != null) el.setAttribute(k, v)
  }
  for (const c of children.flat(Infinity)) {
    if (c == null || c === false) continue
    el.append(c instanceof Node ? c : document.createTextNode(String(c)))
  }
  return el
}

export { icon }

let toastTimer
export function toast(msg, isErr = false) {
  const t = $('toast')
  t.textContent = String(msg).replace(/^Error: /, '')
  t.className = isErr ? 'err' : ''
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => t.classList.add('hidden'), isErr ? 7000 : 3000)
}

/** O Wails v3 pode entregar o payload direto ou dentro de { data }. Aceita os dois. */
export const payload = (ev) => (ev !== null && typeof ev === 'object' && 'data' in ev ? ev.data : ev) ?? ''

export function b64ToBytes(b64) {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

export function fmtSize(n) {
  if (n < 1024) return n + ' B'
  const u = ['KB', 'MB', 'GB', 'TB']
  let i = -1
  do { n /= 1024; i++ } while (n >= 1024 && i < u.length - 1)
  return n.toFixed(n < 10 ? 1 : 0) + ' ' + u[i]
}

/** bytes/s -> "0,81 Mb/s" (megabits, como o MobaXterm mostra). */
export function fmtRate(bytesPerSec) {
  const mbit = (bytesPerSec * 8) / 1e6
  return mbit.toFixed(2).replace('.', ',') + ' Mb/s'
}

export function fmtUptime(sec) {
  const d = Math.floor(sec / 86400)
  if (d >= 1) return d + (d === 1 ? ' dia' : ' dias')
  const hh = Math.floor(sec / 3600)
  const mm = Math.floor((sec % 3600) / 60)
  return hh + 'h' + String(mm).padStart(2, '0')
}

export const joinPath = (dir, name) => (dir.endsWith('/') ? dir : dir + '/') + name
export const parentPath = (p) => p.replace(/\/[^/]+\/?$/, '') || '/'
export const cleanErr = (e) => String(e && e.message ? e.message : e).replace(/^Error: /, '')

// ------------------------------------------------------------------ modais
export function openModal(content, cls = '') {
  const ov = $('overlay')
  ov.replaceChildren(h('div', { class: 'modal ' + cls }, content))
  ov.classList.remove('hidden')
}
export function closeModal() {
  closeDropdown()
  $('overlay').classList.add('hidden')
  $('overlay').replaceChildren()
}
export const modalOpen = () => !$('overlay').classList.contains('hidden')

/** Pede um texto (senha ou nome). Resolve null se cancelar. */
export function promptBox({ title, label, secret = false, value = '', ok = 'OK', remember = '' }) {
  return new Promise((resolve) => {
    const input = h('input', { type: secret ? 'password' : 'text', value, spellcheck: 'false' })
    const keep = remember ? h('input', { type: 'checkbox' }) : null
    // com "remember", devolve { value, remember }; sem ele, só o texto
    const out = (v) => (keep ? { value: v, remember: keep.checked } : v)
    const done = (v) => { closeModal(); resolve(v === null ? null : out(v)) }
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') done(input.value)
      if (e.key === 'Escape') done(null)
    })
    openModal([
      h('h2', {}, title),
      h('label', {}, label),
      input,
      keep ? h('label', { class: 'check' }, keep, ' ' + remember) : null,
      h('div', { class: 'btns' },
        h('button', { onclick: () => done(null) }, 'Cancelar'),
        h('button', { class: 'primary', onclick: () => done(input.value) }, ok)),
    ])
    input.focus()
  })
}

export function confirmBox(title, text, ok = 'Excluir') {
  return new Promise((resolve) => {
    const done = (v) => { closeModal(); resolve(v) }
    openModal([
      h('h2', {}, title),
      h('p', {}, text),
      h('div', { class: 'btns' },
        h('button', { onclick: () => done(false) }, 'Cancelar'),
        h('button', { class: 'danger', onclick: () => done(true) }, ok)),
    ])
  })
}

// ------------------------------------------------------------------ menus (barra e botão direito)
let menuEl = null
let menuCleanup = null

export function closeMenu() {
  if (menuEl) menuEl.remove()
  menuEl = null
  if (menuCleanup) menuCleanup()
  menuCleanup = null
}

/**
 * Mostra um menu suspenso em (x, y).
 * items: { label, onclick, kbd, checked, danger, disabled, icon } ou { sep: true }
 * onClose é chamado quando o menu fecha (a barra de menus usa para saber o estado).
 */
export function showMenu(x, y, items, onClose) {
  closeMenu()
  const el = h('div', { class: 'menu', role: 'menu' })
  for (const it of items) {
    if (it.sep) { el.append(h('div', { class: 'menu-sep' })); continue }
    const row = h('div', {
      class: 'menu-item' + (it.disabled ? ' disabled' : '') + (it.danger ? ' danger' : ''),
      role: 'menuitem',
      onclick: (e) => {
        e.stopPropagation()
        if (it.disabled) return
        closeMenu()
        if (it.onclick) it.onclick()
      },
    },
    h('span', { class: 'mi-check' }, it.checked ? '✓' : ''),
    it.icon ? h('span', { class: 'mi-ico' }, icon(it.icon, 15)) : null,
    h('span', { class: 'mi-label' }, it.label),
    it.kbd ? h('span', { class: 'mi-kbd' }, it.kbd) : null)
    el.append(row)
  }
  document.body.append(el)
  const r = el.getBoundingClientRect()
  el.style.left = Math.max(0, Math.min(x, window.innerWidth - r.width - 2)) + 'px'
  el.style.top = Math.max(0, Math.min(y, window.innerHeight - r.height - 2)) + 'px'
  menuEl = el

  const off = (e) => { if (!el.contains(e.target)) closeMenu() }
  const esc = (e) => { if (e.key === 'Escape') closeMenu() }
  // setTimeout: o clique que abriu o menu não pode fechá-lo
  const t = setTimeout(() => {
    document.addEventListener('mousedown', off, true)
    document.addEventListener('keydown', esc, true)
    window.addEventListener('blur', closeMenu)
  }, 0)
  menuCleanup = () => {
    clearTimeout(t)
    document.removeEventListener('mousedown', off, true)
    document.removeEventListener('keydown', esc, true)
    window.removeEventListener('blur', closeMenu)
    if (onClose) onClose()
  }
}

// ------------------------------------------------------------------ dica ao passar o mouse
let tipEl = null
let tipTimer = null

export function hideTip() {
  clearTimeout(tipTimer)
  if (tipEl) tipEl.classList.remove('show')
}

/** Mostra uma dica (como a do MobaXterm) ao lado do elemento depois de uma pausa curta. build() devolve os nós. */
export function attachTip(el, build) {
  el.addEventListener('mouseenter', () => {
    clearTimeout(tipTimer)
    tipTimer = setTimeout(() => {
      if (!tipEl) { tipEl = h('div', { id: 'tip' }); document.body.append(tipEl) }
      tipEl.replaceChildren(...build().filter(Boolean))
      const r = el.getBoundingClientRect()
      tipEl.style.left = '0px'
      tipEl.style.top = '0px'
      const tw = tipEl.offsetWidth
      const th = tipEl.offsetHeight
      tipEl.style.left = Math.max(4, Math.min(r.right + 10, window.innerWidth - tw - 4)) + 'px'
      tipEl.style.top = Math.max(4, Math.min(r.top - 2, window.innerHeight - th - 4)) + 'px'
      tipEl.classList.add('show')
    }, 350)
  })
  el.addEventListener('mouseleave', hideTip)
  el.addEventListener('mousedown', hideTip)
  el.addEventListener('contextmenu', hideTip)
}

// ------------------------------------------------------------------ listas suspensas
// Todas as listas do app (selects e campos com sugestões) usam este mesmo visual.
let ddList = null
let ddOwner = null
export const comboOpen = () => !!ddList

export function closeDropdown() {
  if (ddList) { ddList.remove(); ddList = null; ddOwner = null }
  document.removeEventListener('mousedown', onDocDown, true)
  window.removeEventListener('resize', closeDropdown)
}
function onDocDown(e) {
  if (ddList && !ddList.contains(e.target) && e.target !== ddOwner) closeDropdown()
}

/**
 * Abre a lista sob o elemento "anchor". items: [{ value, label, disabled }].
 * Devolve controles para o teclado: { move(delta), pick() }.
 */
function openDropdown(anchor, items, current, onChoose) {
  closeDropdown()
  if (!items.length) return null
  const r = anchor.getBoundingClientRect()
  const list = h('div', { class: 'dd-list', style: `left:${r.left}px;width:${r.width}px` })
  let idx = Math.max(0, items.findIndex((i) => i.value === current))
  const rows = items.map((it, i) => h('div', {
    class: 'dd-item' + (it.value === current ? ' sel' : '') + (it.disabled ? ' off' : ''),
    onmousedown: (e) => { e.preventDefault(); if (!it.disabled) { closeDropdown(); onChoose(it.value) } },
    onmousemove: () => { idx = i; mark() },
  }, it.label))
  list.append(...rows)
  const mark = () => {
    rows.forEach((el, i) => el.classList.toggle('active', i === idx))
    rows[idx]?.scrollIntoView({ block: 'nearest' })
  }
  document.body.append(list)
  const below = window.innerHeight - r.bottom
  const need = Math.min(list.offsetHeight, 240)
  if (below < need + 12 && r.top > below) list.style.bottom = `${window.innerHeight - r.top + 4}px`
  else list.style.top = `${r.bottom + 4}px`
  ddList = list
  ddOwner = anchor
  mark()
  document.addEventListener('mousedown', onDocDown, true)
  window.addEventListener('resize', closeDropdown)
  return {
    move(d) { idx = (idx + d + items.length) % items.length; mark() },
    pick() { const it = items[idx]; if (it && !it.disabled) { closeDropdown(); onChoose(it.value) } },
  }
}

/** <select> nativo com a lista do app: o elemento continua sendo o mesmo (value e change funcionam). */
let selCtl = null
function openSelect(sel) {
  const items = [...sel.options].map((o) => ({ value: o.value, label: o.textContent, disabled: o.disabled }))
  selCtl = openDropdown(sel, items, sel.value, (v) => {
    sel.value = v
    sel.dispatchEvent(new Event('change', { bubbles: true }))
    sel.focus()
  })
}
document.addEventListener('mousedown', (e) => {
  const sel = e.target instanceof Element ? e.target.closest('select') : null
  if (!sel || sel.disabled) return
  e.preventDefault()
  sel.focus()
  ddOwner === sel ? closeDropdown() : openSelect(sel)
}, true)
document.addEventListener('keydown', (e) => {
  const sel = e.target instanceof Element && e.target.tagName === 'SELECT' ? e.target : null
  if (!sel) return
  if (ddOwner === sel && selCtl) {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); selCtl.move(e.key === 'ArrowDown' ? 1 : -1) }
    else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selCtl.pick() }
    else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); closeDropdown() }
    else if (e.key === 'Tab') closeDropdown()
  } else if (e.key === 'Enter' || e.key === ' ' || (e.altKey && e.key === 'ArrowDown') || e.key === 'F4') {
    e.preventDefault(); openSelect(sel)
  }
}, true)

/**
 * Campo de texto com sugestões: abre ao clicar ou focar no campo, filtra enquanto digita e aceita texto livre.
 * getItems devolve as sugestões (array ou Promise) e é consultado a cada abertura.
 */
export function combo(input, getItems) {
  input.classList.add('dd')
  input.setAttribute('autocomplete', 'off')
  let ctl = null
  let seq = 0
  const open = async (all) => {
    const mine = ++seq
    let items
    try { items = (await getItems()) || [] } catch { items = [] }
    if (mine !== seq || document.activeElement !== input) return
    const q = all ? '' : input.value.trim().toLowerCase()
    const shown = items.map(String).filter((v) => !q || v.toLowerCase().includes(q))
    ctl = openDropdown(input, shown.map((v) => ({ value: v, label: v })), input.value, (v) => {
      input.value = v
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
  }
  input.addEventListener('mousedown', () => { if (ddOwner !== input) setTimeout(() => open(true), 0) })
  input.addEventListener('focus', () => open(true))
  input.addEventListener('input', () => { if (ddOwner === input) open(false) })
  input.addEventListener('keydown', (e) => {
    const isOpen = ddOwner === input && ctl
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      isOpen ? ctl.move(e.key === 'ArrowDown' ? 1 : -1) : open(true)
    } else if (e.key === 'Enter' && isOpen) {
      e.preventDefault(); e.stopPropagation(); ctl.pick()
    } else if (e.key === 'Escape' && isOpen) {
      e.stopPropagation(); closeDropdown()
    } else if (e.key === 'Tab') closeDropdown()
  }, true)
  input.addEventListener('blur', () => setTimeout(() => { if (ddOwner === input) closeDropdown() }, 120))
  return input
}
