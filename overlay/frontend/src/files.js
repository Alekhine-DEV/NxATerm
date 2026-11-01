// Navegador de arquivos: painel remoto (SFTP) e painel local, seleção múltipla, arrastar e soltar,
// menus, permissões, propriedades, área de transferência de arquivos e a gaveta de transferências.
import * as Runtime from '@wailsio/runtime'
import { Api } from './api.js'
import {
  $, h, icon, toast, payload, fmtSize, joinPath, parentPath, cleanErr, openModal, closeModal, modalOpen,
  promptBox, confirmBox, showMenu, combo,
} from './util.js'
import { settings, saveSettings } from './settings.js'

const Events = Runtime.Events
const MAX_EDIT = 50 * 1024 * 1024

let host = null // { getTab, activeTab, reconnectTab, copyText }

// ------------------------------------------------------------------ caminhos locais
const lsep = (p) => (/^[A-Za-z]:/.test(p) || p.includes('\\') ? '\\' : '/')
const localJoin = (dir, name) => (dir.endsWith('\\') || dir.endsWith('/') ? dir : dir + lsep(dir)) + name
function localParent(p) {
  if (/^[A-Za-z]:\\?$/.test(p) || p === '/') return p
  const s = lsep(p)
  const t = p.endsWith(s) ? p.slice(0, -1) : p
  const i = t.lastIndexOf(s)
  if (i < 0) return p
  const up = t.slice(0, i)
  if (/^[A-Za-z]:$/.test(up)) return up + '\\'
  return up || '/'
}
const shellQ = (p) => "'" + p.replace(/'/g, "'\\''") + "'"
const fmtDate = (unix) => (unix ? new Date(unix * 1000).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' }) : '')
const keyOfTab = (tab) => `${tab.session.user || ''}@${tab.session.host}:${tab.session.port}`

// ------------------------------------------------------------------ área de transferência de arquivos
let clip = null // { mode: 'copy'|'cut', side, tabId, dir, names, paths }

// ------------------------------------------------------------------ painel
const COLS = [
  { key: 'name', label: 'Nome' },
  { key: 'size', label: 'Tamanho' },
  { key: 'date', label: 'Modificado' },
  { key: 'perm', label: 'Permissões' },
]

/**
 * Cria um painel de arquivos. side: 'remote' | 'local'. wide: mostra as colunas de data e permissões.
 * peer(): devolve o painel do outro lado (para enviar/baixar), se existir.
 */
export function createPane(side, tab, { wide = false, peer = () => null } = {}) {
  const remote = side === 'remote'
  const st = {
    path: '', entries: [], sel: new Set(), anchor: null, loading: false, err: '', ready: false,
    hidden: true, sort: { key: 'name', dir: 1 }, filter: '', recent: [],
  }
  const rows = new Map()
  const p = { side, tab, st, el: null, load, refresh, peer, render }

  const joinP = remote ? joinPath : localJoin
  const parentP = remote ? parentPath : localParent
  const list = (path) => (remote ? Api.SFTPList(tab.id, path) : Api.LocalList(path))
  const dead = () => remote && (tab.dead || !tab.ready)
  const ftp = remote && tab.kind === 'ftp'

  // ---- elementos
  const pathEl = h('input', { class: 'fp-path', type: 'text', spellcheck: 'false', placeholder: remote ? '/' : '' })
  const listEl = h('div', { class: 'fp-list', tabindex: '0', 'data-file-drop-target': true, id: `drop-${remote ? 'r' : 'l'}-${tab.id}` })
  const footEl = h('span', { class: 'fp-count' })
  const filterEl = h('input', { class: 'fp-filter', type: 'text', placeholder: 'Filtrar', spellcheck: 'false' })
  const colsEl = wide ? h('div', { class: 'fp-cols' }, h('span', { class: 'fi' }),
    ...COLS.filter((c) => remote || c.key !== 'perm').map((c) => h('span', {
      class: 'c-' + c.key + ' sortable', onclick: () => sortBy(c.key),
    }, c.label, h('i', { class: 'sortmark' })))) : null
  let driveSel = null

  const bar = h('div', { class: 'iconbar' })
  const hiddenBtn = btn('view', 'Mostrar arquivos ocultos', () => { st.hidden = !st.hidden; render() })
  const favBtn = btn('star', 'Adicionar aos favoritos', toggleFav)
  const syncBtn = btn('sync', 'Levar o terminal junto ao navegar aqui', () => {
    settings.termSync = !settings.termSync
    saveSettings()
    syncBtn.classList.toggle('on', settings.termSync)
  })
  syncBtn.classList.toggle('on', !!settings.termSync)

  function btn(ico, title, fn) {
    return h('button', { class: 'ib', title, onclick: (e) => { e.preventDefault(); fn() } }, icon(ico, 17))
  }
  const needAlive = (fn) => () => {
    if (dead()) return toast('A conexão não está ativa.', true)
    fn()
  }
  const buttons = remote
    ? [
        btn('up', 'Pasta acima', needAlive(() => load(parentP(st.path || '/')))),
        btn('refresh', 'Atualizar', needAlive(refresh)),
        btn('home', 'Pasta inicial', needAlive(() => load(''))),
        btn('upload', 'Enviar arquivos para esta pasta', needAlive(() => pickUpload(false))),
        btn('uploaddir', 'Enviar uma pasta para cá', needAlive(() => pickUpload(true))),
        btn('download', 'Baixar a seleção', needAlive(downloadSel)),
        btn('newfile', 'Novo arquivo', needAlive(mkfile)),
        btn('newfolder', 'Nova pasta', needAlive(mkdir)),
        btn('pencil', 'Renomear', needAlive(renameSel)),
        btn('trash', 'Excluir a seleção', needAlive(deleteSel)),
        hiddenBtn,
        ...(tab.kind === 'ssh' ? [syncBtn] : []),
        favBtn,
      ]
    : [
        btn('up', 'Pasta acima', () => load(parentP(st.path))),
        btn('refresh', 'Atualizar', refresh),
        btn('home', 'Pasta pessoal', () => load('')),
        btn('upload', 'Enviar a seleção para o servidor', uploadSel),
        btn('newfile', 'Novo arquivo', mkfile),
        btn('newfolder', 'Nova pasta', mkdir),
        btn('pencil', 'Renomear', renameSel),
        btn('trash', 'Excluir a seleção', deleteSel),
        hiddenBtn,
        favBtn,
      ]
  bar.append(...buttons)

  pathEl.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); load(pathEl.value.trim()) } })
  combo(pathEl, () => favsOf().concat(st.recent.filter((x) => !favsOf().includes(x))))
  pathEl.addEventListener('change', () => { if (document.activeElement !== pathEl) return })
  filterEl.addEventListener('input', () => { st.filter = filterEl.value.trim().toLowerCase(); render() })

  const pathRow = h('div', { class: 'fp-addr' })
  if (!remote) {
    Api.LocalDrives().then((d) => {
      if (!d || d.length < 2) return
      driveSel = h('select', { class: 'fp-drive', title: 'Unidade' }, ...d.map((x) => h('option', { value: x }, x)))
      driveSel.addEventListener('change', () => load(driveSel.value))
      pathRow.prepend(driveSel)
    }).catch(() => {})
  }
  pathRow.append(pathEl)

  p.el = h('div', { class: 'fpane ' + side + (wide ? ' wide' : '') },
    h('div', { class: 'fp-bar' }, bar), pathRow, colsEl,
    listEl,
    h('div', { class: 'fp-foot' }, footEl, filterEl))

  // ---- favoritos
  function favsOf() {
    const k = remote ? keyOfTab(tab) : 'local'
    return (settings.pathFavs && settings.pathFavs[k]) || []
  }
  function toggleFav() {
    if (!st.path) return
    if (!settings.pathFavs || typeof settings.pathFavs !== 'object') settings.pathFavs = {}
    const k = remote ? keyOfTab(tab) : 'local'
    const cur = settings.pathFavs[k] || []
    settings.pathFavs[k] = cur.includes(st.path) ? cur.filter((x) => x !== st.path) : [...cur, st.path]
    saveSettings()
    markFav()
    toast(cur.includes(st.path) ? 'Removido dos favoritos' : 'Adicionado aos favoritos')
  }
  function markFav() {
    favBtn.classList.toggle('on', favsOf().includes(st.path))
  }

  // ---- carregar
  async function load(path, opts = {}) {
    if (dead()) { render(); return }
    st.loading = true
    st.err = ''
    render()
    try {
      const res = await list(path)
      st.path = res.path
      st.entries = res.entries || []
      st.sel = new Set()
      st.anchor = null
      st.ready = true
      st.recent = [res.path, ...st.recent.filter((x) => x !== res.path)].slice(0, 12)
      if (driveSel && /^[A-Za-z]:/.test(res.path)) driveSel.value = res.path.slice(0, 3).toUpperCase()
      if (remote && tab.kind === 'ssh' && settings.termSync && !opts.fromTerm && res.path !== tab.cwd && !tab.dead) {
        Api.Write(tab.id, ' cd -- ' + shellQ(res.path) + '\r').catch(() => {})
        tab.cwd = res.path
      }
    } catch (e) {
      st.err = cleanErr(e)
    }
    st.loading = false
    render()
  }
  function refresh() { return load(st.path) }

  // ---- ordenação e visão
  function sortBy(key) {
    st.sort = st.sort.key === key ? { key, dir: -st.sort.dir } : { key, dir: 1 }
    render()
  }
  function visible() {
    const { key, dir } = st.sort
    const f = st.filter
    const out = st.entries.filter((e) => (st.hidden || !e.name.startsWith('.')) && (!f || e.name.toLowerCase().includes(f)))
    const cmp = {
      name: (a, b) => a.name.localeCompare(b.name, 'pt-BR', { sensitivity: 'base', numeric: true }),
      size: (a, b) => a.size - b.size || cmp.name(a, b),
      date: (a, b) => a.modTime - b.modTime || cmp.name(a, b),
      perm: (a, b) => a.perm - b.perm || cmp.name(a, b),
    }[key]
    return out.sort((a, b) => (a.isDir !== b.isDir ? (a.isDir ? -1 : 1) : cmp(a, b) * dir))
  }
  const byName = (n) => st.entries.find((e) => e.name === n)
  const selEntries = () => [...st.sel].map(byName).filter(Boolean)
  const fullPath = (e) => joinP(st.path, e.name)

  // ---- desenho
  function msg(...c) { return h('div', { class: 'sftp-msg' }, ...c) }
  function render() {
    pathEl.value = st.path
    markFav()
    hiddenBtn.classList.toggle('on', st.hidden)
    if (colsEl) {
      colsEl.querySelectorAll('.sortable').forEach((c) => {
        const k = c.className.match(/c-(\w+)/)[1]
        c.querySelector('.sortmark').textContent = st.sort.key === k ? (st.sort.dir > 0 ? ' ▲' : ' ▼') : ''
      })
    }
    rows.clear()
    if (remote && tab.dead) {
      listEl.replaceChildren(msg('Conexão encerrada' + (tab.deadReason ? ': ' + tab.deadReason : '') + '. ',
        h('button', { class: 'mini-link', onclick: () => host.reconnectTab(tab) }, 'Reconectar')))
      footEl.textContent = ''
      return
    }
    if (remote && !tab.ready) { listEl.replaceChildren(msg('Conectando...')); footEl.textContent = ''; return }
    if (st.loading && !st.entries.length) { listEl.replaceChildren(msg('Carregando...')); return }
    if (st.err) {
      listEl.replaceChildren(msg('Erro: ' + st.err + ' ', h('button', { class: 'mini-link', onclick: refresh }, 'Tentar de novo')))
      footEl.textContent = ''
      return
    }
    const vis = visible()
    listEl.replaceChildren(...(vis.length ? vis.map(row) : [msg(st.entries.length ? 'Nada encontrado' : 'Pasta vazia')]))
    updateFoot()
  }
  function updateFoot() {
    const sel = selEntries()
    const total = st.entries.length
    let t = total + (total === 1 ? ' item' : ' itens')
    if (sel.length) {
      const bytes = sel.reduce((s, e) => s + (e.isDir ? 0 : e.size), 0)
      t += ` · ${sel.length} selecionado${sel.length > 1 ? 's' : ''}` + (bytes ? ' · ' + fmtSize(bytes) : '')
    }
    footEl.textContent = t
  }
  function markSel() {
    for (const [n, el] of rows) el.classList.toggle('sel', st.sel.has(n))
    updateFoot()
  }

  function row(e) {
    const el = h('div', {
      class: 'frow' + (wide ? ' wide' : '') + (e.isDir ? ' dir' : '') + (e.isLink ? ' link' : '') + (st.sel.has(e.name) ? ' sel' : ''),
      title: e.name + (remote ? '\n' + e.mode : ''), draggable: 'true',
    },
    h('span', { class: 'fi' }, icon(e.isDir ? 'folder' : 'file', 15)),
    h('span', { class: 'fn' }, e.name),
    h('span', { class: 'fs' }, e.isDir ? '' : fmtSize(e.size)),
    wide ? h('span', { class: 'fd' }, fmtDate(e.modTime)) : null,
    wide && remote ? h('span', { class: 'fm' }, e.mode) : null)
    rows.set(e.name, el)
    el.addEventListener('mousedown', (ev) => { if (ev.button === 0) pick(ev, e) })
    el.addEventListener('click', (ev) => {
      if (!ev.ctrlKey && !ev.shiftKey && !ev.metaKey && st.sel.size > 1 && st.sel.has(e.name)) { st.sel = new Set([e.name]); st.anchor = e.name; markSel() }
    })
    el.addEventListener('dblclick', () => openEntry(e))
    el.addEventListener('contextmenu', (ev) => {
      ev.preventDefault()
      ev.stopPropagation()
      if (!st.sel.has(e.name)) { st.sel = new Set([e.name]); st.anchor = e.name; markSel() }
      rowMenu(ev.clientX, ev.clientY, e)
    })
    el.addEventListener('dragstart', (ev) => {
      if (!st.sel.has(e.name)) { st.sel = new Set([e.name]); st.anchor = e.name; markSel() }
      const names = [...st.sel]
      ev.dataTransfer.effectAllowed = 'copyMove'
      ev.dataTransfer.setData('application/x-nxaterm', JSON.stringify({ side, tabId: tab.id, dir: st.path, names }))
      ev.dataTransfer.setData('text/plain', names.join('\n'))
    })
    if (e.isDir) wireDrop(el, () => joinP(st.path, e.name), true)
    return el
  }

  function pick(ev, e) {
    listEl.focus({ preventScroll: true })
    if (ev.shiftKey && st.anchor) {
      const names = visible().map((x) => x.name)
      const a = names.indexOf(st.anchor)
      const b = names.indexOf(e.name)
      if (a >= 0 && b >= 0) {
        const [lo, hi] = a < b ? [a, b] : [b, a]
        st.sel = new Set(ev.ctrlKey || ev.metaKey ? [...st.sel, ...names.slice(lo, hi + 1)] : names.slice(lo, hi + 1))
      }
    } else if (ev.ctrlKey || ev.metaKey) {
      st.sel.has(e.name) ? st.sel.delete(e.name) : st.sel.add(e.name)
      st.anchor = e.name
    } else {
      if (st.sel.size === 1 && st.sel.has(e.name)) { st.anchor = e.name; return }
      if (!st.sel.has(e.name) || st.sel.size === 1) st.sel = new Set([e.name])
      st.anchor = e.name
    }
    markSel()
  }
  listEl.addEventListener('mousedown', (ev) => {
    if (ev.target === listEl || ev.target.classList.contains('sftp-msg')) { st.sel = new Set(); st.anchor = null; markSel(); listEl.focus() }
  })
  listEl.addEventListener('contextmenu', (ev) => {
    ev.preventDefault()
    st.sel = new Set()
    markSel()
    bgMenu(ev.clientX, ev.clientY)
  })

  // ---- teclado
  listEl.addEventListener('keydown', (ev) => {
    const mod = ev.ctrlKey || ev.metaKey
    const vis = visible()
    if (mod && ev.key.toLowerCase() === 'a') { ev.preventDefault(); st.sel = new Set(vis.map((x) => x.name)); markSel(); return }
    if (mod && ev.key.toLowerCase() === 'c') { ev.preventDefault(); setClip('copy'); return }
    if (mod && ev.key.toLowerCase() === 'x') { ev.preventDefault(); setClip('cut'); return }
    if (mod && ev.key.toLowerCase() === 'v') { ev.preventDefault(); paste(); return }
    switch (ev.key) {
      case 'ArrowDown': case 'ArrowUp': {
        ev.preventDefault()
        if (!vis.length) return
        const names = vis.map((x) => x.name)
        let i = names.indexOf(st.anchor)
        i = i < 0 ? 0 : Math.max(0, Math.min(names.length - 1, i + (ev.key === 'ArrowDown' ? 1 : -1)))
        const n = names[i]
        if (ev.shiftKey && st.anchor) st.sel.add(n)
        else st.sel = new Set([n])
        st.anchor = n
        markSel()
        rows.get(n)?.scrollIntoView({ block: 'nearest' })
        break
      }
      case 'Enter': { const e = selEntries()[0]; if (e) openEntry(e); break }
      case 'Backspace': ev.preventDefault(); if (!dead()) load(parentP(st.path || '/')); break
      case 'Delete': deleteSel(); break
      case 'F2': renameSel(); break
      case 'F5': ev.preventDefault(); if (!dead()) refresh(); break
      default:
    }
  })

  // ---- soltar arquivos (dentro do app)
  function wireDrop(el, targetDir, isRow) {
    el.addEventListener('dragover', (ev) => {
      if (!ev.dataTransfer.types.includes('application/x-nxaterm')) return
      ev.preventDefault()
      ev.stopPropagation()
      ev.dataTransfer.dropEffect = 'copy'
      el.classList.add('dropping')
    })
    el.addEventListener('dragleave', () => el.classList.remove('dropping'))
    el.addEventListener('drop', (ev) => {
      el.classList.remove('dropping')
      const raw = ev.dataTransfer.getData('application/x-nxaterm')
      if (!raw) return
      ev.preventDefault()
      ev.stopPropagation()
      let src
      try { src = JSON.parse(raw) } catch { return }
      dropOn(src, targetDir(), isRow)
    })
  }
  wireDrop(listEl, () => st.path, false)

  async function dropOn(src, targetDir, onFolder) {
    try {
      if (src.side === 'local' && remote) {
        if (dead()) return
        const paths = src.names.map((n) => localJoin(src.dir, n))
        await Api.Upload(tab.id, paths, targetDir)
      } else if (src.side === 'remote' && !remote) {
        const t = host.getTab(src.tabId)
        if (!t || t.dead) return toast('A conexão de origem não está ativa.', true)
        await Api.Download(src.tabId, src.names.map((n) => joinPath(src.dir, n)), targetDir)
      } else if (src.side === 'remote' && remote) {
        if (src.tabId !== tab.id) return toast('Para copiar entre servidores, baixe e envie de novo.', true)
        if (!onFolder || src.dir === targetDir) return
        await Api.SFTPMove(tab.id, src.names.map((n) => joinPath(src.dir, n)), targetDir)
        toast('Movido')
        refresh()
        return
      } else if (src.side === 'local' && !remote) {
        if (!onFolder || src.dir === targetDir) return
        await Api.LocalMove(src.names.map((n) => localJoin(src.dir, n)), targetDir)
        toast('Movido')
        refresh()
        return
      }
    } catch (e) { toast(cleanErr(e), true) }
  }

  // ---- ações
  function openEntry(e) {
    if (e.isDir) return load(fullPath(e))
    if (remote) return editFile(e)
    Api.LocalOpen(fullPath(e)).catch((err) => toast(cleanErr(err), true))
  }
  async function editFile(e) {
    if (dead()) return
    if (e.size > MAX_EDIT) return downloadSel()
    try {
      await Api.EditRemote(tab.id, fullPath(e))
      toast('Aberto para edição; ao salvar, o arquivo volta para o servidor')
    } catch (err) { toast(cleanErr(err), true) }
  }
  async function pickUpload(dirs) {
    try { await Api.PickUpload(tab.id, st.path, dirs) } catch (e) { toast(cleanErr(e), true) }
  }
  async function downloadSel() {
    const sel = selEntries()
    if (!sel.length) return toast('Selecione o que baixar.', true)
    try { await Api.PickDownload(tab.id, sel.map(fullPath)) } catch (e) { toast(cleanErr(e), true) }
  }
  async function uploadSel() {
    const sel = selEntries()
    const r = peer()
    if (!r || !r.st.ready) return toast('Abra um servidor para enviar.', true)
    if (!sel.length) return toast('Selecione o que enviar.', true)
    try { await Api.Upload(r.tab.id, sel.map(fullPath), r.st.path) } catch (e) { toast(cleanErr(e), true) }
  }
  async function mkdir() {
    const name = await promptBox({ title: 'Nova pasta', label: 'Nome da pasta', ok: 'Criar' })
    if (!name) return
    try { await (remote ? Api.SFTPMkdir(tab.id, joinP(st.path, name)) : Api.LocalMkdir(joinP(st.path, name))); refresh() } catch (e) { toast(cleanErr(e), true) }
  }
  async function mkfile() {
    const name = await promptBox({ title: 'Novo arquivo', label: 'Nome do arquivo', ok: 'Criar' })
    if (!name) return
    try { await (remote ? Api.SFTPCreateFile(tab.id, joinP(st.path, name)) : Api.LocalCreateFile(joinP(st.path, name))); refresh() } catch (e) { toast(cleanErr(e), true) }
  }
  async function renameSel() {
    const e = selEntries()[0]
    if (!e) return toast('Selecione um item.', true)
    const name = await promptBox({ title: 'Renomear', label: 'Novo nome', value: e.name, ok: 'Renomear' })
    if (!name || name === e.name) return
    try { await (remote ? Api.SFTPRename(tab.id, fullPath(e), joinP(st.path, name)) : Api.LocalRename(fullPath(e), joinP(st.path, name))); refresh() } catch (err) { toast(cleanErr(err), true) }
  }
  async function deleteSel() {
    const sel = selEntries()
    if (!sel.length) return toast('Selecione o que excluir.', true)
    const what = sel.length === 1 ? `"${sel[0].name}"` : `${sel.length} itens`
    const hasDir = sel.some((e) => e.isDir && !e.isLink)
    if (!(await confirmBox(remote ? 'Excluir no servidor' : 'Excluir', `Excluir ${what}${hasDir ? ' com todo o conteúdo' : ''}? Isso não pode ser desfeito.`))) return
    let failed = 0
    for (const e of sel) {
      try { await (remote ? Api.SFTPRemove(tab.id, fullPath(e)) : Api.LocalRemove(fullPath(e))) } catch (err) { failed++; toast(cleanErr(err), true) }
    }
    if (!failed) toast(sel.length > 1 ? 'Itens excluídos' : 'Excluído')
    refresh()
  }
  function setClip(mode) {
    const sel = selEntries()
    if (!sel.length) return
    clip = { mode, side, tabId: tab.id, dir: st.path, names: sel.map((e) => e.name), paths: sel.map(fullPath) }
    toast(`${sel.length} item(ns) ${mode === 'copy' ? 'copiado(s)' : 'recortado(s)'}`)
  }
  async function paste() {
    if (!clip) return
    const c = clip
    try {
      if (remote) {
        if (dead()) return
        if (c.side === 'remote') {
          if (c.tabId !== tab.id) return toast('Para copiar entre servidores, baixe e envie de novo.', true)
          if (c.mode === 'cut') await Api.SFTPMove(tab.id, c.paths, st.path)
          else await Api.SFTPCopy(tab.id, c.paths, st.path)
          if (c.mode === 'cut') clip = null
          refresh()
        } else {
          await Api.Upload(tab.id, c.paths, st.path)
        }
      } else if (c.side === 'remote') {
        await Api.Download(c.tabId, c.paths, st.path)
      } else if (c.mode === 'cut') {
        await Api.LocalMove(c.paths, st.path)
        clip = null
        refresh()
      } else {
        await Api.LocalCopy(c.paths, st.path)
      }
    } catch (e) { toast(cleanErr(e), true) }
  }

  // ---- menus
  function rowMenu(x, y, e) {
    const sel = selEntries()
    const multi = sel.length > 1
    const items = []
    if (remote) {
      items.push(e.isDir && !multi ? { label: 'Abrir', onclick: () => openEntry(e) } : { label: 'Editar', icon: 'edit', disabled: multi || e.isDir, onclick: () => editFile(e) })
      items.push({ label: 'Baixar...', icon: 'download', onclick: downloadSel })
    } else {
      items.push({ label: 'Abrir', disabled: multi, onclick: () => openEntry(e) })
      items.push({ label: 'Mostrar na pasta', disabled: multi, onclick: () => Api.LocalShow(fullPath(e)).catch((err) => toast(cleanErr(err), true)) })
      items.push({ label: 'Enviar para o servidor', icon: 'upload', disabled: !peer(), onclick: uploadSel })
    }
    items.push(
      { sep: true },
      { label: 'Copiar', icon: 'copy', kbd: 'Ctrl+C', onclick: () => setClip('copy') },
      { label: 'Recortar', icon: 'cut', kbd: 'Ctrl+X', onclick: () => setClip('cut') },
      { label: 'Colar', icon: 'paste', kbd: 'Ctrl+V', disabled: !clip || !e.isDir, onclick: () => pasteInto(e) },
      { sep: true },
      { label: 'Renomear...', icon: 'pencil', kbd: 'F2', disabled: multi, onclick: renameSel },
      { label: 'Copiar caminho', onclick: () => host.copyText(sel.map(fullPath).join('\n')) })
    if (remote) {
      items.push(
        ...(ftp ? [] : [{ label: 'Permissões...', icon: 'lock', onclick: () => chmodDialog(sel) }]),
        { label: 'Propriedades', icon: 'info', disabled: multi, onclick: () => infoDialog(e) })
    }
    items.push({ sep: true }, ...newItems(), { sep: true }, { label: 'Excluir', icon: 'trash', kbd: 'Del', danger: true, onclick: deleteSel })
    showMenu(x, y, items)
  }
  function newItems() {
    return [
      { label: 'Novo arquivo...', icon: 'newfile', onclick: mkfile },
      { label: 'Nova pasta...', icon: 'newfolder', onclick: mkdir },
    ]
  }
  function bgMenu(x, y) {
    const items = [{ label: 'Colar', icon: 'paste', kbd: 'Ctrl+V', disabled: !clip, onclick: paste }, { sep: true }, ...newItems()]
    if (remote) items.push({ sep: true }, { label: 'Enviar arquivos...', icon: 'upload', onclick: () => pickUpload(false) }, { label: 'Enviar pasta...', icon: 'uploaddir', onclick: () => pickUpload(true) })
    items.push({ sep: true }, { label: 'Atualizar', icon: 'refresh', kbd: 'F5', onclick: refresh },
      { label: 'Mostrar arquivos ocultos', checked: st.hidden, onclick: () => { st.hidden = !st.hidden; render() } })
    showMenu(x, y, items)
  }
  async function pasteInto(e) {
    const prev = st.path
    st.path = fullPath(e)
    try { await paste() } finally { st.path = prev }
  }

  // ---- permissões e propriedades (só remoto)
  function chmodDialog(sel) {
    if (!sel.length) return
    const first = sel[0]
    let perm = first.perm & 0o777
    const bits = []
    const octal = h('input', { type: 'text', class: 'octal', maxlength: '4', spellcheck: 'false' })
    const grid = h('div', { class: 'perm-grid' }, h('span'), ...['Ler', 'Gravar', 'Executar'].map((t) => h('span', { class: 'ph' }, t)))
    ;['Dono', 'Grupo', 'Outros'].forEach((who, wi) => {
      grid.append(h('span', { class: 'pw' }, who))
      for (let b = 0; b < 3; b++) {
        const mask = 1 << (8 - (wi * 3 + b))
        const cb = h('input', { type: 'checkbox' })
        cb.addEventListener('change', () => { perm = cb.checked ? perm | mask : perm & ~mask; sync() })
        bits.push({ cb, mask })
        grid.append(h('label', { class: 'pc' }, cb))
      }
    })
    const rec = h('input', { type: 'checkbox' })
    function sync() {
      for (const { cb, mask } of bits) cb.checked = !!(perm & mask)
      octal.value = perm.toString(8).padStart(3, '0')
    }
    octal.addEventListener('input', () => {
      if (/^[0-7]{3,4}$/.test(octal.value)) { perm = parseInt(octal.value, 8) & 0o777; for (const { cb, mask } of bits) cb.checked = !!(perm & mask) }
    })
    sync()
    const apply = async () => {
      closeModal()
      let failed = 0
      for (const e of sel) {
        try { await Api.SFTPChmod(tab.id, fullPath(e), perm, rec.checked && e.isDir) } catch (err) { failed++; toast(cleanErr(err), true) }
      }
      if (!failed) toast('Permissões alteradas')
      refresh()
    }
    openModal([
      h('h2', {}, 'Permissões'),
      h('p', { class: 'dim' }, sel.length === 1 ? first.name : sel.length + ' itens'),
      grid,
      h('label', {}, 'Valor octal'), octal,
      sel.some((e) => e.isDir) ? h('label', { class: 'check' }, rec, ' Aplicar também ao conteúdo das pastas') : null,
      h('div', { class: 'btns' }, h('button', { onclick: closeModal }, 'Cancelar'), h('button', { class: 'primary', onclick: apply }, 'Aplicar')),
    ])
  }

  async function infoDialog(e) {
    let d
    if (ftp) {
      d = { path: fullPath(e), name: e.name, size: e.size, isDir: e.isDir, link: e.isLink, linkTarget: '', modTime: e.modTime }
    } else {
      try { d = await Api.SFTPInfo(tab.id, fullPath(e)) } catch (err) { return toast(cleanErr(err), true) }
    }
    const sizeVal = h('span', {}, fmtSize(d.size) + (d.size >= 1024 ? ` (${d.size.toLocaleString('pt-BR')} bytes)` : ''))
    const rowsEl = [
      ['Nome', d.name],
      ['Caminho', d.path],
      ['Tipo', d.link ? 'Link' + (d.isDir ? ' para pasta' : '') + (d.linkTarget ? ' → ' + d.linkTarget : '') : d.isDir ? 'Pasta' : 'Arquivo'],
      ['Tamanho', sizeVal],
      ...(ftp ? [] : [
        ['Permissões', `${d.mode} (${(d.perm & 0o777).toString(8).padStart(3, '0')})`],
        ['Dono (UID)', String(d.uid)],
        ['Grupo (GID)', String(d.gid)],
      ]),
      ['Modificado', fmtDate(d.modTime)],
      ...(ftp ? [] : [['Acessado', fmtDate(d.accessTime)]]),
    ].map(([k, v]) => h('div', { class: 'info-row' }, h('span', { class: 'ik' }, k), h('span', { class: 'iv' }, v)))
    const calc = d.isDir && !d.link
      ? h('button', {
        onclick: async (ev) => {
          ev.target.disabled = true
          ev.target.textContent = 'Calculando...'
          try {
            const s = await Api.SFTPDirSize(tab.id, d.path)
            sizeVal.textContent = `${fmtSize(s.bytes)} · ${s.files} arquivo(s), ${s.dirs} pasta(s)`
            ev.target.remove()
          } catch (err) { ev.target.disabled = false; ev.target.textContent = 'Calcular tamanho'; toast(cleanErr(err), true) }
        },
      }, 'Calcular tamanho')
      : null
    openModal([
      h('h2', {}, 'Propriedades'),
      h('div', { class: 'info' }, ...rowsEl),
      h('div', { class: 'btns' }, calc, h('span', { class: 'spacer' }), h('button', { class: 'primary', onclick: closeModal }, 'Fechar')),
    ])
  }

  if (remote) p.dead = dead
  return p
}

