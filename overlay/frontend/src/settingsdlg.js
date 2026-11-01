// Janela de configurações (em abas) e os editores de tema.
import { Api } from './api.js'
import { h, toast, cleanErr, openModal, closeModal, confirmBox } from './util.js'
import { settings, saveSettings, FONTS, allTermThemes } from './settings.js'
import {
  BUILTIN_THEMES, ANSI_KEYS, THEME_KEYS, cleanTheme, themeFromITerm, themeFromJSON, themeToJSON,
} from './termthemes.js'
import { lockDialog } from './organize.js'
import { isWindows } from './sessiontypes.js'
import {
  UI_BUILTIN, uiThemes, applyAppearance, previewUiTheme, setBackgroundData, hasBackground,
} from './uitheme.js'

const KEYS = [
  'font', 'fontSize', 'theme', 'cursorStyle', 'cursorBlink', 'scrollback', 'copyOnSelect', 'rightClick',
  'autoSftp', 'monitoring', 'followFolder', 'termSync', 'proxy', 'editor', 'multiInstance', 'tray', 'closeToTray', 'globalHotkey', 'checkUpdates',
  'uiTheme', 'uiThemes', 'customThemes', 'customCss', 'bgOpacity', 'bgBlur', 'termOpacity',
  'windowTransparent', 'uiOpacity', 'unicode11', 'webgl', 'inlineImages', 'zmodem', 'highlight',
]

const TABS = [['term', 'Terminal'], ['look', 'Aparência'], ['files', 'Arquivos'], ['conn', 'Conexão'], ['sys', 'Sistema']]
const FIELD_NAMES = { background: 'Fundo', foreground: 'Texto', cursor: 'Cursor', selectionBackground: 'Seleção' }
const ANSI_NAMES = ['Preto', 'Vermelho', 'Verde', 'Amarelo', 'Azul', 'Magenta', 'Ciano', 'Branco']

const field = (label, el) => [h('label', {}, label), el]
const copy = (v) => JSON.parse(JSON.stringify(v))

export function settingsDialog(onApply) {
  const draft = {}
  for (const k of KEYS) draft[k] = copy(settings[k])
  show({ draft, tab: 'term', onApply })
}

function themesOf(draft) { return { ...BUILTIN_THEMES, ...draft.customThemes } }
function uiThemesOf(draft) { return { ...UI_BUILTIN, ...draft.uiThemes } }

function uniqueName(base, taken) {
  let n = base
  for (let i = 2; taken[n]; i++) n = `${base} (${i})`
  return n
}

/** Quadro com as cores de um tema do terminal. */
function themePreview(t) {
  const sw = (c) => h('span', { class: 'sw', style: `background:${c}` })
  return h('div', { class: 'tp', style: `background:${t.background};color:${t.foreground}` },
    h('div', { class: 'tp-line' }, 'usuario@servidor:~$ ls -la'),
    h('div', { class: 'tp-line' }, h('span', { style: `color:${t.blue}` }, 'docs/ '), h('span', { style: `color:${t.green}` }, 'run.sh '), h('span', { style: `color:${t.red}` }, 'erro.log '), h('span', { style: `color:${t.yellow}` }, 'notas.txt')),
    h('div', { class: 'tp-sw' }, ANSI_KEYS.map((k) => sw(t[k]))))
}

