// Tema da interface: troca as cores neutras (variáveis CSS do :root) sem mexer nas cores de destaque.
import { settings } from './settings.js'
import { normColor } from './termthemes.js'

// base: 'dark' mantém os cinzas como estão; 'light' inverte a luminosidade.
// hue (0-360) e sat (0-0.5) tingem os cinzas; accent troca a cor de destaque.
export const UI_BUILTIN = {
  Escuro: { base: 'dark', hue: 0, sat: 0, accent: '' },
  Claro: { base: 'light', hue: 0, sat: 0, accent: '' },
  'Meia-noite': { base: 'dark', hue: 225, sat: 0.3, accent: '#4f8cff' },
  Floresta: { base: 'dark', hue: 150, sat: 0.2, accent: '#2fa36b' },
  Vinho: { base: 'dark', hue: 340, sat: 0.2, accent: '#d6457a' },
  'Areia': { base: 'light', hue: 38, sat: 0.3, accent: '#c4721a' },
}

export function uiThemes() {
  return { ...UI_BUILTIN, ...(settings.uiThemes || {}) }
}

function hex2rgb(h) {
  return [parseInt(h.slice(1, 3), 16), parseInt(h.slice(3, 5), 16), parseInt(h.slice(5, 7), 16)]
}
function rgb2hex(r, g, b) {
  return '#' + [r, g, b].map((v) => Math.max(0, Math.min(255, Math.round(v))).toString(16).padStart(2, '0')).join('')
}
function rgb2hsl(r, g, b) {
  r /= 255; g /= 255; b /= 255
  const mx = Math.max(r, g, b), mn = Math.min(r, g, b), l = (mx + mn) / 2
  if (mx === mn) return [0, 0, l]
  const d = mx - mn
  const s = l > 0.5 ? d / (2 - mx - mn) : d / (mx + mn)
  let h
  if (mx === r) h = (g - b) / d + (g < b ? 6 : 0)
  else if (mx === g) h = (b - r) / d + 2
  else h = (r - g) / d + 4
  return [h * 60, s, l]
}
function hsl2rgb(h, s, l) {
  h = ((h % 360) + 360) % 360 / 360
  if (s === 0) { const v = l * 255; return [v, v, v] }
  const q = l < 0.5 ? l * (1 + s) : l + s - l * s, p = 2 * l - q
  const f = (t) => {
    if (t < 0) t += 1
    if (t > 1) t -= 1
    if (t < 1 / 6) return p + (q - p) * 6 * t
    if (t < 1 / 2) return q
    if (t < 2 / 3) return p + (q - p) * (2 / 3 - t) * 6
    return p
  }
  return [f(h + 1 / 3) * 255, f(h) * 255, f(h - 1 / 3) * 255]
}