// ------------------------------------------------------------------ painéis das abas
/** Garante o painel remoto de uma aba SSH (usado pela barra lateral). */
export function remotePaneOf(tab) {
  if (!tab.remote) tab.remote = createPane('remote', tab, { wide: false })
  return tab.remote
}

/** Mostra na barra lateral o navegador da aba ativa. */
export function renderSide() {
  if (!host) return
  const box = $('panel-sftp')
  const t = host.activeTab()
  const msg = (s) => box.replaceChildren(h('div', { class: 'sftp-msg' }, s))
  if (!t) return msg('Nenhuma sessão aberta.')
  if (t.kind === 'sftp' || t.kind === 'ftp') return msg('Esta sessão mostra os arquivos na própria aba.')
  if (t.kind !== 'ssh') return msg('Sem arquivos nesta sessão.')
  const pane = remotePaneOf(t)
  if (box.firstChild !== pane.el) box.replaceChildren(pane.el)
  if (t.dead || !t.ready) { pane.load(''); return }
  if (!pane.st.ready && !pane.st.loading && !pane.st.err && $('panel-sftp').offsetParent !== null) pane.load('')
}

/** Monta a tela de arquivos de uma aba SFTP: local à esquerda, remoto à direita. */
export function buildFileTab(tab) {
  const local = createPane('local', tab, { wide: true, peer: () => remote })
  const remote = createPane('remote', tab, { wide: true, peer: () => local })
  tab.local = local
  tab.remote = remote
  const split = h('div', { class: 'fp-split' },
    h('div', { class: 'fp-side' }, h('div', { class: 'fp-title' }, 'Este computador'), local.el),
    h('div', { class: 'fp-side' }, h('div', { class: 'fp-title' }, tab.session.name || 'Servidor'), remote.el))
  tab.pane.append(split)
  local.load('')
  remote.load('')
}

