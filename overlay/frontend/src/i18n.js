// Idiomas da interface: português (original), inglês e espanhol.
// Os textos do app estão em português; o tradutor troca texto, placeholder, title e aria-label no DOM
// (inclusive o que for criado depois) usando o dicionário. Textos que não estão nele ficam como estão.
import { DICT_A } from './i18n-dict-a.js'
import { DICT_B } from './i18n-dict-b.js'
import { DICT_C } from './i18n-dict-c.js'
import { DICT_D } from './i18n-dict-d.js'

export const LANGS = { pt: 'Português', en: 'English', es: 'Español' }
const COL = { en: 1, es: 2 }

const exact = { en: new Map(), es: new Map() }
const templates = { en: [], es: [] } // { re, out, n }

for (const line of (DICT_A + '\n' + DICT_B + '\n' + DICT_C + '\n' + DICT_D).split('\n')) {
  const parts = line.split('¦').map((x) => x.trim())
  if (parts.length < 3 || !parts[0]) continue
  for (const lang of ['en', 'es']) {
    const to = parts[COL[lang]]
    if (!to) continue
    if (/\{\d+\}/.test(parts[0])) {
      const re = new RegExp('^' + parts[0].replace(/[.*+?^$()|[\]\\]/g, '\\$&').replace(/\{(\d+)\}/g, '([\\s\\S]*?)') + '$')
      templates[lang].push({ re, out: to, len: parts[0].length })
    } else {
      exact[lang].set(parts[0], to)
    }
  }
}
for (const lang of ['en', 'es']) templates[lang].sort((a, b) => b.len - a.len)

/** Resolve 'auto' pelo idioma do sistema (o padrão é o português). */
export function resolveLang(pref) {
  if (pref === 'pt' || pref === 'en' || pref === 'es') return pref
  const n = (navigator.language || 'pt').toLowerCase()
  if (n.startsWith('en')) return 'en'
  if (n.startsWith('es')) return 'es'
  return 'pt'
}

let lang = 'pt'
export const currentLang = () => lang

function trCore(s, l) {
  const hit = exact[l].get(s)
  if (hit !== undefined) return hit
  for (const t of templates[l]) {
    const m = t.re.exec(s)
    if (m) return t.out.replace(/\{(\d+)\}/g, (_, i) => trPart(m[Number(i) + 1] ?? '', l))
  }
  return s
}

// preserva os espaços das pontas
function trPart(s, l) {
  const core = s.trim()
  if (!core) return s
  const out = trCore(core, l)
  if (out === core) return s
  const i = s.indexOf(core)
  return s.slice(0, i) + out + s.slice(i + core.length)
}

/** Traduz um texto do app (para mensagens montadas em tempo de execução). */
export function t(s) { return lang === 'pt' ? s : trPart(String(s), lang) }

// ------------------------------------------------------------------ DOM
const ATTRS = ['title', 'placeholder', 'aria-label']
const SKIP = '.xterm, .no-i18n, textarea, script, style, pre, code, [contenteditable="true"]'
const SKIP_ATTR = '.xterm, .no-i18n, script, style' // o placeholder de um textarea pode ser traduzido; o conteúdo, não
const textSrc = new WeakMap() // nó de texto -> { src, out }
const attrSrc = new WeakMap() // elemento -> Map(atributo -> { src, out })
let observer = null

function skipped(el, sel = SKIP) { return !el || !!(el.closest && el.closest(sel)) }

function doText(node) {
  if (skipped(node.parentElement)) return
  const rec = textSrc.get(node)
  const mine = rec && node.data === rec.out
  if (lang === 'pt') {
    if (mine) { node.data = rec.src; textSrc.delete(node) }
    return
  }
  const src = mine ? rec.src : node.data
  const out = trPart(src, lang)
  if (out !== src) {
    textSrc.set(node, { src, out })
    if (node.data !== out) node.data = out
  } else {
    textSrc.delete(node)
  }
}

function doAttr(el, name) {
  if (skipped(el, SKIP_ATTR)) return
  let m = attrSrc.get(el)
  const rec = m && m.get(name)
  const cur = el.getAttribute(name)
  if (cur === null) { if (m) m.delete(name); return }
  const mine = rec && cur === rec.out
  if (lang === 'pt') {
    if (mine) { el.setAttribute(name, rec.src); m.delete(name) }
    return
  }
  const src = mine ? rec.src : cur
  const out = trPart(src, lang)
  if (out !== src) {
    if (!m) { m = new Map(); attrSrc.set(el, m) }
    m.set(name, { src, out })
    if (cur !== out) el.setAttribute(name, out)
  } else if (m) {
    m.delete(name)
  }
}

function walk(root) {
  if (root.nodeType === Node.TEXT_NODE) { doText(root); return }
  if (root.nodeType !== Node.ELEMENT_NODE) return
  if (skipped(root)) return
  for (const a of ATTRS) if (root.hasAttribute(a)) doAttr(root, a)
  const tw = document.createTreeWalker(root, NodeFilter.SHOW_TEXT | NodeFilter.SHOW_ELEMENT)
  for (let n = tw.nextNode(); n; n = tw.nextNode()) {
    if (n.nodeType === Node.TEXT_NODE) doText(n)
    else for (const a of ATTRS) if (n.hasAttribute(a)) doAttr(n, a)
  }
}

function onMutations(list) {
  for (const m of list) {
    if (m.type === 'childList') m.addedNodes.forEach(walk)
    else if (m.type === 'characterData') doText(m.target)
    else if (m.type === 'attributes') doAttr(m.target, m.attributeName)
  }
}

/** Aplica o idioma (preferência: 'auto' | 'pt' | 'en' | 'es') à tela inteira e às próximas mudanças. */
export function setLang(pref) {
  const next = resolveLang(pref)
  const was = lang
  lang = next
  document.documentElement.lang = { pt: 'pt-BR', en: 'en', es: 'es' }[next]
  if (!observer) observer = new MutationObserver(onMutations)
  observer.disconnect()
  if (was !== next || next !== 'pt') walk(document.body)
  if (next !== 'pt') {
    observer.observe(document.body, { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: ATTRS })
  }
}
