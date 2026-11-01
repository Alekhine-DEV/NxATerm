import '@xterm/xterm/css/xterm.css'
import './style.css'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { WebglAddon } from '@xterm/addon-webgl'
import { ImageAddon } from '@xterm/addon-image'
import * as Runtime from '@wailsio/runtime'
import { Api } from './api.js'
import {
  $, h, icon, toast, payload, b64ToBytes, fmtSize, fmtRate, fmtUptime, cleanErr,
  openModal, closeModal, modalOpen, promptBox, confirmBox, showMenu, closeMenu, attachTip, hideTip, comboOpen,
} from './util.js'
import { settings, loadSettings, saveSettings, termOptions } from './settings.js'
import { editSession, editMacro, tunnelDialog, keygenDialog, aboutDialog, shortcutsDialog } from './dialogs.js'
import { keyOf, actionFor, capture } from './keys.js'
import { memo, memoKey } from './memo.js'
import { settingsDialog } from './settingsdlg.js'
import { recordHistory, historyDialog, workspaceDialog, lockGate } from './organize.js'
import { applyAppearance, setBackgroundData, setStartupTransparent } from './uitheme.js'
import { initConnect } from './connect.js'
import { attachHighlight } from './highlight.js'
import { createZmodem } from './zmodemfx.js'
import { batchDialog } from './batch.js'
import { syncDialog, syncOnOpen } from './sync.js'
import { setLang } from './i18n.js'
import { initAi, renderAiPanel } from './ai.js'
import { checkUpdates } from './updates.js'
import { initFiles, renderSide, buildFileTab } from './files.js'
import { TYPE_INFO, typeOf, hasFiles, hasSSH, hasTerminal, opensExternally, sessionSummary, isWindows } from './sessiontypes.js'
import { createVncView, connectVnc, disposeVnc } from './vnc.js'

const Events = Runtime.Events

// ------------------------------------------------------------------ estado
let sessions = []
let folders = [] // pastas criadas pelo usuário (podem estar vazias)
let macros = []
const tabs = new Map() // id -> tab
let activeId = 'home'
let tabCounter = 0
let tabNumber = 0
let sideView = 'sessions'
let layout = 'single'
const collapsed = new Set() // pastas recolhidas na árvore de sessões

const LAYOUTS = {
  single: { n: 1, label: 'Uma janela' },
  cols: { n: 2, label: '2 colunas (lado a lado)' },
  rows: { n: 2, label: '2 linhas (uma sobre a outra)' },
  grid: { n: 4, label: 'Grade 2×2' },
}

// MultiExec (como no MobaXterm): com o modo ligado, o que se digita/cola em um terminal vai para todos,
// exceto os que estiverem em `off` (marcados como "Excluir do modo MultiExec").
const me = { open: false, off: new Set() }

const activeTab = () => tabs.get(activeId) || null
const allTabs = () => [...tabs.values()]
const aliveTabs = () => allTabs().filter((t) => !t.dead && t.ready && t.term) // só abas com terminal recebem MultiExec e macros