// ------------------------------------------------------------------ soltar arquivos do sistema
async function onSystemDrop(ev) {
  let d
  try { d = JSON.parse(payload(ev)) } catch { return }
  const files = d.files || []
  const m = /^drop-([rtl])-(.+)$/.exec(d.target || '')
  if (!files.length || !m) return
  const tab = host.getTab(m[2])
  if (!tab || tab.dead || !tab.ready) return toast('A conexão não está ativa.', true)
  try {
    if (m[1] === 'l') return toast('Solte os arquivos no lado do servidor para enviá-los.', true)
    let dir = tab.remote?.st.path
    if (m[1] === 't') dir = tab.cwd || dir
    if (!dir) dir = (await Api.SFTPList(tab.id, '')).path
    await Api.Upload(tab.id, files, dir)
  } catch (e) { toast(cleanErr(e), true) }
}

// ------------------------------------------------------------------ transferências
const xf = new Map() // id -> info
const xrows = new Map() // id -> { el, ... }
let drawer = null
let open = false
const STATE = { queued: 'Na fila', running: 'Transferindo', conflict: 'Aguardando resposta', done: 'Concluído', error: 'Erro', canceled: 'Cancelado', paused: 'Pausado' }
const prevState = new Map()
let refreshTimer

function buildDrawer() {
  drawer = h('div', { id: 'xfers', class: 'hidden' })
  $('app').insertBefore(drawer, $('statusbar'))
  drawer.append(
    h('div', { class: 'xf-head', onclick: () => { open = !open; renderDrawer() } },
      icon('transfer', 15), h('span', { class: 'xf-title' }, 'Transferências'), h('span', { class: 'xf-sum' }),
      h('button', { class: 'mini-link', onclick: (e) => { e.stopPropagation(); Api.ClearTransfers() } }, 'Limpar'),
      h('span', { class: 'xf-caret' }, '▾')),
    h('div', { class: 'xf-list' }))
}

