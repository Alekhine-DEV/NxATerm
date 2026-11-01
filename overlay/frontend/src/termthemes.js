// Temas de cores do terminal: os que vêm no app, e a leitura de temas de outros programas
// (iTerm2 .itermcolors, Windows Terminal e o JSON do próprio NxATerm).

export const ANSI_KEYS = [
  'black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white',
  'brightBlack', 'brightRed', 'brightGreen', 'brightYellow', 'brightBlue', 'brightMagenta', 'brightCyan', 'brightWhite',
]
export const BASE_KEYS = ['background', 'foreground', 'cursor', 'selectionBackground']
export const THEME_KEYS = [...BASE_KEYS, ...ANSI_KEYS]

// [nome, fundo, texto, seleção, [16 cores ANSI]]; o cursor usa a cor do texto
const T = (name, bg, fg, sel, ansi) => {
  const o = { background: bg, foreground: fg, cursor: fg, selectionBackground: sel }
  ANSI_KEYS.forEach((k, i) => { o[k] = ansi[i] })
  return [name, o]
}

export const BUILTIN_THEMES = Object.fromEntries([
  T('Padrão', '#000000', '#e6e6e6', '#3b5b8f', ['#000000', '#e5534b', '#4ec24e', '#e6c84a', '#4e8cff', '#d670d6', '#4fd1d1', '#e6e6e6', '#7f7f7f', '#ff6b63', '#6be06b', '#fff06a', '#7aa7ff', '#f08cf0', '#7aeaea', '#ffffff']),
  T('Claro', '#ffffff', '#1f1f1f', '#b5d5ff', ['#1f1f1f', '#c91b1b', '#1a7f37', '#8a6a00', '#0b57d0', '#a626a4', '#0e7490', '#d4d4d4', '#6e6e6e', '#e5484d', '#2da44e', '#b08800', '#2f81f7', '#c13bbf', '#1f9eb7', '#ffffff']),
  T('Dracula', '#282a36', '#f8f8f2', '#44475a', ['#21222c', '#ff5555', '#50fa7b', '#f1fa8c', '#bd93f9', '#ff79c6', '#8be9fd', '#f8f8f2', '#6272a4', '#ff6e6e', '#69ff94', '#ffffa5', '#d6acff', '#ff92df', '#a4ffff', '#ffffff']),
  T('Solarized escuro', '#002b36', '#93a1a1', '#073642', ['#073642', '#dc322f', '#859900', '#b58900', '#268bd2', '#d33682', '#2aa198', '#eee8d5', '#586e75', '#cb4b16', '#586e75', '#657b83', '#839496', '#6c71c4', '#93a1a1', '#fdf6e3']),
  T('Solarized claro', '#fdf6e3', '#657b83', '#eee8d5', ['#073642', '#dc322f', '#859900', '#b58900', '#268bd2', '#d33682', '#2aa198', '#eee8d5', '#002b36', '#cb4b16', '#586e75', '#657b83', '#839496', '#6c71c4', '#93a1a1', '#fdf6e3']),
  T('Monokai', '#272822', '#f8f8f2', '#49483e', ['#272822', '#f92672', '#a6e22e', '#f4bf75', '#66d9ef', '#ae81ff', '#a1efe4', '#f8f8f2', '#75715e', '#f92672', '#a6e22e', '#f4bf75', '#66d9ef', '#ae81ff', '#a1efe4', '#f9f8f5']),
  T('Nord', '#2e3440', '#d8dee9', '#434c5e', ['#3b4252', '#bf616a', '#a3be8c', '#ebcb8b', '#81a1c1', '#b48ead', '#88c0d0', '#e5e9f0', '#4c566a', '#bf616a', '#a3be8c', '#ebcb8b', '#81a1c1', '#b48ead', '#8fbcbb', '#eceff4']),
  T('One Dark', '#282c34', '#abb2bf', '#3e4451', ['#282c34', '#e06c75', '#98c379', '#e5c07b', '#61afef', '#c678dd', '#56b6c2', '#abb2bf', '#545862', '#e06c75', '#98c379', '#e5c07b', '#61afef', '#c678dd', '#56b6c2', '#c8ccd4']),
  T('Gruvbox escuro', '#282828', '#ebdbb2', '#504945', ['#282828', '#cc241d', '#98971a', '#d79921', '#458588', '#b16286', '#689d6a', '#a89984', '#928374', '#fb4934', '#b8bb26', '#fabd2f', '#83a598', '#d3869b', '#8ec07c', '#ebdbb2']),
  T('Gruvbox claro', '#fbf1c7', '#3c3836', '#d5c4a1', ['#fbf1c7', '#cc241d', '#98971a', '#d79921', '#458588', '#b16286', '#689d6a', '#7c6f64', '#928374', '#9d0006', '#79740e', '#b57614', '#076678', '#8f3f71', '#427b58', '#3c3836']),
  T('Tokyo Night', '#1a1b26', '#c0caf5', '#33467c', ['#15161e', '#f7768e', '#9ece6a', '#e0af68', '#7aa2f7', '#bb9af7', '#7dcfff', '#a9b1d6', '#414868', '#f7768e', '#9ece6a', '#e0af68', '#7aa2f7', '#bb9af7', '#7dcfff', '#c0caf5']),
  T('Catppuccin Mocha', '#1e1e2e', '#cdd6f4', '#45475a', ['#45475a', '#f38ba8', '#a6e3a1', '#f9e2af', '#89b4fa', '#f5c2e7', '#94e2d5', '#bac2de', '#585b70', '#f38ba8', '#a6e3a1', '#f9e2af', '#89b4fa', '#f5c2e7', '#94e2d5', '#a6adc8']),
  T('GitHub escuro', '#0d1117', '#c9d1d9', '#264f78', ['#484f58', '#ff7b72', '#3fb950', '#d29922', '#58a6ff', '#bc8cff', '#39c5cf', '#b1bac4', '#6e7681', '#ffa198', '#56d364', '#e3b341', '#79c0ff', '#d2a8ff', '#56d4dd', '#f0f6fc']),
  T('GitHub claro', '#ffffff', '#24292f', '#b6d6fd', ['#24292f', '#cf222e', '#116329', '#4d2d00', '#0969da', '#8250df', '#1b7c83', '#6e7781', '#57606a', '#a40e26', '#1a7f37', '#633c01', '#218bff', '#a475f9', '#3192aa', '#8c959f']),
  T('Tomorrow Night', '#1d1f21', '#c5c8c6', '#373b41', ['#1d1f21', '#cc6666', '#b5bd68', '#f0c674', '#81a2be', '#b294bb', '#8abeb7', '#c5c8c6', '#969896', '#cc6666', '#b5bd68', '#f0c674', '#81a2be', '#b294bb', '#8abeb7', '#ffffff']),
  T('Material', '#263238', '#eeffff', '#314549', ['#000000', '#ff5370', '#c3e88d', '#ffcb6b', '#82aaff', '#c792ea', '#89ddff', '#ffffff', '#546e7a', '#ff5370', '#c3e88d', '#ffcb6b', '#82aaff', '#c792ea', '#89ddff', '#ffffff']),
  T('Ayu escuro', '#0a0e14', '#b3b1ad', '#253340', ['#01060e', '#ea6c73', '#91b362', '#f9af4f', '#53bdfa', '#fae994', '#90e1c6', '#c7c7c7', '#686868', '#f07178', '#c2d94c', '#ffb454', '#59c2ff', '#ffee99', '#95e6cb', '#ffffff']),
  T('Rosé Pine', '#191724', '#e0def4', '#403d52', ['#26233a', '#eb6f92', '#31748f', '#f6c177', '#9ccfd8', '#c4a7e7', '#ebbcba', '#e0def4', '#6e6a86', '#eb6f92', '#31748f', '#f6c177', '#9ccfd8', '#c4a7e7', '#ebbcba', '#e0def4']),
  T('Verde clássico', '#000000', '#33ff66', '#0a4d1f', ['#000000', '#cc3333', '#33ff66', '#cccc33', '#3366cc', '#cc33cc', '#33cccc', '#cccccc', '#555555', '#ff5555', '#66ff99', '#ffff55', '#5588ff', '#ff55ff', '#55ffff', '#ffffff']),
])