// ------------------------------------------------------------------ janela principal
function show(st) {
  const { draft } = st
  const live = (el, key, kind) => {
    const read = kind === 'check' ? () => el.checked : kind === 'num' ? () => Number(el.value) : () => el.value
    el.addEventListener(kind === 'check' || el.tagName === 'SELECT' ? 'change' : 'input', () => { draft[key] = read() })
    return el
  }
  const txt = (key, ph = '') => live(h('input', { type: 'text', value: draft[key], spellcheck: 'false', placeholder: ph }), key)
  const chk = (key) => live(h('input', { type: 'checkbox', checked: !!draft[key] }), key, 'check')
  const num = (key, min, max, step = 1) => live(h('input', { type: 'number', min, max, step, value: draft[key] }), key, 'num')
  const sel = (key, opts) => live(h('select', {}, ...opts.map(([v, l]) => h('option', { value: v, selected: v === draft[key] }, l))), key)
  const range = (key, min, max, step, fmt) => {
    const out = h('span', { class: 'rng-out' }, fmt(draft[key]))
    const el = h('input', { type: 'range', min, max, step, value: draft[key] })
    el.addEventListener('input', () => { draft[key] = Number(el.value); out.textContent = fmt(draft[key]); st.liveApply?.() })
    return h('div', { class: 'rng' }, el, out)
  }
  const check = (key, label) => h('label', { class: 'check' }, chk(key), ' ' + label)
  const pct = (v) => Math.round(v * 100) + '%'

  const body = h('div', { class: 'set-pane' })
  const panes = {}

  // ---- Terminal
  {
    const themeSel = h('select', {})
    const prev = h('div', {})
    const refreshThemes = () => {
      const all = themesOf(draft)
      if (!all[draft.theme]) draft.theme = 'Padrão'
      themeSel.replaceChildren(
        h('optgroup', { label: 'Do aplicativo' }, Object.keys(BUILTIN_THEMES).map((k) => h('option', { value: k, selected: k === draft.theme }, k))),
        Object.keys(draft.customThemes).length
          ? h('optgroup', { label: 'Seus temas' }, Object.keys(draft.customThemes).map((k) => h('option', { value: k, selected: k === draft.theme }, k)))
          : null)
      prev.replaceChildren(themePreview(all[draft.theme]))
    }
    themeSel.addEventListener('change', () => { draft.theme = themeSel.value; refreshThemes() })
    refreshThemes()
    const isCustom = () => !!draft.customThemes[draft.theme]
    const act = (label, fn, extra = {}) => h('button', { class: 'mini-t', onclick: fn, ...extra }, label)
    const ruleList = h('div', { class: 'hl-list' })
    const hlCheck = h('input', { type: 'checkbox', checked: draft.highlight.enabled })
    hlCheck.addEventListener('change', () => { draft.highlight.enabled = hlCheck.checked })
    const renderRules = () => {
      ruleList.replaceChildren(...draft.highlight.rules.map((r, i) => {
        const p = h('input', { type: 'text', value: r.pattern, spellcheck: 'false', placeholder: 'Texto' })
        p.addEventListener('input', () => { r.pattern = p.value })
        const rx = h('input', { type: 'checkbox', checked: r.regex, title: 'Expressão regular' })
        rx.addEventListener('change', () => { r.regex = rx.checked })
        const col = h('input', { type: 'color', value: r.color })
        col.addEventListener('input', () => { r.color = col.value })
        return h('div', { class: 'hl-row' }, p, h('label', { class: 'check rx' }, rx, ' Regex'), col,
          h('button', { class: 'ib', title: 'Remover', onclick: () => { draft.highlight.rules.splice(i, 1); renderRules() } }, '×'))
      }))
    }
    renderRules()
    panes.term = [
      h('div', { class: 'row' },
        h('div', {}, field('Fonte', sel('font', Object.keys(FONTS).map((k) => [k, k])))),
        h('div', { class: 'narrow' }, field('Tamanho', num('fontSize', 8, 32)))),
      h('label', {}, 'Tema de cores'), themeSel, prev,
      h('div', { class: 'set-actions' },
        act('Editar', () => themeEditor(st, draft.theme)),
        act('Novo', () => themeEditor(st, draft.theme, true)),
        act('Importar', async () => {
          try {
            const f = await Api.ImportText('Escolha o arquivo do tema')
            if (!f || !f.name) return
            let name = f.name.replace(/\.[^.]+$/, '')
            let theme
            if (/<plist/i.test(f.content)) theme = themeFromITerm(f.content)
            else { const j = themeFromJSON(f.content); theme = j.theme; if (j.name) name = j.name }
            name = uniqueName(name, themesOf(draft))
            draft.customThemes[name] = theme
            draft.theme = name
            show(st)
            toast(`Tema "${name}" importado`)
          } catch (e) { toast(cleanErr(e), true) }
        }),
        act('Exportar', async () => {
          try {
            const p = await Api.ExportText(`${draft.theme}.json`, themeToJSON(draft.theme, themesOf(draft)[draft.theme]))
            if (p) toast('Tema exportado')
          } catch (e) { toast(cleanErr(e), true) }
        }),
        act('Excluir', async () => {
          if (!isCustom()) return
          const name = draft.theme
          if (!await confirmBox('Excluir tema', `Excluir o tema "${name}"?`)) { show(st); return }
          delete draft.customThemes[name]
          draft.theme = 'Padrão'
          show(st)
        }, isCustom() ? {} : { disabled: true })),
      h('div', { class: 'row' },
        h('div', {}, field('Cursor', sel('cursorStyle', [['block', 'Bloco'], ['bar', 'Barra'], ['underline', 'Sublinhado']]))),
        h('div', {}, field('Linhas de histórico', num('scrollback', 500, 200000, 500)))),
      check('cursorBlink', 'Cursor piscando'),
      check('copyOnSelect', 'Copiar ao selecionar'),
      field('Botão direito no terminal', sel('rightClick', [['paste', 'Colar'], ['menu', 'Abrir menu']])),
      h('h3', {}, 'Exibição'),
      check('unicode11', 'Largura correta de emojis e caracteres largos'),
      check('inlineImages', 'Imagens no terminal (sixel e iTerm2)'),
      check('webgl', 'Renderização por GPU (novas abas)'),
      check('zmodem', 'Transferir arquivos pelo terminal (Zmodem: rz e sz)'),
      h('h3', {}, 'Realce de palavras'),
      h('label', { class: 'check' }, hlCheck, ' Ativar'),
      ruleList,
      h('button', { class: 'mini-t', onclick: () => { draft.highlight.rules.push({ pattern: '', color: '#ffd866', regex: false }); renderRules() } }, '+ Regra'),
    ]
  }

  // ---- Aparência
  {
    const uiSel = h('select', {})
    const refreshUi = () => {
      const all = uiThemesOf(draft)
      if (!all[draft.uiTheme]) draft.uiTheme = 'Escuro'
      uiSel.replaceChildren(
        h('optgroup', { label: 'Do aplicativo' }, Object.keys(UI_BUILTIN).map((k) => h('option', { value: k, selected: k === draft.uiTheme }, k))),
        Object.keys(draft.uiThemes).length
          ? h('optgroup', { label: 'Seus temas' }, Object.keys(draft.uiThemes).map((k) => h('option', { value: k, selected: k === draft.uiTheme }, k)))
          : null)
    }
    refreshUi()
    const preview = () => { previewUiTheme(uiThemesOf(draft)[draft.uiTheme]) }
    uiSel.addEventListener('change', () => { draft.uiTheme = uiSel.value; preview() })
    st.liveApply = () => {}
    const isCustom = () => !!draft.uiThemes[draft.uiTheme]
    const act = (label, fn, extra = {}) => h('button', { class: 'mini-t', onclick: fn, ...extra }, label)
    const bgNote = h('span', { class: 'bg-state' }, hasBackground() ? 'Imagem definida' : 'Sem imagem')
    const css = h('textarea', { rows: 6, spellcheck: 'false', placeholder: '/* CSS */', class: 'mono' }, draft.customCss)
    css.addEventListener('input', () => { draft.customCss = css.value })
    panes.look = [
      h('label', {}, 'Tema da interface'), uiSel,
      h('div', { class: 'set-actions' },
        act('Editar', () => uiEditor(st, draft.uiTheme)),
        act('Novo', () => uiEditor(st, draft.uiTheme, true)),
        act('Excluir', async () => {
          if (!isCustom()) return
          const name = draft.uiTheme
          if (!await confirmBox('Excluir tema', `Excluir o tema "${name}"?`)) { show(st); return }
          delete draft.uiThemes[name]
          draft.uiTheme = 'Escuro'
          show(st)
        }, isCustom() ? {} : { disabled: true })),
      h('h3', {}, 'Fundo do terminal'),
      field('Opacidade do terminal', range('termOpacity', 0.2, 1, 0.05, pct)),
      h('div', { class: 'set-actions bg-row' },
        act('Escolher imagem', async () => {
          try {
            const url = await Api.PickBackground()
            if (!url) return
            settings.hasBg = true
            setBackgroundData(url)
            bgNote.textContent = 'Imagem definida'
            saveSettings()
          } catch (e) { toast(cleanErr(e), true) }
        }),
        act('Remover imagem', async () => {
          try {
            await Api.ClearBackground()
            settings.hasBg = false
            setBackgroundData('')
            bgNote.textContent = 'Sem imagem'
            saveSettings()
          } catch (e) { toast(cleanErr(e), true) }
        }),
        bgNote),
      field('Visibilidade da imagem', range('bgOpacity', 0.05, 1, 0.05, pct)),
      field('Desfoque da imagem', range('bgBlur', 0, 30, 1, (v) => v + ' px')),
      h('h3', {}, 'Janela'),
      check('windowTransparent', 'Janela transparente (ao reabrir o app)'),
      field('Opacidade da interface', range('uiOpacity', 0.4, 1, 0.05, pct)),
      h('h3', {}, 'CSS personalizado'),
      css,
    ]
    st.liveApply = () => {
      // pré-visualiza imagem/opacidade enquanto arrasta
      const bak = { bgOpacity: settings.bgOpacity, bgBlur: settings.bgBlur, termOpacity: settings.termOpacity }
      Object.assign(settings, { bgOpacity: draft.bgOpacity, bgBlur: draft.bgBlur, termOpacity: draft.termOpacity })
      applyAppearance()
      Object.assign(settings, bak)
    }
  }

  // ---- Arquivos
  panes.files = [
    check('autoSftp', 'Abrir o navegador SFTP ao conectar'),
    check('followFolder', 'Navegador SFTP acompanha o terminal'),
    check('termSync', 'Terminal acompanha o navegador de arquivos'),
    check('monitoring', 'Uso do servidor na barra de status'),
    field('Editor de arquivos remotos', txt('editor', 'Programa padrão do sistema')),
  ]

  // ---- Conexão
  panes.conn = [field('Proxy geral', txt('proxy', 'Sem proxy (socks5://host:1080 ou http://host:3128)'))]

  // ---- Sistema
  panes.sys = [
    check('multiInstance', 'Permitir várias janelas do NxATerm'),
    check('tray', 'Ícone na bandeja do sistema (ao reabrir o app)'),
    check('closeToTray', 'Fechar a janela envia para a bandeja (ao reabrir o app)'),
    isWindows ? field('Atalho global para mostrar/esconder', txt('globalHotkey', 'Nenhum (ex.: Ctrl+Alt+T)')) : null,
    check('checkUpdates', 'Avisar de versão nova ao abrir'),
    h('div', { class: 'set-actions' },
      h('button', { onclick: () => lockDialog(() => show(st)) }, 'Senha de abertura...')),
    h('div', { class: 'set-actions' },
      h('button', {
        onclick: async () => {
          try { await Api.RegisterLinkHandler(); toast('Links ssh:// e telnet:// agora abrem no NxATerm') } catch (e) { toast(cleanErr(e), true) }
        },
      }, 'Abrir links ssh:// e telnet:// com o NxATerm')),
  ]

  const tabs = h('div', { class: 'set-tabs' }, TABS.map(([id, label]) =>
    h('button', {
      class: 'set-tab' + (st.tab === id ? ' active' : ''),
      onclick: () => { st.tab = id; show(st) },
    }, label)))
  body.append(...panes[st.tab].flat(Infinity).filter(Boolean))

  const cancel = () => { applyAppearance(); closeModal() }
  openModal([
    h('h2', {}, 'Configurações'),
    tabs, body,
    h('div', { class: 'btns' },
      h('button', { onclick: cancel }, 'Cancelar'),
      h('button', {
        class: 'primary',
        onclick: () => {
          draft.fontSize = Math.min(32, Math.max(8, Math.round(draft.fontSize) || 14))
          draft.scrollback = Math.min(200000, Math.max(500, Math.round(draft.scrollback) || 10000))
          draft.proxy = String(draft.proxy).trim()
          draft.editor = String(draft.editor).trim()
          draft.highlight.rules = draft.highlight.rules.filter((r) => r.pattern)
          const hotkeyChanged = isWindows && draft.globalHotkey.trim() !== settings.globalHotkey
          draft.globalHotkey = String(draft.globalHotkey).trim()
          for (const k of KEYS) settings[k] = draft[k]
          saveSettings()
          if (hotkeyChanged) Api.SetGlobalHotkey(settings.globalHotkey).catch((e) => toast(cleanErr(e), true))
          applyAppearance()
          closeModal()
          st.onApply()
        },
      }, 'Aplicar')),
  ], 'set-modal')
  if (st.tab === 'look') previewUiTheme(uiThemesOf(draft)[draft.uiTheme])
}

