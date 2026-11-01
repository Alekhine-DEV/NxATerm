// Realce de palavras no terminal: pinta o fundo dos trechos que casam com as regras do usuário.
import { settings } from './settings.js'

const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

let cache = { key: '', list: [] }
/** Regras compiladas (as inválidas são ignoradas). */
export function compiledRules() {
  const hl = settings.highlight
  if (!hl || !hl.enabled) return []
  const key = JSON.stringify(hl.rules)
  if (cache.key === key) return cache.list
  const list = []
  for (const r of hl.rules) {
    try {
      list.push({ re: new RegExp(r.regex ? r.pattern : escapeRe(r.pattern), 'gi'), color: r.color })
    } catch { /* expressão inválida */ }
  }
  cache = { key, list }
  return list
}

/** Trechos [{ start, end, color }] (em índices do texto) que casam com alguma regra. */
export function findMatches(text, rules) {
  const out = []
  for (const r of rules) {
    r.re.lastIndex = 0
    let m
    let guard = 0
    while ((m = r.re.exec(text)) && guard++ < 50) {
      if (m[0] === '') { r.re.lastIndex++; continue }
      out.push({ start: m.index, end: m.index + m[0].length, color: r.color })
    }
  }
  return out
}

const lum = (hex) => {
  const v = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
  return 0.2126 * v[0] + 0.7152 * v[1] + 0.0722 * v[2]
}

/** Liga o realce a um terminal. Devolve { refresh, stop }. */
export function attachHighlight(term) {
  if (!term.onWriteParsed || !term.registerDecoration) return { refresh() {}, stop() {} }
  const lines = new Map() // linha absoluta -> { text, decos }
  let timer = 0
  let busy = false

  const clearLine = (abs) => {
    const e = lines.get(abs)
    if (e) { e.decos.forEach((d) => d.dispose()); lines.delete(abs) }
  }

  const scan = () => {
    timer = 0
    if (busy) return
    busy = true
    try {
      const rules = compiledRules()
      const buf = term.buffer.active
      if (!rules.length) {
        for (const abs of [...lines.keys()]) clearLine(abs)
        return
      }
      const cursorAbs = buf.baseY + buf.cursorY
      for (let y = 0; y < term.rows; y++) {
        const abs = buf.viewportY + y
        const line = buf.getLine(abs)
        if (!line) continue
        // monta o texto e a coluna de cada caractere (caracteres largos ocupam 2 colunas)
        let text = ''
        const colOf = []
        const cell = buf.getNullCell()
        for (let x = 0; x < line.length; x++) {
          const c = line.getCell(x, cell)
          if (!c || c.getWidth() === 0) continue
          const ch = c.getChars() || ' '
          for (let k = 0; k < ch.length; k++) colOf.push(x)
          text += ch
        }
        const prev = lines.get(abs)
        if (prev && prev.text === text) continue
        if (prev) clearLine(abs)
        const ms = findMatches(text, rules)
        const decos = []
        for (const m of ms) {
          const marker = term.registerMarker(abs - cursorAbs)
          if (!marker) continue
          const x0 = colOf[m.start]
          const x1 = (colOf[m.end - 1] ?? x0) + 1
          const d = term.registerDecoration({
            marker, x: x0, width: Math.max(1, x1 - x0), layer: 'top',
            backgroundColor: m.color, foregroundColor: lum(m.color) > 0.55 ? '#000000' : '#ffffff',
          })
          if (d) decos.push(d)
        }
        lines.set(abs, { text, decos })
      }
      // linhas que já saíram do buffer
      for (const abs of [...lines.keys()]) if (!buf.getLine(abs)) clearLine(abs)
    } catch { /* o realce nunca deve quebrar o terminal */ } finally { busy = false }
  }

  const schedule = () => { if (!timer) timer = setTimeout(scan, 150) }
  const subs = [term.onWriteParsed(schedule), term.onScroll(schedule), term.onResize(schedule)]
  schedule()
  return {
    /** As regras mudaram: refaz tudo. */
    refresh() { for (const abs of [...lines.keys()]) clearLine(abs); schedule() },
    stop() {
      clearTimeout(timer)
      subs.forEach((s) => s.dispose())
      for (const abs of [...lines.keys()]) clearLine(abs)
    },
  }
}