function fmtEta(sec) {
  if (!isFinite(sec) || sec < 0) return ''
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return m >= 60 ? `${Math.floor(m / 60)}h${String(m % 60).padStart(2, '0')}` : `${m}:${String(s).padStart(2, '0')}`
}

function renderDrawer() {
  if (!drawer) return
  const list = drawer.querySelector('.xf-list')
  const items = [...xf.values()]
  drawer.classList.toggle('hidden', items.length === 0)
  drawer.classList.toggle('open', open)
  drawer.querySelector('.xf-caret').textContent = open ? '▾' : '▴'
  const active = items.filter((i) => ['queued', 'running', 'conflict'].includes(i.state))
  const speed = active.reduce((s, i) => s + (i.speed || 0), 0)
  drawer.querySelector('.xf-sum').textContent = active.length
    ? `${active.length} em andamento` + (speed ? ' · ' + fmtSize(speed) + '/s' : '')
    : `${items.length} concluída(s)`
  for (const id of [...xrows.keys()]) if (!xf.has(id)) { xrows.get(id).el.remove(); xrows.delete(id) }
  for (const i of items) {
    let r = xrows.get(i.id)
    if (!r) { r = makeRow(i); xrows.set(i.id, r); list.append(r.el) }
    updateRow(r, i)
  }
}