const hex2 = (n) => Math.max(0, Math.min(255, Math.round(n))).toString(16).padStart(2, '0')
export const rgbHex = (r, g, b) => '#' + hex2(r) + hex2(g) + hex2(b)

/** Aceita "#rgb", "#rrggbb" ou "rgb(r,g,b)"; devolve "#rrggbb" ou null. */
export function normColor(v) {
  if (typeof v !== 'string') return null
  const s = v.trim().toLowerCase()
  let m = /^#([0-9a-f]{3})$/.exec(s)
  if (m) return '#' + m[1].split('').map((c) => c + c).join('')
  m = /^#([0-9a-f]{6})(?:[0-9a-f]{2})?$/.exec(s)
  if (m) return '#' + m[1]
  m = /^rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)/.exec(s)
  if (m) return rgbHex(+m[1], +m[2], +m[3])
  return null
}

/** Limpa um objeto de tema: só as chaves conhecidas, só cores válidas; o que faltar vem do tema Padrão. */
export function cleanTheme(t) {
  const out = { ...BUILTIN_THEMES['Padrão'] }
  for (const k of THEME_KEYS) {
    const c = normColor(t && t[k])
    if (c) out[k] = c
  }
  return out
}

/** Lê um .itermcolors (plist XML). */
export function themeFromITerm(xml) {
  const doc = new DOMParser().parseFromString(xml, 'application/xml')
  if (doc.querySelector('parsererror')) throw new Error('arquivo de tema inválido')
  const top = doc.querySelector('plist > dict')
  if (!top) throw new Error('não parece um tema do iTerm2')
  const colors = {}
  const kids = [...top.children]
  for (let i = 0; i + 1 < kids.length; i += 2) {
    if (kids[i].tagName !== 'key' || kids[i + 1].tagName !== 'dict') continue
    const d = {}
    const dk = [...kids[i + 1].children]
    for (let j = 0; j + 1 < dk.length; j += 2) d[dk[j].textContent] = parseFloat(dk[j + 1].textContent)
    const c = (n) => Math.round(Math.min(1, Math.max(0, d[n] ?? 0)) * 255)
    colors[kids[i].textContent] = rgbHex(c('Red Component'), c('Green Component'), c('Blue Component'))
  }
  const t = {
    background: colors['Background Color'], foreground: colors['Foreground Color'],
    cursor: colors['Cursor Color'], selectionBackground: colors['Selection Color'],
  }
  ANSI_KEYS.forEach((k, i) => { t[k] = colors[`Ansi ${i} Color`] })
  if (!t.background || !t.foreground) throw new Error('o tema não tem cores de fundo e de texto')
  return cleanTheme(t)
}