function openURL(uri) {
  if (!/^https?:\/\//i.test(uri)) return
  try {
    if (Runtime.Browser && Runtime.Browser.OpenURL) {
      const r = Runtime.Browser.OpenURL(uri)
      if (r && r.catch) r.catch(() => {})
    } else {
      window.open(uri, '_blank')
    }
  } catch { /* sem navegador disponível */ }
}

const copyText = (text) => navigator.clipboard.writeText(text).catch(() => {})

// ------------------------------------------------------------------ dados
async function refreshSessions() {
  sessions = (await Api.ListSessions()) || []
  folders = (await Api.ListFolders()) || []
  renderTree()
  renderHome()
}

async function refreshMacros() {
  macros = (await Api.ListMacros()) || []
  renderMacros()
}

// ------------------------------------------------------------------ barra de menus e toolbar
const MENUS = {
  Terminal: () => [
    { label: 'Nova sessão...', kbd: keyOf('newSession'), icon: 'session', onclick: () => editSession(null, refreshSessions) },
    { label: 'Conexão rápida', kbd: keyOf('quick'), icon: 'send', onclick: () => $('quick').focus() },
    { sep: true },
    { label: 'Duplicar aba', kbd: keyOf('duplicate'), disabled: !activeTab(), onclick: () => duplicateTab(activeTab()) },
    { label: 'Reconectar', disabled: !activeTab() || !activeTab().dead, onclick: () => reconnectTab(activeTab()) },
    { label: 'Fechar aba', kbd: keyOf('closeTab'), disabled: !activeTab(), onclick: () => closeTab(activeId) },
    { sep: true },
    { label: 'Buscar no terminal', kbd: keyOf('search'), disabled: !activeTab() || !activeTab().term, onclick: () => toggleSearch(activeTab()) },
    { label: 'Limpar tela', disabled: !activeTab() || !activeTab().term, onclick: () => activeTab().term.clear() },
    { sep: true },
    { label: 'Bloquear', icon: 'lock', onclick: lockNow },
    { label: 'Sair', icon: 'power', onclick: () => Api.Quit() },
  ],
  Sessões: () => [
    ...treeMenu(''),
    { sep: true },
    { label: 'Histórico de conexões...', icon: 'refresh', onclick: () => historyDialog(openSession) },
    { label: 'Espaços de trabalho...', icon: 'split', onclick: openWorkspaces },
    { sep: true },
    { label: 'Mostrar a lista de sessões', icon: 'star', onclick: () => showSide('sessions') },
  ],
  Exibir: () => [
    { label: 'Barra lateral', kbd: keyOf('sidebar'), checked: !$('workspace').classList.contains('no-side'), onclick: toggleSidebar },
    { label: 'Barra de status', checked: settings.showStatusBar, onclick: toggleStatusbar },
    { sep: true },
    ...Object.entries(LAYOUTS).map(([k, v]) => ({ label: v.label, checked: layout === k, onclick: () => setLayout(k) })),
    { sep: true },
    { label: 'Aumentar fonte', onclick: () => zoomFont(1) },
    { label: 'Diminuir fonte', onclick: () => zoomFont(-1) },
  ],
  Ferramentas: () => [
    { label: 'MultiExec', kbd: keyOf('multiexec'), checked: me.open, icon: 'multiexec', onclick: toggleMultiexec },
    { label: 'Túneis SSH...', kbd: keyOf('tunnels'), icon: 'tunnel', onclick: openTunnels },
    { label: 'Executar em lote...', icon: 'playall', onclick: () => batchDialog(sessions) },
    { label: 'Gerador de chaves SSH...', icon: 'key', onclick: keygenDialog },
    { sep: true },
    { label: 'Assistente de IA', icon: 'bot', onclick: () => showSide('ai') },
    { label: 'Sincronização...', icon: 'refresh', onclick: openSync },
  ],
  Macros: () => [
    ...(visibleMacros('active').length
      ? visibleMacros('active').map((m) => ({ label: (m.pinned ? '★ ' : '') + m.name, icon: 'play', onclick: () => runMacro(m, false) }))
      : [{ label: '(nenhuma macro)', disabled: true }]),
    { sep: true },
    { label: 'Nova macro...', icon: 'plus', onclick: () => editMacro(null, refreshMacros) },
    { label: 'Gerenciar macros', icon: 'send', onclick: () => showSide('macros') },
    { sep: true },
    { label: 'Exportar macros...', icon: 'download', onclick: exportMacros },
    { label: 'Importar macros...', icon: 'upload', onclick: importMacros },
  ],
  Configurações: () => [
    { label: 'Configurações...', kbd: keyOf('settings'), icon: 'gear', onclick: openSettings },
    { label: 'Atalhos de teclado...', icon: 'keyboard', onclick: shortcutsDialog },
    { sep: true },
    { label: 'Esquecer senhas memorizadas', disabled: !memo.size, onclick: () => { memo.clear(); toast('Senhas esquecidas') } },
  ],
  Sobre: () => [
    { label: 'Verificar atualizações...', icon: 'refresh', onclick: () => checkUpdates({ silent: false, openURL }) },
    { label: 'Sobre o NxATerm', icon: 'help', onclick: () => aboutDialog(() => checkUpdates({ silent: false, openURL })) },
  ],
}

let barOpen = null
let barLastClosed = { name: '', at: 0 }

function openBarMenu(btn, name) {
  const r = btn.getBoundingClientRect()
  showMenu(r.left, r.bottom, MENUS[name](), () => {
    btn.classList.remove('open')
    if (barOpen === name) { barOpen = null; barLastClosed = { name, at: Date.now() } }
  })
  barOpen = name
  btn.classList.add('open')
}

function buildMenubar() {
  const bar = $('menubar')
  for (const name of Object.keys(MENUS)) {
    const btn = h('button', { class: 'mb-item' }, name)
    btn.addEventListener('click', () => {
      if (barLastClosed.name === name && Date.now() - barLastClosed.at < 250) return // o clique que fechou
      openBarMenu(btn, name)
    })
    btn.addEventListener('mouseenter', () => { if (barOpen && barOpen !== name) openBarMenu(btn, name) })
    bar.append(btn)
  }
}

function toolbarMenu(btn, items) {
  const r = btn.getBoundingClientRect()
  showMenu(r.left, r.bottom, items)
}

function buildToolbar() {
  // Só o que se usa o tempo todo; o resto está nos menus de cima.
  const tb = $('toolbar')
  const add = (label, ico, color, onclick, id) => {
    const b = h('button', { class: 'tb-btn', id: id || null, style: `--c:${color}` },
      h('span', { class: 'tb-ico' }, icon(ico, 22)), h('span', { class: 'tb-lbl' }, label))
    b.addEventListener('click', (e) => onclick(b, e))
    tb.append(b)
    return b
  }
  add('Sessão', 'session', '#4aa8ff', () => editSession(null, refreshSessions))
  tb.append(h('span', { class: 'tb-sep' }))
  add('Dividir', 'split', '#58d6a4', (b) => toolbarMenu(b, Object.entries(LAYOUTS).map(([k, v]) => ({
    label: v.label, checked: layout === k, onclick: () => setLayout(k),
  }))))
  add('MultiExec', 'multiexec', '#c18bff', toggleMultiexec, 'tb-multiexec')
  add('Túneis', 'tunnel', '#5fd0e8', openTunnels)
  add('Macros', 'send', '#ff7ab6', () => showSide('macros'))
}

function applySettingsEverywhere() {
  for (const t of allTabs()) applyTermSettings(t)
  renderStatus()
  syncFollow()
  pollStats()
}

function openSettings() { settingsDialog(applySettingsEverywhere) }

const syncHooks = () => ({
  onData: () => Promise.all([refreshSessions(), refreshMacros()]),
  onSettings: applySettingsEverywhere,
})
function openSync() { syncDialog(syncHooks()) }

function openTunnels() {
  tunnelDialog(() => {
    const t = activeTab()
    return t && !t.dead && hasSSH(t.session) ? { id: t.id, name: t.label } : null
  })
}

async function importSSHConfig() {
  try {
    const n = await Api.ImportSSHConfig()
    toast(n > 0 ? n + ' sessão(ões) importada(s) de ~/.ssh/config' : 'Nada novo para importar')
    await refreshSessions()
  } catch (e) { toast('Não foi possível ler ~/.ssh/config: ' + cleanErr(e), true) }
}

// ------------------------------------------------------------------ barra lateral
const SIDE_VIEWS = [
  { id: 'sessions', title: 'Sessões', icon: 'star' },
  { id: 'sftp', title: 'Navegador SFTP', icon: 'folder' },
  { id: 'macros', title: 'Macros', icon: 'send' },
  { id: 'ai', title: 'Assistente', icon: 'bot' },
]

function buildSidetabs() {
  const nav = $('sidetabs')
  for (const v of SIDE_VIEWS) {
    nav.append(h('button', { class: 'st-btn', 'data-view': v.id, title: v.title, onclick: () => showSide(v.id) }, icon(v.icon, 20)))
  }
}

function showSide(view) {
  sideView = view
  $('workspace').classList.remove('no-side')
  for (const v of SIDE_VIEWS) {
    $('panel-' + v.id).classList.toggle('hidden', v.id !== view)
  }
  document.querySelectorAll('.st-btn').forEach((b) => b.classList.toggle('active', b.dataset.view === view))
  if (view === 'sftp') {
    renderSide()
    syncFollow()
  }
  if (view === 'macros') renderMacros()
  if (view === 'ai') renderAiPanel()
}

function toggleSidebar() {
  $('workspace').classList.toggle('no-side')
}

function toggleStatusbar() {
  settings.showStatusBar = !settings.showStatusBar
  $('statusbar').classList.toggle('hidden', !settings.showStatusBar)
  saveSettings()
}

function initResizer() {
  const rz = $('resizer')
  rz.addEventListener('mousedown', (e) => {
    e.preventDefault()
    const move = (ev) => {
      const w = Math.min(640, Math.max(220, ev.clientX))
      settings.sidebarWidth = w
      $('workspace').style.setProperty('--sidew', w + 'px')
    }
    const up = () => {
      document.removeEventListener('mousemove', move)
      document.removeEventListener('mouseup', up)
      document.body.classList.remove('resizing')
      saveSettings()
    }
    document.body.classList.add('resizing')
    document.addEventListener('mousemove', move)
    document.addEventListener('mouseup', up)
  })
}

// ------------------------------------------------------------------ painel: sessões
/** Monta a árvore de pastas a partir de "Produção/Web" etc. (as pastas só existem enquanto têm sessões). */
function buildFolderTree(list, extraFolders = []) {
  const root = { name: '', path: '', children: new Map(), items: [], total: 0 }
  for (const f of extraFolders) {
    let node = root
    let path = ''
    for (const part of f.split('/').filter(Boolean)) {
      path = path ? path + '/' + part : part
      if (!node.children.has(part)) node.children.set(part, { name: part, path, children: new Map(), items: [], total: 0 })
      node = node.children.get(part)
    }
  }
  for (const s of list) {
    let node = root
    root.total++
    let path = ''
    for (const part of (s.folder || '').split('/').map((x) => x.trim()).filter(Boolean)) {
      path = path ? path + '/' + part : part
      if (!node.children.has(part)) node.children.set(part, { name: part, path, children: new Map(), items: [], total: 0 })
      node = node.children.get(part)
      node.total++
    }
    node.items.push(s)
  }
  return root
}

function renderTree() {
  const raw = $('quick').value.trim()
  const q = raw.toLowerCase()
  // "#tag" filtra pelas tags da sessão; o resto do texto continua valendo como busca comum
  const tagQ = q.startsWith('#') ? q.slice(1) : null
  const list = sessions.filter((s) => {
    if (!q) return true
    if (tagQ !== null) return (s.tags || []).some((t) => t.toLowerCase().includes(tagQ))
    return [s.name, s.host, s.user, s.folder, s.notes, ...(s.tags || []), sessionSummary(s), TYPE_INFO[typeOf(s)].label].some((x) => (x || '').toLowerCase().includes(q))
  })

  const root = buildFolderTree(list, q ? [] : folders)
  const tree = $('tree')
  tree.replaceChildren()

  const draw = (node, into) => {
    const names = [...node.children.keys()].sort((a, b) => a.localeCompare(b, 'pt-BR', { sensitivity: 'base' }))
    for (const name of names) {
      const child = node.children.get(name)
      const closed = collapsed.has(child.path) && !q
      const inner = h('div', { class: 'fb-inner' })
      const body = h('div', { class: 'folder-body' + (closed ? ' closed' : '') }, inner)
      const head = h('div', {
        class: 'folder-head' + (closed ? ' closed' : ''), 'data-folder': child.path,
        onclick: () => {
          // sem redesenhar a árvore: só troca a classe, e o CSS anima
          const nowClosed = !collapsed.has(child.path)
          nowClosed ? collapsed.add(child.path) : collapsed.delete(child.path)
          head.classList.toggle('closed', nowClosed)
          body.classList.toggle('closed', nowClosed)
        },
      },
      h('span', { class: 'caret' }, icon('chev', 12)),
      h('span', { class: 'ico-folder' }, icon('folder', 15)),
      h('span', { class: 'fname' }, child.name))
      wireFolderHead(head, child.path)
      draw(child, inner)
      into.append(head, body)
    }
    for (const s of node.items) into.append(sessionRow(s))
  }
  draw(root, tree)

  if (!list.length && q) {
    const qc = parseQuick(raw)
    tree.append(qc
      ? h('div', { class: 'tree-msg action', onclick: () => connectQuick() }, icon('play', 14), `Conectar a ${qc.user}@${qc.host}${qc.port !== 22 ? ':' + qc.port : ''} (Enter)`)
      : h('div', { class: 'tree-msg' }, 'Nenhuma sessão encontrada.'))
  } else if (!sessions.length && !folders.length) {
    tree.append(h('div', { class: 'tree-msg' }, 'Nenhuma sessão'))
  }
}

/** Enter na caixa do topo: conecta em usuario@host, ou abre a única sessão que sobrou no filtro. */
function connectQuick() {
  const input = $('quick')
  const raw = input.value.trim()
  if (!raw) return
  const qc = parseQuick(raw)
  const q = raw.toLowerCase()
  const hits = sessions.filter((s) =>
    [s.name, s.host, s.user, s.folder, sessionSummary(s)].some((x) => (x || '').toLowerCase().includes(q)))
  if (hits.length === 1) {
    input.value = ''
    renderTree()
    openSession(hits[0])
  } else if (!hits.length && qc) {
    input.value = ''
    renderTree()
    openSession(qc)
  } else if (hits.length > 1) {
    toast(`${hits.length} sessões encontradas`)
  }
}

/** Menu e arrastar-e-soltar das pastas. */
function wireFolderHead(head, path) {
  head.addEventListener('contextmenu', (e) => {
    e.preventDefault()
    e.stopPropagation()
    showMenu(e.clientX, e.clientY, treeMenu(path))
  })
  head.addEventListener('dragover', (e) => { if (dragSession) { e.preventDefault(); e.stopPropagation(); head.classList.add('drop') } })
  head.addEventListener('dragleave', () => head.classList.remove('drop'))
  head.addEventListener('drop', (e) => { e.preventDefault(); e.stopPropagation(); head.classList.remove('drop'); moveSession(dragSession, path) })
}

let dragSession = null

async function moveSession(s, folder) {
  if (!s || (s.folder || '') === folder) return
  try {
    await Api.SaveSession({ ...s, folder })
    await refreshSessions()
  } catch (e) { toast(cleanErr(e), true) }
}

const folderSessions = (path) => sessions.filter((s) => (s.folder || '') === path || (s.folder || '').startsWith(path + '/'))

function allFolderPaths() {
  const set = new Set()
  for (const f of [...folders, ...sessions.map((s) => s.folder || '')]) {
    let path = ''
    for (const part of f.split('/').filter(Boolean)) {
      path = path ? path + '/' + part : part
      set.add(path)
    }
  }
  return [...set]
}

async function newFolder(parent = '') {
  const name = await promptBox({
    title: 'Nova pasta', ok: 'Criar',
    label: parent ? `Nome da pasta dentro de "${parent}" (use / para criar subpastas)` : 'Nome da pasta (use / para criar subpastas)',
  })
  const clean = (name || '').trim()
  if (!clean) return
  const path = parent ? parent + '/' + clean : clean
  try {
    await Api.CreateFolder(path)
    let up = ''
    for (const part of path.split('/').slice(0, -1)) { up = up ? up + '/' + part : part; collapsed.delete(up) }
    await refreshSessions()
  } catch (e) { toast(cleanErr(e), true) }
}

async function renameFolder(path) {
  const name = await promptBox({ title: 'Renomear ou mover pasta', label: 'Caminho da pasta', value: path, ok: 'Renomear' })
  if (name === null) return
  try { await Api.RenameFolder(path, name); await refreshSessions() } catch (e) { toast(cleanErr(e), true) }
}

function folderDeleteChoice(path, n) {
  return new Promise((resolve) => {
    const done = (v) => { closeModal(); resolve(v) }
    openModal([
      h('h2', {}, 'Excluir pasta'),
      h('p', {}, `A pasta "${path}" tem ${n} sessão(ões), contando as subpastas. O que fazer com elas?`),
      h('div', { class: 'btns' },
        h('button', { onclick: () => done(null) }, 'Cancelar'),
        h('button', { onclick: () => done('keep') }, 'Manter as sessões (sobem um nível)'),
        h('button', { class: 'danger', onclick: () => done('all') }, 'Excluir pasta e sessões')),
    ])
  })
}

async function deleteFolder(path) {
  const n = folderSessions(path).length
  let withSessions = false
  if (n === 0) {
    if (!(await confirmBox('Excluir pasta', `Excluir a pasta "${path}"?`))) return
  } else {
    const choice = await folderDeleteChoice(path, n)
    if (!choice) return
    withSessions = choice === 'all'
  }
  try {
    await Api.DeleteFolder(path, withSessions)
    collapsed.delete(path)
    await refreshSessions()
  } catch (e) { toast(cleanErr(e), true) }
}

async function openFolder(path) {
  const list = folderSessions(path)
  if (!list.length) return toast('Esta pasta não tem sessões.', true)
  if (list.length > 8 && !(await confirmBox('Abrir sessões', `Abrir ${list.length} sessões de uma vez?`, 'Abrir'))) return
  for (const s of list) await openSession(s)
}

async function duplicateSession(s) {
  try {
    await Api.SaveSession({ ...s, id: '', name: s.name + ' (cópia)' })
    await refreshSessions()
  } catch (e) { toast(cleanErr(e), true) }
}

async function moveSessionPrompt(s) {
  const f = await promptBox({ title: 'Mover sessão', label: 'Pasta de destino', value: s.folder || '', ok: 'Mover' })
  if (f !== null) moveSession(s, f.trim())
}

async function deleteSession(s) {
  if (!(await confirmBox('Excluir sessão', `Excluir "${s.name}"? Isso não apaga nada no servidor.`))) return
  try { await Api.DeleteSession(s.id); await refreshSessions() } catch (e) { toast(cleanErr(e), true) }
}

// importar / exportar
function importSummary(r) {
  return `${r.added} sessão(ões) importada(s)` + (r.skipped ? `, ${r.skipped} ignorada(s) (já existiam ou são de um tipo sem suporte)` : '')
}

async function importFile() {
  try {
    const r = await Api.ImportSessionsFile()
    if (!r || !r.path) return
    toast(importSummary(r))
    await refreshSessions()
  } catch (e) { toast(cleanErr(e), true) }
}

async function importPutty() {
  try {
    const r = await Api.ImportPuTTY()
    toast(importSummary(r))
    await refreshSessions()
  } catch (e) { toast(cleanErr(e), true) }
}

async function exportFile() {
  try { const p = await Api.ExportSessions(); if (p) toast('Sessões salvas em ' + p) } catch (e) { toast(cleanErr(e), true) }
}

async function exportHtml() {
  try { const p = await Api.ExportHTML(); if (p) toast('Página salva em ' + p) } catch (e) { toast(cleanErr(e), true) }
}

/** Menu do botão direito na lista de sessões (path = pasta clicada, '' = fundo da lista). */
function treeMenu(path = '') {
  const inFolder = !!path
  return [
    { label: inFolder ? 'Nova sessão nesta pasta...' : 'Nova sessão...', icon: 'session', kbd: inFolder ? '' : keyOf('newSession'),
      onclick: () => editSession(null, refreshSessions, inFolder ? { folder: path } : undefined) },
    { label: inFolder ? 'Nova subpasta...' : 'Nova pasta...', icon: 'newfolder', onclick: () => newFolder(path) },
    ...(inFolder ? [
      { sep: true },
      { label: 'Abrir todas as sessões da pasta', icon: 'playall', onclick: () => openFolder(path) },
      { label: 'Executar comando em lote na pasta...', icon: 'send', onclick: () => batchDialog(sessions, { ids: sessions.filter((x) => x.folder === path || x.folder.startsWith(path + '/')).map((x) => x.id) }) },
      { label: 'Renomear ou mover pasta...', icon: 'pencil', onclick: () => renameFolder(path) },
      { label: 'Excluir pasta...', icon: 'trash', danger: true, onclick: () => deleteFolder(path) },
      { label: collapsed.has(path) ? 'Expandir esta pasta' : 'Recolher esta pasta', onclick: () => { collapsed.has(path) ? collapsed.delete(path) : collapsed.add(path); renderTree() } },
    ] : []),
    { sep: true },
    { label: 'Importar de arquivo...', icon: 'upload', onclick: importFile },
    { label: 'Exportar todas as sessões para arquivo...', icon: 'download', onclick: exportFile },
    { label: 'Importar do OpenSSH (~/.ssh/config)', onclick: importSSHConfig },
    ...(isWindows ? [{ label: 'Importar do PuTTY', onclick: importPutty }] : []),
    { label: 'Gerar página HTML com as sessões...', icon: 'file', onclick: exportHtml },
    { sep: true },
    { label: 'Recolher todas as pastas', onclick: () => { for (const f of allFolderPaths()) collapsed.add(f); renderTree() } },
    { label: 'Expandir todas as pastas', onclick: () => { collapsed.clear(); renderTree() } },
    { label: 'Atualizar a lista', icon: 'refresh', onclick: refreshSessions },
  ]
}

function sessionRow(s) {
  const online = allTabs().some((t) => t.session.id === s.id && !t.dead)
  const info = TYPE_INFO[typeOf(s)]
  const row = h('div', { class: 'sess', draggable: 'true', ondblclick: () => { hideTip(); openSession(s) } },
    h('span', { class: 'sess-ico' + (online ? ' on' : ''), style: `color:${info.color}` }, icon(info.icon, 15)),
    h('span', { class: 'name' }, s.name))
  if (s.color) { row.dataset.color = s.color; row.style.setProperty('--sc', s.color) }
  attachTip(row, () => [
    h('div', { class: 'tip-title' }, s.name),
    h('div', { class: 'tip-line' }, info.label + ' · ' + sessionSummary(s)),
    s.folder ? h('div', { class: 'tip-dim' }, s.folder.replace(/\//g, ' › ')) : null,
    (s.tags || []).length ? h('div', { class: 'tip-dim' }, s.tags.map((t) => '#' + t).join('  ')) : null,
    s.notes ? h('div', { class: 'tip-dim' }, s.notes) : null,
  ])
  row.addEventListener('dragstart', (e) => { dragSession = s; e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', s.id) })
  row.addEventListener('dragend', () => { dragSession = null })
  row.addEventListener('contextmenu', (e) => {
    e.preventDefault()
    e.stopPropagation()
    showMenu(e.clientX, e.clientY, [
      { label: 'Abrir', icon: 'play', onclick: () => openSession(s) },
      ...(typeOf(s) === 'ssh' ? [{ label: 'Abrir só os arquivos (SFTP)', icon: 'folder', onclick: () => openSession({ ...s, type: 'sftp' }) }] : []),
      { sep: true },
      { label: 'Editar...', icon: 'pencil', onclick: () => editSession(s, refreshSessions) },
      { label: 'Duplicar', icon: 'copy', onclick: () => duplicateSession(s) },
      { label: 'Mover para a pasta...', icon: 'newfolder', onclick: () => moveSessionPrompt(s) },
      { label: 'Copiar o destino', onclick: () => copyText(sessionSummary(s)) },
      { sep: true },
      { label: 'Nova sessão nesta pasta...', icon: 'session', onclick: () => editSession(null, refreshSessions, s.folder ? { folder: s.folder } : undefined) },
      { label: 'Nova pasta...', icon: 'newfolder', onclick: () => newFolder(s.folder || '') },
      { sep: true },
      { label: 'Excluir sessão...', icon: 'trash', danger: true, onclick: () => deleteSession(s) },
    ])
  })
  return row
}

// ------------------------------------------------------------------ painel: macros e ferramentas
/** A macro vale para esta sessão? Sem vínculo, vale para todas. */
function macroApplies(m, sess) {
  const ids = m.sessions || []
  const tg = (m.tags || []).map((t) => t.toLowerCase())
  if (!ids.length && !tg.length) return true
  if (!sess) return false
  if (ids.includes(sess.id)) return true
  return (sess.tags || []).some((t) => tg.includes(t.toLowerCase()))
}

function visibleMacros(scope = settings.macroScope) {
  if (scope === 'all') return macros
  const t = activeTab()
  return macros.filter((m) => macroApplies(m, t ? t.session : null))
}

function renderMacros() {
  const box = $('macro-list')
  const scope = settings.macroScope
  const list = visibleMacros()
  const seg = (id, label) => h('button', {
    class: 'seg' + (scope === id ? ' on' : ''),
    onclick: () => { settings.macroScope = id; saveSettings(); renderMacros() },
  }, label)
  const head = h('div', { class: 'macro-head' }, seg('active', 'Da sessão ativa'), seg('all', 'Todas'))
  if (!list.length) {
    box.replaceChildren(head, h('div', { class: 'sftp-msg' }, macros.length ? 'Nenhuma macro para esta sessão' : 'Nenhuma macro'))
    return
  }
  let dragId = null
  const rows = list.map((m) => {
    const row = h('div', { class: 'macro', title: m.command, draggable: scope === 'all' ? 'true' : 'false' },
      h('span', { class: 'name' }, m.name),
      h('button', {
        class: 'mini' + (m.pinned ? ' on' : ''), title: m.pinned ? 'Desafixar' : 'Fixar no topo',
        onclick: async () => {
          try { await Api.SaveMacro({ ...m, pinned: !m.pinned }); refreshMacros() } catch (e) { toast(cleanErr(e), true) }
        },
      }, icon('pin', 13)),
      h('button', { class: 'mini', title: 'Executar na aba ativa', onclick: () => runMacro(m, false) }, icon('play', 13)),
      h('button', { class: 'mini', title: 'Executar em todas as abas abertas', onclick: () => runMacro(m, true) }, icon('playall', 13)),
      h('button', { class: 'mini', title: 'Editar', onclick: () => editMacro(m, refreshMacros) }, icon('pencil', 13)))
    if (scope === 'all') {
      row.addEventListener('dragstart', (e) => { dragId = m.id; e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', m.id) })
      row.addEventListener('dragover', (e) => { if (dragId && dragId !== m.id) { e.preventDefault(); row.classList.add('drop') } })
      row.addEventListener('dragleave', () => row.classList.remove('drop'))
      row.addEventListener('drop', async (e) => {
        e.preventDefault()
        row.classList.remove('drop')
        if (!dragId || dragId === m.id) return
        const ids = macros.map((x) => x.id).filter((x) => x !== dragId)
        ids.splice(ids.indexOf(m.id), 0, dragId)
        dragId = null
        try { await Api.ReorderMacros(ids); refreshMacros() } catch (err) { toast(cleanErr(err), true) }
      })
    }
    return row
  })
  box.replaceChildren(head, ...rows)
}

async function exportMacros() {
  if (!macros.length) { toast('Não há macros para exportar.', true); return }
  try {
    const data = macros.map(({ name, command, enter, pinned, tags }) => ({ name, command, enter, pinned: !!pinned, tags: tags || [] }))
    const p = await Api.ExportText('nxaterm-macros.json', JSON.stringify(data, null, 2))
    if (p) toast('Macros exportadas')
  } catch (e) { toast(cleanErr(e), true) }
}

async function importMacros() {
  try {
    const f = await Api.ImportText('Escolha o arquivo de macros')
    if (!f || !f.name) return
    const arr = JSON.parse(f.content)
    if (!Array.isArray(arr)) throw new Error('arquivo de macros inválido')
    let n = 0
    for (const m of arr) {
      if (!m || typeof m.name !== 'string' || typeof m.command !== 'string') continue
      await Api.SaveMacro({ id: '', name: m.name, command: m.command, enter: m.enter !== false, pinned: !!m.pinned, tags: Array.isArray(m.tags) ? m.tags : [] })
      n++
    }
    await refreshMacros()
    toast(`${n} macro${n === 1 ? '' : 's'} importada${n === 1 ? '' : 's'}`)
  } catch (e) { toast(cleanErr(e), true) }
}

function runMacro(m, everywhere) {
  const targets = everywhere ? aliveTabs() : (activeTab() && !activeTab().dead ? [activeTab()] : [])
  if (!targets.length) return toast('Nenhuma sessão aberta.', true)
  const text = m.command.replace(/\r?\n/g, '\r') + (m.enter ? '\r' : '')
  for (const t of targets) Api.Write(t.id, text).catch(() => {})
  toast(`Macro "${m.name}" enviada para ${targets.length} aba(s)`)
}

// ------------------------------------------------------------------ espaços de trabalho e bloqueio
function openWorkspaces() {
  workspaceDialog(
    () => ({
      items: allTabs().filter((t) => t.session && t.kind !== 'home').map((t) => ({ ...t.session })),
      layout,
    }),
    async (w) => {
      if (w.layout && LAYOUTS[w.layout]) setLayout(w.layout)
      for (const item of w.items) {
        // sessão salva: usa a versão atual (a senha nunca é guardada)
        const cur = item.id ? sessions.find((x) => x.id === item.id) : null
        await openSession(cur || item)
      }
    })
}

async function lockNow() {
  let has = false
  try { has = await Api.HasLock() } catch { /* sem bloqueio */ }
  if (!has) { toast('Defina uma senha de abertura em Configurações > Sistema', true); return }
  await lockGate()
}

// ------------------------------------------------------------------ início
function renderHome() {
  const home = $('home')
  const recent = (settings.recent || []).map((id) => sessions.find((x) => x.id === id)).filter(Boolean).slice(0, 8)
  const tile = (ico, label, color, fn) => h('button', { class: 'tile', onclick: fn },
    h('span', { style: `color:${color}` }, icon(ico, 26)), label)
  home.replaceChildren(h('div', { class: 'home-wrap' },
    h('div', { class: 'home-logo' }, icon('terminal', 30), h('span', {}, 'NxATerm')),
    h('div', { class: 'tiles' },
      tile('session', 'Nova sessão', '#4aa8ff', () => editSession(null, refreshSessions)),
      tile('shell', 'Shell local', 'var(--g-cccccc)', openLocalShell),
      tile('upload', 'Importar sessões', '#f5a43a', importFile)),
    recent.length
      ? h('div', { class: 'recent' }, h('div', { class: 'recent-title' }, 'Recentes'),
        recent.map((x) => {
          const info = TYPE_INFO[typeOf(x)]
          const row = h('div', { class: 'recent-row', onclick: () => { hideTip(); openSession(x) } },
            h('span', { style: `color:${info.color}`, class: 'sess-ico' }, icon(info.icon, 15)), h('span', { class: 'name' }, x.name))
          attachTip(row, () => [h('div', { class: 'tip-title' }, x.name), h('div', { class: 'tip-line' }, info.label + ' · ' + sessionSummary(x))])
          return row
        }))
      : null))
}

async function openLocalShell() {
  try { await startTab({ id: '', type: 'shell', name: 'Shell local', shell: '' }, '') } catch (e) { toast(cleanErr(e), true) }
}

function rememberRecent(s) {
  if (!s.id) return
  settings.recent = [s.id, ...(settings.recent || []).filter((x) => x !== s.id)].slice(0, 12)
  saveSettings()
  renderHome()
}

// ------------------------------------------------------------------ abas / terminal

async function askSecret(title, label, key) {
  const r = await promptBox({ title, label, secret: true, ok: 'Conectar', remember: 'Memorizar até fechar o NxATerm' })
  return r && { value: r.value, remember: r.remember, key }
}

async function openExternal(s) {
  let password = ''
  // o cliente RDP do Linux aceita a senha pela entrada padrão; no Windows e no macOS o próprio cliente pergunta
  if (typeOf(s) === 'rdp' && !isWindows && /Linux|X11/.test(navigator.userAgent)) {
    const key = memoKey('pw', s)
    if (memo.has(key)) password = memo.get(key)
    else {
      const r = await promptBox({ title: 'Senha', label: `Senha de ${s.user || s.host} (opcional)`, secret: true, ok: 'Continuar', remember: 'Memorizar até fechar o NxATerm' })
      if (r === null) return
      password = r.value
      if (r.remember && password) memo.set(key, password)
    }
  }
  try {
    await Api.OpenRemote(s, password)
    recordHistory(s)
    rememberRecent(s)
  } catch (e) { toast(cleanErr(e), true) }
}

async function openSession(s) {
  if (opensExternally(s)) return openExternal(s)
  let secret = ''
  let save = null // chave onde guardar a senha depois que a conexão der certo
  let fromMemo = false
  if (hasFiles(s) && s.authMethod === 'password' && !(typeOf(s) === 'ftp' && !s.user)) {
    const key = memoKey('pw', s)
    if (memo.has(key)) { secret = memo.get(key); fromMemo = true } else {
      const r = await askSecret('Senha', `Senha de ${s.user}@${s.host}`, key)
      if (!r) return
      secret = r.value
      if (r.remember) save = key
    }
  }
  try {
    await startTab(s, secret)
    if (save) memo.set(save, secret)
    recordHistory(s)
    rememberRecent(s)
  } catch (e) {
    const msg = String(e)
    if (fromMemo && /unable to authenticate/.test(msg)) {
      memo.delete(memoKey('pw', s))
      toast('A senha memorizada foi recusada', true)
      return openSession(s)
    }
    if (msg.includes('PASSPHRASE_REQUIRED')) {
      const key = memoKey('pp', s)
      let pass = memo.get(key)
      let keep = false
      if (pass === undefined) {
        const r = await askSecret('Chave protegida', 'Senha da chave privada', key)
        if (!r) return
        pass = r.value
        keep = r.remember
      }
      try {
        await startTab(s, pass)
        if (keep) memo.set(key, pass)
        rememberRecent(s)
      } catch (e2) {
        memo.delete(key)
        toast(cleanErr(e2), true)
      }
    } else {
      toast(cleanErr(e), true)
    }
  }
}

async function startTab(s, secret) {
  const prevActive = activeId
  const kind = typeOf(s)
  const isTerm = hasTerminal(s)
  const isVnc = kind === 'vnc'
  let vncView = null
  const id = 'tab' + (++tabCounter) + '-' + Math.random().toString(36).slice(2, 8)
  const num = ++tabNumber
  const label = `${num}. ${s.name}`

  let term = null
  let fit = null
  let search = null
  let host = null
  let excl = null
  let pane
  if (isTerm) {
    const opts = termOptions()
    if (kind === 'serial') opts.convertEol = true // equipamentos seriais costumam mandar só LF
    term = new Terminal(opts)
    fit = new FitAddon()
    search = new SearchAddon()
    term.loadAddon(fit)
    term.loadAddon(search)
    term.loadAddon(new WebLinksAddon((_e, uri) => openURL(uri)))
    if (settings.unicode11) {
      try { term.loadAddon(new Unicode11Addon()); term.unicode.activeVersion = '11' } catch { /* segue com Unicode 6 */ }
    }
    if (settings.inlineImages) {
      try { term.loadAddon(new ImageAddon()) } catch { /* sem imagens */ }
    }

    host = h('div', { class: 'term-host', id: 'drop-t-' + id, 'data-file-drop-target': true })
    excl = h('input', { type: 'checkbox' })
    excl.addEventListener('change', () => { excl.checked ? me.off.add(id) : me.off.delete(id); renderMultiexec() })
    pane = h('div', { class: 'pane term hidden', style: `background:${opts.theme.background}` },
      h('div', { class: 'pane-head' }, h('span', { class: 'pane-title' }, label)),
      host,
      h('label', { class: 'pane-foot' }, excl, ` Excluir "${label}" do modo MultiExec`))
  } else if (isVnc) {
    vncView = createVncView(label)
    pane = vncView.pane
  } else {
    pane = h('div', { class: 'pane files hidden' }, h('div', { class: 'pane-head' }, h('span', { class: 'pane-title' }, label)))
  }
  pane.addEventListener('mousedown', () => { if (activeId !== id) activate(id) }, true)
  $('stage').append(pane)

  const tabEl = h('div', { class: 'tab', onclick: () => activate(id) },
    h('span', { class: 'tab-ico', style: `color:${TYPE_INFO[kind].color}` }, icon(TYPE_INFO[kind].tabIcon, 14)),
    h('span', { class: 'label' }, label),
    h('span', { class: 'x', title: 'Fechar aba', onclick: (e) => { e.stopPropagation(); closeTab(id) } }, '×'))
  if (s.color) { tabEl.dataset.color = s.color; tabEl.style.setProperty('--sc', s.color) }
  tabEl.addEventListener('mousedown', (e) => { if (e.button === 1) { e.preventDefault(); closeTab(id) } })
  tabEl.addEventListener('contextmenu', (e) => {
    e.preventDefault()
    const t = tabs.get(id)
    showMenu(e.clientX, e.clientY, [
      { label: 'Duplicar sessão', kbd: keyOf('duplicate'), onclick: () => duplicateTab(t) },
      { label: 'Reconectar', disabled: !t.dead, onclick: () => reconnectTab(t) },
      { sep: true },
      { label: 'Fechar aba', onclick: () => closeTab(id) },
      { label: 'Fechar as outras abas', onclick: () => allTabs().filter((x) => x.id !== id).forEach((x) => closeTab(x.id)) },
    ])
  })
  $('tabbar').append(tabEl)

  const tab = {
    id, kind, session: s, secret, label, term, fit, search, pane, host, tabEl, excl,
    dead: false, ready: false, unsub: [], searchBar: null, remote: null, local: null,
    stats: null, statsFails: 0, statsBusy: false, noStats: false,
    followInstalled: false, cwd: '', vnc: vncView ? vncView.ctl : null,
  }
  tabs.set(id, tab)
  activate(id) // precisa estar visível para o fit() calcular colunas/linhas
  if (isTerm) {
    term.open(host)
    if (settings.webgl) {
      try {
        const gl = new WebglAddon()
        gl.onContextLoss(() => gl.dispose())
        term.loadAddon(gl)
      } catch { /* sem GPU: fica no renderizador padrão */ }
    }
    try { fit.fit() } catch { /* ainda sem tamanho */ }
    tab.hl = attachHighlight(term)
    // listeners ANTES de conectar, para não perder o prompt inicial
    if (settings.zmodem && !s.encoding && ['ssh', 'serial', 'wsl', 'shell'].includes(kind)) {
      tab.zm = createZmodem({ id, term, pane })
    }
    tab.unsub.push(Events.On('term:data:' + id, (ev) => {
      const bytes = b64ToBytes(payload(ev))
      if (tab.zm) tab.zm.consume(bytes)
      else term.write(bytes)
    }))
  }
  tab.unsub.push(Events.On('term:closed:' + id, (ev) => markDead(id, payload(ev))))

  try {
    if (isVnc) await connectVnc({ session: s, host: vncView.host, ctl: vncView.ctl, onClosed: (reason) => markDead(id, reason) })
    else await Api.Connect(id, s, secret, term ? term.cols : 0, term ? term.rows : 0)
  } catch (e) {
    disposeTab(id)
    activate(prevActive)
    throw e
  }

  tab.ready = true
  if (isTerm) {
    wireTerminal(tab)
    if (kind === 'ssh') {
      if (settings.autoSftp) showSide('sftp')
      else renderSide()
      if (settings.followFolder) setTimeout(() => installFollow(tab), 700)
    }
    term.focus()
  } else if (!isVnc) {
    buildFileTab(tab)
  }
  renderTree()
  renderMultiexec()
  pollStats()
}

function wireTerminal(tab) {
  const { term, host, id } = tab

  term.onData((d) => {
    if (tab.dead) { if (d === '\r') reconnectTab(tab); return }
    if (tab.zm && tab.zm.active) return // durante o Zmodem o teclado não vai ao servidor
    for (const t of broadcastTargets(tab)) Api.Write(t.id, d).catch(() => {})
  })
  term.onResize(({ cols, rows }) => { if (!tab.dead) Api.Resize(id, cols, rows).catch(() => {}) })

  let raf = 0
  new ResizeObserver(() => {
    cancelAnimationFrame(raf)
    raf = requestAnimationFrame(() => { if (!tab.pane.classList.contains('hidden')) { try { tab.fit.fit() } catch { /* oculto */ } } })
  }).observe(host)

  term.onSelectionChange(() => {
    if (settings.copyOnSelect && term.hasSelection()) copyText(term.getSelection())
  })

  // OSC 7: o shell informa a pasta atual (usado por "Seguir pasta do terminal")
  term.parser.registerOscHandler(7, (data) => {
    const m = /^file:\/\/[^/]*(\/.*)$/.exec(data)
    if (m) {
      tab.cwd = m[1]
      if (tab.kind === 'ssh' && settings.followFolder && sideView === 'sftp' && tab.id === activeId) followCwd(tab)
    }
    return true
  })

  term.attachCustomKeyEventHandler((e) => {
    // atalhos globais são tratados no document; o xterm não deve mandá-los ao servidor
    if (e.type === 'keydown' && isShortcut(e)) return false
    return true
  })

  host.addEventListener('contextmenu', (e) => {
    e.preventDefault()
    if (settings.rightClick === 'paste') {
      pasteInto(tab)
    } else {
      showMenu(e.clientX, e.clientY, [
        { label: 'Copiar', kbd: keyOf('copy'), disabled: !term.hasSelection(), onclick: () => copyText(term.getSelection()) },
        { label: 'Colar', kbd: keyOf('paste'), onclick: () => pasteInto(tab) },
        { label: 'Selecionar tudo', onclick: () => term.selectAll() },
        { sep: true },
        { label: 'Buscar...', kbd: keyOf('search'), onclick: () => toggleSearch(tab) },
        { label: 'Limpar tela', onclick: () => term.clear() },
        { label: 'Duplicar sessão', onclick: () => duplicateTab(tab) },
      ])
    }
  })
}

function pasteInto(tab) {
  if (!tab || !tab.term) return
  navigator.clipboard.readText().then((t) => { if (t) tab.term.paste(t) }).catch(() => {})
}

function applyTermSettings(tab) {
  if (!tab.term) return
  const o = termOptions()
  for (const [k, v] of Object.entries(o)) tab.term.options[k] = v
  tab.pane.style.background = o.theme.background
  tab.hl?.refresh()
  try { tab.fit.fit() } catch { /* oculto */ }
}

function zoomFont(delta) {
  settings.fontSize = Math.min(32, Math.max(8, settings.fontSize + delta))
  for (const t of allTabs()) applyTermSettings(t)
  saveSettings()
}

function markDead(id, reason) {
  const t = tabs.get(id)
  if (!t || t.dead) return
  t.dead = true
  t.tabEl.classList.add('dead')
  if (t.term) {
    t.term.write('\r\n\x1b[33m[conexão encerrada' + (reason ? ': ' + reason : '') + ' — pressione Enter para reconectar]\x1b[0m\r\n')
  }
  t.deadReason = reason
  if (t.vnc) toast('Conexão VNC encerrada' + (reason ? ': ' + reason : ''))
  t.remote?.render()
  renderTree()
  renderMultiexec()
  if (id === activeId) { renderSide(); renderStatus() }
}

function closeTab(id) {
  const t = tabs.get(id)
  if (!t) return
  if (!t.dead && t.kind !== 'vnc') Api.Close(id).catch(() => {})
  const order = [...tabs.keys()]
  const idx = order.indexOf(id)
  disposeTab(id)
  if (activeId === id) {
    const rest = [...tabs.keys()]
    activate(rest.length ? rest[Math.min(idx, rest.length - 1)] : 'home')
  } else {
    applyLayout()
  }
  renderTree()
  renderMultiexec()
}

function disposeTab(id) {
  const t = tabs.get(id)
  if (!t) return
  t.unsub.forEach((u) => { try { u() } catch { /* já removido */ } })
  try { t.hl?.stop() } catch { /* ok */ }
  try { t.zm?.dispose() } catch { /* ok */ }
  if (t.vnc) disposeVnc(t.vnc)
  try { if (t.term) t.term.dispose() } catch { /* já removido */ }
  t.pane.remove()
  t.tabEl.classList.add('closing')
  setTimeout(() => t.tabEl.remove(), 160)
  tabs.delete(id)
  me.off.delete(id)
}

async function duplicateTab(t) {
  if (!t) return
  try { await startTab(t.session, t.secret) } catch (e) { toast(cleanErr(e), true) }
}

async function reconnectTab(t) {
  if (!t) return
  const { session, secret } = t
  closeTab(t.id)
  try { await startTab(session, secret) } catch (e) { toast(cleanErr(e), true) }
}

function activate(id) {
  if (id !== 'home' && !tabs.has(id)) id = 'home'
  activeId = id
  $('tab-home').classList.toggle('active', id === 'home')
  for (const t of allTabs()) t.tabEl.classList.toggle('active', t.id === id)
  const t = activeTab()
  if (t) t.tabEl.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  applyLayout()
  renderSide()
  renderStatus()
  renderMultiexec()
  if (settings.macroScope === 'active') renderMacros()
  if (t) {
    if (t.kind === 'ssh' && settings.followFolder && !t.followInstalled && !t.dead) installFollow(t)
    pollStats()
  }
}

function buildTabbar() {
  $('tabbar').append(h('div', { class: 'tab home', id: 'tab-home', title: 'Início', onclick: () => activate('home') }, icon('home', 15)))
}

// ------------------------------------------------------------------ layout (split)
function setLayout(k) {
  layout = k
  applyLayout()
}

function applyLayout() {
  const stage = $('stage')
  const order = [...tabs.keys()]
  const vis = []
  const multi = me.open
  if (activeId !== 'home' && tabs.has(activeId)) {
    if (multi && tabs.get(activeId).term) {
      vis.push(...order.filter((id) => tabs.get(id).term)) // MultiExec mostra todos os terminais lado a lado
    } else if (multi) {
      vis.push(activeId) // aba de arquivos: aparece sozinha
    } else {
      const n = LAYOUTS[layout].n
      const start = Math.floor(order.indexOf(activeId) / n) * n
      vis.push(...order.slice(start, start + n))
    }
  }
  const visSet = new Set(vis)

  $('home').classList.toggle('hidden', activeId !== 'home')
  const split = layout !== 'single' || multi
  for (const t of allTabs()) {
    t.pane.classList.toggle('hidden', !visSet.has(t.id))
    t.pane.classList.toggle('focused', split && t.id === activeId)
  }
  stage.classList.toggle('split', split)
  stage.classList.toggle('multi', multi)

  const count = Math.max(1, vis.length)
  let cols = 1
  let rows = 1
  if (multi) { cols = count <= 3 ? count : Math.ceil(Math.sqrt(count)); rows = Math.ceil(count / cols) }
  else if (layout === 'cols') cols = count
  else if (layout === 'rows') rows = count
  else if (layout === 'grid') { cols = count <= 1 ? 1 : 2; rows = Math.ceil(count / cols) }
  stage.style.gridTemplateColumns = `repeat(${cols}, minmax(0, 1fr))`
  stage.style.gridTemplateRows = `repeat(${rows}, minmax(0, 1fr))`

  requestAnimationFrame(() => {
    for (const id of vis) { try { tabs.get(id).fit?.fit() } catch { /* oculto */ } }
    const t = activeTab()
    if (t && t.term && !t.pane.classList.contains('hidden') && !modalOpen()) t.term.focus()
  })
}

// ------------------------------------------------------------------ busca no terminal
function toggleSearch(tab) {
  if (!tab || !tab.term) return
  if (tab.searchBar) {
    tab.searchBar.remove()
    tab.searchBar = null
    tab.search.clearDecorations?.()
    tab.term.focus()
    return
  }
  const input = h('input', { type: 'text', placeholder: 'Buscar no terminal...', spellcheck: 'false' })
  const next = () => tab.search.findNext(input.value, { caseSensitive: false })
  const prev = () => tab.search.findPrevious(input.value, { caseSensitive: false })
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); e.shiftKey ? prev() : next() }
    if (e.key === 'Escape') { e.preventDefault(); toggleSearch(tab) }
  })
  input.addEventListener('input', next)
  tab.searchBar = h('div', { class: 'search-bar' }, input,
    h('button', { class: 'mini', title: 'Anterior (Shift+Enter)', onclick: prev }, '↑'),
    h('button', { class: 'mini', title: 'Próximo (Enter)', onclick: next }, '↓'),
    h('button', { class: 'mini', title: 'Fechar (Esc)', onclick: () => toggleSearch(tab) }, icon('close', 13)))
  tab.pane.append(tab.searchBar)
  input.focus()
}

// ------------------------------------------------------------------ MultiExec
function broadcastTargets(tab) {
  if (me.open && !me.off.has(tab.id)) return meTargets()
  return [tab]
}

function meTargets() {
  return aliveTabs().filter((t) => !me.off.has(t.id))
}

function toggleMultiexec() {
  me.open = !me.open
  $('multiexec').classList.toggle('hidden', !me.open)
  $('tb-multiexec').classList.toggle('on', me.open)
  if (me.open) {
    if (!tabs.size) toast('MultiExec ligado')
    else if (activeId === 'home') activate([...tabs.keys()][0])
  }
  renderMultiexec()
  applyLayout()
  renderStatus()
  const t = activeTab()
  if (t && t.term && !t.pane.classList.contains('hidden')) t.term.focus()
}

function renderMultiexec() {
  for (const t of allTabs()) {
    if (!t.excl) continue
    t.excl.checked = me.off.has(t.id)
    t.pane.classList.toggle('excluded', me.off.has(t.id))
  }
  if (!me.open) return
  $('multiexec').replaceChildren(
    h('span', { class: 'me-title' }, icon('multiexec', 18)),
    h('span', { class: 'me-text' }, 'MultiExec ativo'),
    h('span', { class: 'me-count' }, `${meTargets().length} de ${aliveTabs().length} recebendo`),
    h('span', { class: 'grow' }),
    h('button', { class: 'me-btn', onclick: multiPasteDialog }, icon('send', 15), ' Multi-colar'),
    h('button', { class: 'me-btn exit', onclick: toggleMultiexec }, icon('close', 15), ' Sair'))
}

function multiPasteDialog() {
  if (!meTargets().length) return toast('Nenhum terminal está recebendo o MultiExec.', true)
  const ta = h('textarea', { rows: 9, spellcheck: 'false' })
  const enter = h('input', { type: 'checkbox', checked: true })
  const send = () => {
    if (!ta.value) return closeModal()
    const text = ta.value.replace(/\r?\n/g, '\r') + (enter.checked && !/\n$/.test(ta.value) ? '\r' : '')
    const targets = meTargets()
    for (const t of targets) Api.Write(t.id, text).catch(() => {})
    closeModal()
    toast(`Texto enviado para ${targets.length} terminal(is)`)
  }
  openModal([
    h('h2', {}, 'Multi-colar'),
    ta,
    h('label', { class: 'check' }, enter, ' Enviar Enter no final'),
    h('div', { class: 'btns' },
      h('button', { onclick: closeModal }, 'Cancelar'),
      h('button', { class: 'primary', onclick: send }, 'Enviar para todos')),
  ], 'wide')
  ta.focus()
}

// ------------------------------------------------------------------ seguir pasta do terminal
// Configura o bash para avisar a pasta atual a cada prompt (sequência OSC 7).
// O espaço inicial evita que o comando entre no histórico (HISTCONTROL=ignorespace).
const FOLLOW_CMD = ' PROMPT_COMMAND=\'printf "\\033]7;file://%s%s\\a" "$HOSTNAME" "$PWD"\'"${PROMPT_COMMAND:+;$PROMPT_COMMAND}"\r'

function installFollow(tab) {
  if (tab.kind !== 'ssh' || tab.followInstalled || tab.dead) return
  tab.followInstalled = true
  Api.Write(tab.id, FOLLOW_CMD).catch(() => {})
}

let followTimer
function followCwd(tab) {
  clearTimeout(followTimer)
  followTimer = setTimeout(() => {
    const r = tab.remote
    if (r && tab.cwd && tab.cwd !== r.st.path && !tab.dead) r.load(tab.cwd, { fromTerm: true })
  }, 250)
}

function syncFollow() {
  const t = activeTab()
  if (!settings.followFolder || !t || t.dead || t.kind !== 'ssh') return
  installFollow(t)
  if (t.cwd) followCwd(t)
}

// ------------------------------------------------------------------ barra de status (monitoramento)
const SB = {}

function buildStatusbar() {
  const seg = (key, ico, title, extra = '') => {
    SB[key] = h('span', { class: 'sb-val' }, '—')
    return h('span', { class: 'sb-seg ' + extra, title }, ico ? icon(ico, 14) : null, SB[key])
  }
  SB.cpuBar = h('span', { class: 'bar-fill' })
  SB.name = h('span', { class: 'sb-name' }, 'NxATerm')
  const sb = $('statusbar')
  sb.append(
    h('span', { class: 'sb-seg sb-host' }, SB.name),
    h('span', { class: 'sb-seg sb-mon', title: 'Uso de CPU do servidor' }, icon('cpu', 14), h('span', { class: 'bar' }, SB.cpuBar), (SB.cpu = h('span', { class: 'sb-val' }, '—'))),
    seg('mem', 'mem', 'Memória usada / total', 'sb-mon'),
    h('span', { class: 'sb-seg sb-mon', title: 'Rede: envio ↑ e recebimento ↓' }, (SB.up = h('span', { class: 'sb-val' }, '—')), (SB.down = h('span', { class: 'sb-val' }, '—'))),
    seg('uptime', 'clock', 'Tempo ligado', 'sb-mon'),
    seg('user', 'user', 'Usuário'),
    seg('disk', 'disk', 'Uso do disco (/)', 'sb-mon'),
    h('span', { class: 'sb-fill' }),
    h('span', { class: 'sb-seg muted', title: 'Layout' }, (SB.layout = h('span', { class: 'sb-val' }, ''))))
}

function renderStatus() {
  const t = activeTab()
  SB.layout.textContent = me.open ? 'MultiExec' : layout === 'single' ? '' : LAYOUTS[layout].label
  SB.name.textContent = t ? t.session.name : 'NxATerm'
  const s = t && !t.dead && settings.monitoring ? t.stats : null
  const dash = '—'
  SB.cpu.textContent = s ? Math.round(s.cpu) + '%' : dash
  SB.cpuBar.style.width = s ? Math.round(s.cpu) + '%' : '0%'
  const fmt = (n) => n.toFixed(2).replace('.', ',')
  SB.mem.textContent = s && s.memTotal ? `${fmt(s.memUsed)} GB / ${fmt(s.memTotal)} GB` : dash
  SB.up.textContent = s ? '↑ ' + fmtRate(s.netUp) : dash
  SB.down.textContent = s ? '↓ ' + fmtRate(s.netDown) : ''
  SB.uptime.textContent = s && s.uptimeSec ? fmtUptime(s.uptimeSec) : dash
  SB.user.textContent = s && s.user ? s.user : (t && !t.dead && t.session.user ? t.session.user : dash)
  SB.disk.textContent = s ? '/: ' + s.diskPct + '%' : dash
  document.querySelectorAll('.sb-mon').forEach((e) => e.classList.toggle('hidden', !s))
  SB.user.parentElement.classList.toggle('hidden', !(s || (t && !t.dead && t.session.user)))
}

async function pollStats() {
  const t = activeTab()
  if (!t || t.kind !== 'ssh' || t.dead || !t.ready || !settings.monitoring || t.noStats || t.statsBusy) return
  t.statsBusy = true
  try {
    t.stats = await Api.Stats(t.id)
    t.statsFails = 0
  } catch {
    if (++t.statsFails >= 3) t.noStats = true // provavelmente não é Linux
  }
  t.statsBusy = false
  if (t.id === activeId) renderStatus()
}

// ------------------------------------------------------------------ atalhos
function isShortcut(e) {
  const id = actionFor(e)
  if (!id) return false
  // copiar/colar dentro de campos de texto da interface (não do terminal) é o normal do navegador
  const el = e.target
  const inField = el && /^(INPUT|TEXTAREA)$/.test(el.tagName) && !el.classList.contains('xterm-helper-textarea')
  return !(inField && (id === 'copy' || id === 'paste' || id === 'paste2'))
}

function handleShortcut(e) {
  const t = activeTab()
  switch (actionFor(e)) {
    case 'newSession': editSession(null, refreshSessions); return true
    case 'quick': showSide('sessions'); $('quick').focus(); return true
    case 'localShell': openLocalShell(); return true
    case 'duplicate': if (t) duplicateTab(t); return true
    case 'closeTab': if (t) closeTab(t.id); return true
    case 'prevTab':
    case 'nextTab': {
      const order = ['home', ...tabs.keys()]
      const step = actionFor(e) === 'nextTab' ? 1 : order.length - 1
      activate(order[(order.indexOf(activeId) + step) % order.length])
      return true
    }
    case 'search': if (t && t.term) toggleSearch(t); return true
    case 'multiexec': toggleMultiexec(); return true
    case 'copy': if (t && t.term && t.term.hasSelection()) copyText(t.term.getSelection()); return true
    case 'paste':
    case 'paste2': if (t) pasteInto(t); return true
    case 'sidebar': toggleSidebar(); return true
    case 'tunnels': openTunnels(); return true
    case 'settings': openSettings(); return true
    default: return false
  }
}

// ------------------------------------------------------------------ conexão rápida / init
function parseQuick(text) {
  const m = text.trim().match(/^([^@\s]+)@([^:\s]+|\[[0-9a-fA-F:]+\])(?::(\d{1,5}))?$/)
  if (!m) return null
  return { id: '', type: 'ssh', name: m[1] + '@' + m[2], folder: '', host: m[2].replace(/^\[|\]$/g, ''), port: m[3] ? parseInt(m[3], 10) : 22, user: m[1], authMethod: 'password', keyPath: '', startupCommand: '', notes: '' }
}

function initEvents() {
  $('quick').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); connectQuick() }
    if (e.key === 'Escape') { e.target.value = ''; renderTree(); e.target.blur() }
  })
  $('quick').addEventListener('input', () => {
    if (sideView !== 'sessions') showSide('sessions')
    renderTree()
  })
  $('tree').addEventListener('contextmenu', (e) => {
    if (e.target.closest('.folder-head, .sess')) return
    e.preventDefault()
    showMenu(e.clientX, e.clientY, treeMenu(''))
  })
  $('tree').addEventListener('dragover', (e) => { if (dragSession) e.preventDefault() })
  $('tree').addEventListener('drop', (e) => { if (!e.target.closest('.folder-head')) { e.preventDefault(); moveSession(dragSession, '') } })
  $('btn-macro-new').addEventListener('click', () => editMacro(null, refreshMacros))

  document.addEventListener('keydown', (e) => {
    if (capture.active) return
    if (e.key === 'Escape' && modalOpen()) { if (!comboOpen()) closeModal(); return }
    if (modalOpen()) return
    if (isShortcut(e) && handleShortcut(e)) { e.preventDefault(); e.stopPropagation() }
  }, true)

  // Ctrl + roda do mouse = zoom da fonte
  $('stage').addEventListener('wheel', (e) => {
    if (!e.ctrlKey) return
    e.preventDefault()
    zoomFont(e.deltaY < 0 ? 1 : -1)
  }, { passive: false })

  // clique no fundo fecha menus abertos pela barra
  window.addEventListener('blur', closeMenu)

  setInterval(pollStats, 3000)
}

async function init() {
  await loadSettings()
  setLang(settings.lang)
  setStartupTransparent(settings.windowTransparent)
  applyAppearance()
  await lockGate()
  if (settings.hasBg) Api.GetBackground().then((d) => setBackgroundData(d)).catch(() => {})
  $('workspace').style.setProperty('--sidew', settings.sidebarWidth + 'px')
  $('statusbar').classList.toggle('hidden', !settings.showStatusBar)

  buildMenubar()
  buildToolbar()
  buildSidetabs()
  buildTabbar()
  buildStatusbar()
  initResizer()
  initEvents()
  showSide('sessions')
  activate('home')

  await initFiles({ getTab: (id) => tabs.get(id), activeTab, allTabs, reconnectTab, copyText })
  await Promise.allSettled([refreshSessions(), refreshMacros()])
  await initConnect(openSession)
  initAi({ activeTab, allTabs, openSession, sessions: () => sessions })
  setTimeout(() => syncOnOpen(syncHooks()), 2500)
  if (settings.checkUpdates) setTimeout(() => checkUpdates({ silent: true, openURL }), 6000)
}

init().catch((e) => toast(cleanErr(e), true))