function makeRow(i) {
  const bar = h('div', { class: 'xf-fill' })
  const r = {
    el: null,
    bar,
    name: h('span', { class: 'xf-name' }),
    info: h('span', { class: 'xf-info' }),
    acts: h('span', { class: 'xf-acts' }),
    ico: h('span', { class: 'xf-ico' }, icon(i.dir === 'up' ? 'upload' : 'download', 14)),
    state: '',
  }
  r.el = h('div', { class: 'xf-row' }, r.ico, r.name, h('div', { class: 'xf-bar' }, bar), r.info, r.acts)
  return r
}

function actBtn(ico, title, fn) {
  return h('button', { class: 'ib', title, onclick: (e) => { e.stopPropagation(); fn().catch((err) => toast(cleanErr(err), true)) } }, icon(ico, 14))
}

function updateRow(r, i) {
  r.name.textContent = i.name
  r.name.title = `${i.src}\n→ ${i.dst}`
  const pct = i.total > 0 ? Math.min(100, (i.done / i.total) * 100) : i.state === 'done' ? 100 : 0
  r.bar.style.width = pct + '%'
  r.el.className = 'xf-row ' + i.state
  let t = STATE[i.state] || i.state
  if (i.state === 'running') {
    t = `${fmtSize(i.done)} / ${fmtSize(i.total)} · ${Math.floor(pct)}%`
    if (i.speed > 0) t += ` · ${fmtSize(i.speed)}/s · ${fmtEta((i.total - i.done) / i.speed)}`
  } else if (i.state === 'done') {
    t = `Concluído · ${fmtSize(i.total)}` + (i.skipped ? ` · ${i.skipped} pulado(s)` : '')
  } else if (i.state === 'paused') {
    t = `Pausado em ${Math.floor(pct)}%`
  } else if (i.state === 'error') {
    t = 'Erro: ' + i.err
    r.info.title = i.err
  }
  r.info.textContent = t
  if (r.state !== i.state) {
    r.state = i.state
    const a = []
    if (i.state === 'running') a.push(actBtn('pause', 'Pausar', () => Api.PauseTransfer(i.id)))
    if (['paused', 'error', 'canceled'].includes(i.state)) a.push(actBtn('play', 'Continuar de onde parou', () => Api.ResumeTransfer(i.id)))
    if (['queued', 'running', 'conflict'].includes(i.state)) a.push(actBtn('stop', 'Cancelar', () => Api.CancelTransfer(i.id)))
    else a.push(actBtn('close', 'Remover da lista', () => Api.RemoveTransfer(i.id)))
    r.acts.replaceChildren(...a)
  }
}

