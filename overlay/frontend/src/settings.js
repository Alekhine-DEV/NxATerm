import { Api } from './api.js'
import { BUILTIN_THEMES, cleanTheme } from './termthemes.js'
import { paneBackground, termSeeThrough } from './uitheme.js'

export const FONTS = {
  'Consolas': 'Consolas, "Cascadia Mono", "DejaVu Sans Mono", monospace',
  'Cascadia Mono': '"Cascadia Mono", Consolas, "DejaVu Sans Mono", monospace',
  'Courier New': '"Courier New", Courier, monospace',
  'Lucida Console': '"Lucida Console", Consolas, monospace',
}

/** Temas do terminal: os que vêm no app mais os criados/importados pelo usuário. */
export function allTermThemes() {
  return { ...BUILTIN_THEMES, ...(settings.customThemes || {}) }
}

export const DEFAULTS = {
  font: 'Consolas',
  fontSize: 14,
  scrollback: 10000,
  theme: 'Padrão',
  cursorStyle: 'block', // block | bar | underline
  cursorBlink: true,
  copyOnSelect: true,
  rightClick: 'paste', // paste | menu
  sidebarWidth: 260,
  showStatusBar: true,
  monitoring: true,
  followFolder: false,
  autoSftp: true,
  recent: [], // ids das últimas sessões abertas (tela inicial)
  keys: {}, // atalhos personalizados: { idDaAção: 'Ctrl+Shift+T' }
  proxy: '', // proxy geral das conexões (socks5://host:porta ou http://host:porta)
  pathFavs: {}, // pastas favoritas por servidor ('usuário@host:porta') e 'local'
  editor: '', // programa para editar arquivos remotos (vazio = padrão do sistema)
  termSync: false, // levar o terminal junto ao navegar no painel de arquivos
  multiInstance: false, // várias janelas do app ao mesmo tempo (vale ao reabrir)
  uiTheme: 'Escuro', // tema da interface
  uiThemes: {}, // temas da interface criados pelo usuário: { nome: {base,hue,sat,accent} }
  customThemes: {}, // temas do terminal criados/importados: { nome: {background,...} }
  customCss: '', // CSS extra do usuário
  hasBg: false, // existe imagem de fundo guardada
  bgOpacity: 0.35, // visibilidade da imagem de fundo
  bgBlur: 0, // desfoque da imagem de fundo (px)
  termOpacity: 1, // opacidade do fundo do terminal
  windowTransparent: false, // janela transparente (vale ao reabrir)
  uiOpacity: 0.9, // opacidade da interface com a janela transparente
  unicode11: true, // largura correta de emojis e caracteres largos
  webgl: false, // renderizador acelerado por GPU
  zmodem: true, // transferências rz/sz pelo terminal
  inlineImages: true, // mostra imagens no terminal (sixel e protocolo do iTerm2)
  highlight: { enabled: false, rules: [] }, // realce de palavras: rules = [{ pattern, color, regex }]
}

export const settings = { ...DEFAULTS }

export async function loadSettings() {
  try {
    const raw = await Api.LoadSettings()
    if (raw) Object.assign(settings, JSON.parse(raw))
  } catch { /* usa os padrões */ }
  // valida o que veio do arquivo
  if (!FONTS[settings.font]) settings.font = DEFAULTS.font
  for (const k of ['uiThemes', 'customThemes']) {
    if (!settings[k] || typeof settings[k] !== 'object' || Array.isArray(settings[k])) settings[k] = {}
  }
  for (const [k, v] of Object.entries(settings.customThemes)) settings.customThemes[k] = cleanTheme(v)
  if (!allTermThemes()[settings.theme]) settings.theme = DEFAULTS.theme
  if (typeof settings.uiTheme !== 'string') settings.uiTheme = DEFAULTS.uiTheme
  if (typeof settings.customCss !== 'string') settings.customCss = ''
  settings.hasBg = !!settings.hasBg
  settings.bgOpacity = clampF(settings.bgOpacity, 0.05, 1, DEFAULTS.bgOpacity)
  settings.bgBlur = clampNum(settings.bgBlur, 0, 30, 0)
  settings.termOpacity = clampF(settings.termOpacity, 0.2, 1, 1)
  settings.uiOpacity = clampF(settings.uiOpacity, 0.4, 1, DEFAULTS.uiOpacity)
  for (const k of ['windowTransparent', 'webgl']) settings[k] = !!settings[k]
  for (const k of ['unicode11', 'inlineImages', 'zmodem']) settings[k] = settings[k] !== false
  const hl = settings.highlight
  settings.highlight = {
    enabled: !!(hl && hl.enabled),
    rules: hl && Array.isArray(hl.rules)
      ? hl.rules.filter((r) => r && typeof r.pattern === 'string' && r.pattern).slice(0, 50)
          .map((r) => ({ pattern: r.pattern.slice(0, 200), color: normHex(r.color), regex: !!r.regex }))
      : [],
  }
  settings.fontSize = clampNum(settings.fontSize, 8, 32, DEFAULTS.fontSize)
  settings.scrollback = clampNum(settings.scrollback, 500, 200000, DEFAULTS.scrollback)
  settings.sidebarWidth = clampNum(settings.sidebarWidth, 220, 640, DEFAULTS.sidebarWidth)
  if (!['block', 'bar', 'underline'].includes(settings.cursorStyle)) settings.cursorStyle = 'block'
  if (!['paste', 'menu'].includes(settings.rightClick)) settings.rightClick = 'paste'
  if (!Array.isArray(settings.recent)) settings.recent = []
  if (typeof settings.proxy !== 'string') settings.proxy = ''
  settings.multiInstance = !!settings.multiInstance
  settings.termSync = !!settings.termSync
  if (typeof settings.editor !== 'string') settings.editor = ''
  if (!settings.pathFavs || typeof settings.pathFavs !== 'object' || Array.isArray(settings.pathFavs)) settings.pathFavs = {}
  if (!settings.keys || typeof settings.keys !== 'object' || Array.isArray(settings.keys)) settings.keys = {}
}

let saveTimer
/** Grava com um pequeno atraso, para juntar várias mudanças seguidas (ex.: arrastar a barra lateral). */
export function saveSettings() {
  clearTimeout(saveTimer)
  saveTimer = setTimeout(() => { Api.SaveSettings(JSON.stringify(settings)).catch(() => {}) }, 400)
}

function clampF(v, lo, hi, dflt) {
  const n = Number(v)
  return Number.isFinite(n) ? Math.min(hi, Math.max(lo, n)) : dflt
}

function normHex(c) { return /^#[0-9a-f]{6}$/i.test(c || '') ? c.toLowerCase() : '#ffd866' }

function clampNum(v, lo, hi, dflt) {
  const n = Number(v)
  return Number.isFinite(n) ? Math.min(hi, Math.max(lo, Math.round(n))) : dflt
}

/** Tema de cores do terminal, com o fundo ajustado à opacidade/imagem de fundo. */
export function termTheme() {
  const t = allTermThemes()[settings.theme] || BUILTIN_THEMES['Padrão']
  return { ...t, background: paneBackground(t.background) }
}

/** Opções do xterm derivadas das configurações. */
export function termOptions() {
  return {
    fontFamily: FONTS[settings.font],
    fontSize: settings.fontSize,
    scrollback: settings.scrollback,
    theme: termTheme(),
    allowTransparency: termSeeThrough(),
    allowProposedApi: true,
    cursorStyle: settings.cursorStyle,
    cursorBlink: settings.cursorBlink,
  }
}
