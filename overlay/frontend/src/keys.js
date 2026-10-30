import { settings, saveSettings } from './settings.js'

// Ações que aceitam atalho. "def" é o padrão; vazio = sem atalho até a pessoa escolher um.
export const ACTIONS = [
  { id: 'newSession', label: 'Nova sessão', def: 'Ctrl+Shift+T' },
  { id: 'quick', label: 'Conexão rápida', def: '' },
  { id: 'localShell', label: 'Shell local', def: '' },
  { id: 'duplicate', label: 'Duplicar aba', def: 'Ctrl+Shift+D' },
  { id: 'closeTab', label: 'Fechar aba', def: 'Ctrl+Shift+W' },
  { id: 'prevTab', label: 'Aba anterior', def: 'Ctrl+PageUp' },
  { id: 'nextTab', label: 'Próxima aba', def: 'Ctrl+PageDown' },
  { id: 'search', label: 'Buscar no terminal', def: 'Ctrl+Shift+F' },
  { id: 'multiexec', label: 'MultiExec', def: 'Ctrl+Shift+M' },
  { id: 'copy', label: 'Copiar', def: 'Ctrl+Shift+C' },
  { id: 'paste', label: 'Colar', def: 'Ctrl+Shift+V' },
  { id: 'paste2', label: 'Colar (alternativo)', def: 'Ctrl+Shift+Insert' },
  { id: 'sidebar', label: 'Barra lateral', def: '' },
  { id: 'tunnels', label: 'Túneis SSH', def: '' },
  { id: 'settings', label: 'Configurações', def: '' },
]

/** Estado da captura no editor de atalhos: enquanto ativa, os atalhos globais ficam parados. */
export const capture = { active: false }

const MODS = ['Ctrl', 'Alt', 'Shift']

function keyName(code) {
  if (/^Key[A-Z]$/.test(code)) return code.slice(3)
  if (/^Digit\d$/.test(code)) return code.slice(5)
  if (/^Numpad\d$/.test(code)) return 'Num' + code.slice(6)
  const map = { Backquote: '`', Minus: '-', Equal: '=', BracketLeft: '[', BracketRight: ']', Backslash: '\\', Semicolon: ';', Quote: "'", Comma: ',', Period: '.', Slash: '/', Space: 'Space', ArrowUp: 'Up', ArrowDown: 'Down', ArrowLeft: 'Left', ArrowRight: 'Right' }
  return map[code] || code
}

const isModifierKey = (code) => /^(Control|Shift|Alt|Meta|OS)(Left|Right)?$/.test(code)

/** Texto do atalho a partir de um evento de teclado ('' se for só um modificador ou uma combinação inválida). */
export function comboOf(e) {
  if (isModifierKey(e.code) || e.metaKey) return ''
  if (e.getModifierState && e.getModifierState('AltGraph')) return '' // AltGr é digitação (ABNT2), nunca atalho
  const fkey = /^F\d{1,2}$/.test(e.code)
  if (!fkey && !e.ctrlKey && !e.altKey) return ''
  if (e.ctrlKey && e.altKey) return ''
  return [...MODS.filter((m) => e[m.toLowerCase() + 'Key']), keyName(e.code)].join('+')
}

const overrides = () => (settings.keys && typeof settings.keys === 'object' ? settings.keys : (settings.keys = {}))

/** Atalho atual de uma ação. */
export function keyOf(id) {
  const o = overrides()
  if (id in o) return o[id]
  return ACTIONS.find((a) => a.id === id)?.def || ''
}

/** Ação ligada a esse atalho, se houver. */
export function actionFor(e) {
  const c = comboOf(e)
  if (!c) return null
  return ACTIONS.find((a) => keyOf(a.id) === c)?.id || null
}

/** Define o atalho de uma ação. Se outra ação usava o mesmo, ela fica sem atalho; devolve o rótulo dela. */
export function setKey(id, combo) {
  let taken = null
  if (combo) {
    for (const a of ACTIONS) {
      if (a.id !== id && keyOf(a.id) === combo) { overrides()[a.id] = ''; taken = a.label }
    }
  }
  const def = ACTIONS.find((a) => a.id === id)?.def || ''
  if (combo === def) delete overrides()[id]
  else overrides()[id] = combo
  saveSettings()
  return taken
}

export function resetKeys() {
  settings.keys = {}
  saveSettings()
}

export const isDefault = (id) => !(id in overrides())