function onXfer(i) {
  const isNew = !xf.has(i.id)
  xf.set(i.id, i)
  if (isNew && !open) open = true
  renderDrawer()
  const before = prevState.get(i.id)
  prevState.set(i.id, i.state)
  if (before !== i.state && ['done', 'error', 'canceled', 'paused'].includes(i.state)) {
    clearTimeout(refreshTimer)
    refreshTimer = setTimeout(() => refreshPanesOf(i.tabId), 300)
    if (i.state === 'done') {
      const busy = [...xf.values()].some((x) => ['queued', 'running', 'conflict'].includes(x.state))
      if (!busy) toast([...xf.values()].filter((x) => x.state === 'done').length > 1 ? 'Transferências concluídas' : `"${i.name}" ${i.dir === 'up' ? 'enviado' : 'copiado'}`)
    }
  }
}

function refreshPanesOf(tabId) {
  const t = host.getTab(tabId)
  if (t) {
    t.remote?.refresh()
    t.local?.refresh()
  }
  // cópias locais (sem aba) e downloads: atualiza qualquer painel local aberto
  for (const x of host.allTabs()) x.local?.refresh()
}

// ---- diálogo de conflito
const conflicts = []
let conflictOpen = false

function onConflict(ev) {
  try { conflicts.push(JSON.parse(payload(ev))) } catch { return }
  nextConflict()
}