/** Lê o JSON de um esquema do Windows Terminal ou do próprio NxATerm. */
export function themeFromJSON(text) {
  let j = JSON.parse(text)
  if (Array.isArray(j.schemes)) j = j.schemes[0]
  if (!j || typeof j !== 'object') throw new Error('tema inválido')
  const wt = { purple: 'magenta', brightPurple: 'brightMagenta', cursorColor: 'cursor', selectionBackground: 'selectionBackground' }
  const t = {}
  for (const [k, v] of Object.entries(j)) t[wt[k] || k] = v
  if (!normColor(t.background) || !normColor(t.foreground)) throw new Error('o tema não tem cores de fundo e de texto')
  return { name: typeof j.name === 'string' ? j.name : '', theme: cleanTheme(t) }
}

/** Esquema no formato do Windows Terminal (o NxATerm também lê este formato). */
export function themeToJSON(name, theme) {
  const t = cleanTheme(theme)
  const o = { name, background: t.background, foreground: t.foreground, cursorColor: t.cursor, selectionBackground: t.selectionBackground }
  for (const k of ANSI_KEYS) o[k === 'magenta' ? 'purple' : k === 'brightMagenta' ? 'brightPurple' : k] = t[k]
  return JSON.stringify(o, null, 2)
}

/** "#rrggbb" + opacidade (0 a 1) -> "rgba(...)" para o fundo do terminal. */
export function withAlpha(hex, a) {
  const c = normColor(hex) || '#000000'
  if (a >= 1) return c
  return `rgba(${parseInt(c.slice(1, 3), 16)}, ${parseInt(c.slice(3, 5), 16)}, ${parseInt(c.slice(5, 7), 16)}, ${a})`
}