// ------------------------------------------------------------------ editor de tema do terminal
function themeEditor(st, name, asNew = false) {
  const { draft } = st
  const all = themesOf(draft)
  const src = all[name] || BUILTIN_THEMES['Padrão']
  const builtin = !!BUILTIN_THEMES[name]
  const editing = !asNew && !builtin ? name : null // nome do tema personalizado que está sendo alterado
  const t = { ...src }
  const nameIn = h('input', { type: 'text', value: editing || uniqueName(asNew ? 'Meu tema' : `${name} (meu)`, all), spellcheck: 'false' })
  const prevBox = h('div', {})
  const upd = () => prevBox.replaceChildren(themePreview(t))
  const pick = (key, label) => {
    const c = h('input', { type: 'color', value: t[key] })
    c.addEventListener('input', () => { t[key] = c.value; upd() })
    return h('label', { class: 'cp' }, c, h('span', {}, label))
  }
  upd()
  const back = () => show(st)
  openModal([
    h('h2', {}, 'Tema do terminal'),
    field('Nome', nameIn),
    prevBox,
    h('h3', {}, 'Cores principais'),
    h('div', { class: 'cp-grid' }, Object.entries(FIELD_NAMES).map(([k, l]) => pick(k, l))),
    h('h3', {}, 'Cores ANSI'),
    h('div', { class: 'cp-grid' }, ANSI_KEYS.map((k, i) => pick(k, (i >= 8 ? 'Claro ' : '') + ANSI_NAMES[i % 8].toLowerCase()))),
    h('div', { class: 'btns' },
      h('button', { onclick: back }, 'Cancelar'),
      h('button', {
        class: 'primary',
        onclick: () => {
          const nm = nameIn.value.trim()
          if (!nm) { toast('Dê um nome ao tema', true); return }
          if (BUILTIN_THEMES[nm]) { toast('Esse nome já é de um tema do aplicativo', true); return }
          if (draft.customThemes[nm] && nm !== editing) { toast('Já existe um tema com esse nome', true); return }
          if (editing && nm !== editing) delete draft.customThemes[editing]
          draft.customThemes[nm] = cleanTheme(t)
          draft.theme = nm
          back()
        },
      }, 'Salvar')),
  ], 'set-modal')
}