function nextConflict() {
  if (conflictOpen || !conflicts.length) return
  if (modalOpen()) { setTimeout(nextConflict, 400); return }
  const c = conflicts.shift()
  const info = xf.get(c.id)
  if (info && info.state !== 'conflict') return nextConflict() // já foi resolvido ou cancelado
  conflictOpen = true
  const all = h('input', { type: 'checkbox' })
  const answer = (action) => {
    closeModal()
    conflictOpen = false
    Api.AnswerConflict(c.id, action, all.checked).catch(() => {})
    nextConflict()
  }
  const side = (label, size, time) => h('div', { class: 'cf-side' }, h('b', {}, label), h('span', {}, fmtSize(size)), h('span', {}, fmtDate(time)))
  openModal([
    h('h2', {}, 'O arquivo já existe'),
    h('p', { class: 'cf-name' }, c.name),
    h('div', { class: 'cf-cmp' }, side('Novo', c.srcSize, c.srcTime), side('Existente', c.dstSize, c.dstTime)),
    h('label', { class: 'check' }, all, ' Aplicar a todos os conflitos desta transferência'),
    h('div', { class: 'btns' },
      h('button', { onclick: () => answer('cancel') }, 'Cancelar'),
      h('span', { class: 'spacer' }),
      h('button', { onclick: () => answer('skip') }, 'Pular'),
      h('button', { onclick: () => answer('rename') }, 'Manter os dois'),
      c.dstSize < c.srcSize ? h('button', { onclick: () => answer('resume') }, 'Continuar') : null,
      h('button', { class: 'primary', onclick: () => answer('overwrite') }, 'Substituir')),
  ])
}

