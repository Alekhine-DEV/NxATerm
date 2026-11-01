import { Api } from './api.js'

export const FONTS = {
  'Consolas': 'Consolas, "Cascadia Mono", "DejaVu Sans Mono", monospace',
  'Cascadia Mono': '"Cascadia Mono", Consolas, "DejaVu Sans Mono", monospace',
  'Courier New': '"Courier New", Courier, monospace',
  'Lucida Console': '"Lucida Console", Consolas, monospace',
}

export const THEMES = {
  Padrão: {
    background: '#000000', foreground: '#e6e6e6', cursor: '#e6e6e6', selectionBackground: '#3b5b8f',
    black: '#000000', red: '#e5534b', green: '#4ec24e', yellow: '#e6c84a', blue: '#4e8cff', magenta: '#d670d6', cyan: '#4fd1d1', white: '#e6e6e6',
    brightBlack: '#7f7f7f', brightRed: '#ff6b63', brightGreen: '#6be06b', brightYellow: '#fff06a', brightBlue: '#7aa7ff', brightMagenta: '#f08cf0', brightCyan: '#7aeaea', brightWhite: '#ffffff',
  },
  Dracula: {
    background: '#282a36', foreground: '#f8f8f2', cursor: '#f8f8f2', selectionBackground: '#44475a',
    black: '#21222c', red: '#ff5555', green: '#50fa7b', yellow: '#f1fa8c', blue: '#bd93f9', magenta: '#ff79c6', cyan: '#8be9fd', white: '#f8f8f2',
    brightBlack: '#6272a4', brightRed: '#ff6e6e', brightGreen: '#69ff94', brightYellow: '#ffffa5', brightBlue: '#d6acff', brightMagenta: '#ff92df', brightCyan: '#a4ffff', brightWhite: '#ffffff',
  },
  'Solarized escuro': {
    background: '#002b36', foreground: '#93a1a1', cursor: '#93a1a1', selectionBackground: '#073642',
    black: '#073642', red: '#dc322f', green: '#859900', yellow: '#b58900', blue: '#268bd2', magenta: '#d33682', cyan: '#2aa198', white: '#eee8d5',
    brightBlack: '#586e75', brightRed: '#cb4b16', brightGreen: '#586e75', brightYellow: '#657b83', brightBlue: '#839496', brightMagenta: '#6c71c4', brightCyan: '#93a1a1', brightWhite: '#fdf6e3',
  },
  Claro: {
    background: '#ffffff', foreground: '#1f1f1f', cursor: '#1f1f1f', selectionBackground: '#b5d5ff',
    black: '#1f1f1f', red: '#c91b1b', green: '#1a7f37', yellow: '#8a6a00', blue: '#0b57d0', magenta: '#a626a4', cyan: '#0e7490', white: '#d4d4d4',
    brightBlack: '#6e6e6e', brightRed: '#e5484d', brightGreen: '#2da44e', brightYellow: '#b08800', brightBlue: '#2f81f7', brightMagenta: '#c13bbf', brightCyan: '#1f9eb7', brightWhite: '#ffffff',
  },
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
  multiInstance: false, // várias janelas do app ao mesmo tempo (vale ao reabrir)
}

export const settings = { ...DEFAULTS }

export async function loadSettings() {
  try {
    const raw = await Api.LoadSettings()
    if (raw) Object.assign(settings, JSON.parse(raw))
  } catch { /* usa os padrões */ }
  // valida o que veio do arquivo
  if (!FONTS[settings.font]) settings.font = DEFAULTS.font
  if (!THEMES[settings.theme]) settings.theme = DEFAULTS.theme
  settings.fontSize = clampNum(settings.fontSize, 8, 32, DEFAULTS.fontSize)
  settings.scrollback = clampNum(settings.scrollback, 500, 200000, DEFAULTS.scrollback)
  settings.sidebarWidth = clampNum(settings.sidebarWidth, 220, 640, DEFAULTS.sidebarWidth)
  if (!['block', 'bar', 'underline'].includes(settings.cursorStyle)) settings.cursorStyle = 'block'
  if (!['paste', 'menu'].includes(settings.rightClick)) settings.rightClick = 'paste'
  if (!Array.isArray(settings.recent)) settings.recent = []
  if (typeof settings.proxy !== 'string') settings.proxy = ''
  settings.multiInstance = !!settings.multiInstance
  if (!settings.keys || typeof settings.keys !== 'object' || Array.isArray(settings.keys)) settings.keys = {}
}

let saveTimer
/** Grava com um pequeno atraso, para juntar várias mudanças seguidas (ex.: arrastar a barra lateral). */
export function saveSettings() {
  clearTimeout(saveTimer)
  saveTimer = setTimeout(() => { Api.SaveSettings(JSON.stringify(settings)).catch(() => {}) }, 400)
}

function clampNum(v, lo, hi, dflt) {
  const n = Number(v)
  return Number.isFinite(n) ? Math.min(hi, Math.max(lo, Math.round(n))) : dflt
}

/** Opções do xterm derivadas das configurações. */
export function termOptions() {
  return {
    fontFamily: FONTS[settings.font],
    fontSize: settings.fontSize,
    scrollback: settings.scrollback,
    theme: THEMES[settings.theme],
    cursorStyle: settings.cursorStyle,
    cursorBlink: settings.cursorBlink,
  }
}