// ------------------------------------------------------------------ editor de tema da interface
function uiEditor(st, name, asNew = false) {
  const { draft } = st
  const all = uiThemesOf(draft)
  const src = all[name] || UI_BUILTIN.Escuro
  const editing = !asNew && draft.uiThemes[name] ? name : null
  const t = { ...src }
  const nameIn = h('input', { type: 'text', value: editing || uniqueName(asNew ? 'Meu tema' : `${name} (meu)`, all), spellcheck: 'false' })
  const base = h('select', {}, h('option', { value: 'dark', selected: t.base !== 'light' }, 'Escuro'), h('option', { value: 'light', selected: t.base === 'light' }, 'Claro'))
  const rng = (key, min, max, step, fmt) => {
    const out = h('span', { class: 'rng-out' }, fmt(t[key]))
    const el = h('input', { type: 'range', min, max, step, value: t[key] })
    el.addEventListener('input', () => { t[key] = Number(el.value); out.textContent = fmt(t[key]); previewUiTheme(t) })
    return h('div', { class: 'rng' }, el, out)
  }
  const useAccent = h('input', { type: 'checkbox', checked: !!t.accent })
  const accent = h('input', { type: 'color', value: t.accent || '#2d7ad6' })
  const sync = () => { t.accent = useAccent.checked ? accent.value : ''; previewUiTheme(t) }
  useAccent.addEventListener('change', sync)
  accent.addEventListener('input', () => { useAccent.checked = true; sync() })
  base.addEventListener('change', () => { t.base = base.value; previewUiTheme(t) })
  previewUiTheme(t)
  const back = () => { show(st) }
  openModal([
    h('h2', {}, 'Tema da interface'),
    field('Nome', nameIn),
    field('Base', base),
    field('Matiz', rng('hue', 0, 360, 1, (v) => v + '°')),
    field('Intensidade da cor', rng('sat', 0, 0.5, 0.01, (v) => Math.round(v * 200) + '%')),
    h('label', {}, 'Cor de destaque'),
    h('div', { class: 'row acc' }, h('label', { class: 'check' }, useAccent, ' Personalizar'), accent),
    h('div', { class: 'btns' },
      h('button', { onclick: back }, 'Cancelar'),
      h('button', {
        class: 'primary',
        onclick: () => {
          const nm = nameIn.value.trim()
          if (!nm) { toast('Dê um nome ao tema', true); return }
          if (UI_BUILTIN[nm]) { toast('Esse nome já é de um tema do aplicativo', true); return }
          if (draft.uiThemes[nm] && nm !== editing) { toast('Já existe um tema com esse nome', true); return }
          if (editing && nm !== editing) delete draft.uiThemes[editing]
          draft.uiThemes[nm] = { base: t.base === 'light' ? 'light' : 'dark', hue: t.hue, sat: t.sat, accent: t.accent }
          draft.uiTheme = nm
          back()
        },
      }, 'Salvar')),
  ], 'set-modal')
}