// ------------------------------------------------------------------ início
/** hostApi: { getTab(id), activeTab(), allTabs(), reconnectTab(tab), copyText(text) } */
export async function initFiles(hostApi) {
  host = hostApi
  buildDrawer()
  Events.On('xfer:update', (ev) => { try { onXfer(JSON.parse(payload(ev))) } catch { /* ignora */ } })
  Events.On('xfer:conflict', onConflict)
  Events.On('xfer:cleared', async () => {
    try {
      const list = (await Api.Transfers()) || []
      const ids = new Set(list.map((i) => i.id))
      for (const id of [...xf.keys()]) if (!ids.has(id)) { xf.delete(id); prevState.delete(id) }
      renderDrawer()
    } catch { /* ignora */ }
  })
  Events.On('files:dropped', onSystemDrop)
  Events.On('edit:saved', (ev) => {
    try {
      const d = JSON.parse(payload(ev))
      toast(d.ok ? `"${d.name}" salvo no servidor` : `Não foi possível salvar "${d.name}": ${d.err}`, !d.ok)
      if (d.ok) host.getTab(d.tabId)?.remote?.refresh()
    } catch { /* ignora */ }
  })
  try { for (const i of (await Api.Transfers()) || []) { xf.set(i.id, i); prevState.set(i.id, i.state) } } catch { /* sem backend */ }
  renderDrawer()
}