/** Cores da :root que são cinzas (pouca saturação) e a cor original delas. */
let neutral = null
function neutrals() {
  if (neutral) return neutral
  neutral = {}
  for (const sheet of document.styleSheets) {
    let rules
    try { rules = sheet.cssRules } catch { continue }
    for (const r of rules) {
      if (r.selectorText !== ':root') continue
      for (let i = 0; i < r.style.length; i++) {
        const name = r.style[i]
        if (!name.startsWith('--')) continue
        const c = normColor(r.style.getPropertyValue(name))
        if (!c || !/^#[0-9a-f]{6}$/.test(c)) continue
        const [h, s, l] = rgb2hsl(...hex2rgb(c))
        if (s <= 0.32 || name.startsWith('--t-')) neutral[name] = { hex: c, h, s, l, hued: name.startsWith('--t-') }
      }
    }
  }
  return neutral
}

/** Calcula as variáveis de um tema. */
export function computeUiVars(theme, opacity = 1) {
  const vars = {}
  const light = theme.base === 'light'
  const tint = Math.max(0, Math.min(0.5, Number(theme.sat) || 0))
  for (const [name, o] of Object.entries(neutrals())) {
    let l = light ? 1 - o.l : o.l
    // no claro, evita o branco puro e o preto puro em tudo: deixa o contraste mais suave
    if (light) l = 0.04 + l * 0.92
    let h = o.h, s = o.s
    if (tint > 0 && !o.hued) { h = theme.hue; s = tint * (0.4 + 0.6 * (light ? Math.abs(0.5 - l) * 2 : 1 - l)) }
    const [r, g, b] = hsl2rgb(h, s, l)
    // fundos escuros ficam translúcidos quando a janela é transparente
    if (opacity < 1 && o.l < 0.26) vars[name] = `rgba(${Math.round(r)}, ${Math.round(g)}, ${Math.round(b)}, ${opacity})`
    else vars[name] = rgb2hex(r, g, b)
  }
  const ac = normColor(theme.accent)
  if (ac) {
    const [h, s, l] = rgb2hsl(...hex2rgb(ac))
    const dark = (k) => rgb2hex(...hsl2rgb(h, s, Math.max(0.12, l * k)))
    vars['--accent'] = ac
    vars['--accent-2'] = dark(0.78)
    vars['--sel'] = dark(light ? 0.9 : 0.62)
  }
  return vars
}

let applied = []

/** Aplica um tema da interface sem gravar (pré-visualização nos editores). */
export function previewUiTheme(theme) {
  const root = document.documentElement
  for (const n of applied) root.style.removeProperty(n)
  const vars = computeUiVars(theme, 1)
  for (const [k, v] of Object.entries(vars)) root.style.setProperty(k, v)
  applied = Object.keys(vars)
  root.style.colorScheme = theme.base === 'light' ? 'light' : 'dark'
}
let startupTransparent = false
export function setStartupTransparent(v) { startupTransparent = !!v }

function setStyleTag(id, css) {
  let el = document.getElementById(id)
  if (!css) { el?.remove(); return }
  if (!el) { el = document.createElement('style'); el.id = id; document.head.appendChild(el) }
  el.textContent = css
}

let bgData = ''
export function setBackgroundData(d) { bgData = d || ''; applyAppearance() }
export function hasBackground() { return !!bgData }

/** A janela foi criada transparente? (a escolha só vale ao reabrir o app) */
export function transparentWindow() { return startupTransparent }

/** O terminal deixa o fundo vazar (imagem, transparência da janela ou opacidade abaixo de 100%)? */
export function termSeeThrough() { return !!bgData || settings.termOpacity < 1 }

export function applyAppearance() {
  const root = document.documentElement
  const theme = uiThemes()[settings.uiTheme] || UI_BUILTIN.Escuro
  const op = startupTransparent ? settings.uiOpacity : 1
  const vars = computeUiVars(theme, op)
  for (const n of applied) root.style.removeProperty(n)
  for (const [k, v] of Object.entries(vars)) root.style.setProperty(k, v)
  applied = Object.keys(vars)
  root.style.colorScheme = theme.base === 'light' ? 'light' : 'dark'
  root.dataset.theme = theme.base === 'light' ? 'light' : 'dark'

  // fundo do palco: imagem e/ou transparência
  root.style.setProperty('--stage-bg', termSeeThrough() ? 'transparent' : 'var(--g-000000)')
  const stage = document.getElementById('stage')
  if (stage) {
    let layer = document.getElementById('bglayer')
    if (!layer) {
      layer = document.createElement('div')
      layer.id = 'bglayer'
      stage.prepend(layer)
    }
    layer.style.backgroundImage = bgData ? `url("${bgData}")` : 'none'
    layer.style.opacity = String(settings.bgOpacity)
    layer.style.filter = settings.bgBlur > 0 ? `blur(${settings.bgBlur}px)` : 'none'
    layer.style.display = bgData ? 'block' : 'none'
  }
  setStyleTag('custom-css', settings.customCss || '')
}

/** Cor de fundo do painel do terminal conforme o tema e a opacidade escolhidos. */
export function paneBackground(bg) {
  if (hasBackground()) return 'rgba(0, 0, 0, 0)'
  const a = settings.termOpacity
  if (a >= 1) return bg
  const [r, g, b] = hex2rgb(normColor(bg) || '#000000')
  return `rgba(${r}, ${g}, ${b}, ${a})`
}
