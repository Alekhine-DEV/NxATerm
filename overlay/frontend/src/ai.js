// Assistente de IA (qualquer serviço compatível com a API da OpenAI) e ferramentas do servidor MCP local.
import * as Runtime from '@wailsio/runtime'
import { Api } from './api.js'
import { h, icon, toast, cleanErr, openModal, closeModal, modalOpen, payload, combo } from './util.js'
import { settings, saveSettings } from './settings.js'
import { typeOf } from './sessiontypes.js'
import { resolveLang } from './i18n.js'

const Events = Runtime.Events

export const PRESETS = {
  openai: ['OpenAI', 'https://api.openai.com/v1'],
  openrouter: ['OpenRouter', 'https://openrouter.ai/api/v1'],
  ollama: ['Ollama (local)', 'http://localhost:11434/v1'],
  lmstudio: ['LM Studio (local)', 'http://localhost:1234/v1'],
  custom: ['Outro', ''],
}

let ctx = null // { activeTab, allTabs, openSession, sessions }
let apiKey = ''
let keyLoaded = false
let conv = [] // mensagens da conversa (formato do backend)
let busy = false
let stopToken = 0
let built = false
const el = {}

async function loadKey() {
  if (keyLoaded) return
  keyLoaded = true
  if (!settings.ai.remember) return
  try { apiKey = (await Api.AiLoadKey()) || '' } catch { /* sem chave guardada */ }
}

const cfg = () => ({ baseUrl: settings.ai.baseUrl, apiKey, model: settings.ai.model })

// ------------------------------------------------------------------ contexto
function screenText(tab, n = 60) {
  const buf = tab?.term?.buffer?.active
  if (!buf) return ''
  const lines = []
  for (let i = Math.max(0, buf.length - n); i < buf.length; i++) lines.push(buf.getLine(i)?.translateToString(true) ?? '')
  while (lines.length && !lines[lines.length - 1].trim()) lines.pop()
  const text = lines.join('\n')
  return text.length > 6000 ? text.slice(-6000) : text
}

function describeSession(s) {
  if (!s) return 'nenhuma'
  const ty = typeOf(s)
  const where = s.host ? `${s.user ? s.user + '@' : ''}${s.host}` : ''
  return `${s.name || ''} (${ty}${where ? ', ' + where : ''})`
}

const LANG_NAME = { pt: 'português do Brasil', en: 'English', es: 'español' }

function systemPrompt() {
  const tab = ctx.activeTab()
  const lang = resolveLang(settings.lang)
  let s = 'Você é o assistente do NxATerm, um gerenciador de terminais SSH, SFTP e shell. ' +
    'Ajude com comandos de shell, administração de servidores e diagnóstico, de forma objetiva. ' +
    `Responda em ${LANG_NAME[lang]}. Coloque comandos em blocos de código. ` +
    `Sessão ativa: ${describeSession(tab?.session)}.`
  if (settings.ai.agent) {
    s += ' Você pode usar a ferramenta run_command para executar comandos na sessão ativa; o usuário aprova cada um. ' +
      'Peça um comando por vez, prefira comandos de leitura e explique antes de qualquer comando que altere o sistema.'
  }
  if (settings.ai.withScreen && tab) {
    const scr = screenText(tab)
    if (scr) s += '\n\nÚltimas linhas do terminal do usuário:\n```\n' + scr + '\n```'
  }
  if (settings.ai.extra.trim()) s += '\n\n' + settings.ai.extra.trim()
  return s
}

// ------------------------------------------------------------------ painel
const BLOCK = /```([\w+-]*)\n?([\s\S]*?)```/g

function renderRich(text) {
  const box = h('div', { class: 'ai-text' })
  let last = 0
  for (const m of text.matchAll(BLOCK)) {
    if (m.index > last) box.append(textBlock(text.slice(last, m.index)))
    box.append(codeBlock(m[2].replace(/\n$/, '')))
    last = m.index + m[0].length
  }
  if (last < text.length) box.append(textBlock(text.slice(last)))
  return box
}

function textBlock(t) {
  t = t.replace(/^\n+|\n+$/g, '')
  if (!t) return document.createDocumentFragment()
  const p = h('div', { class: 'ai-p' })
  // `código` em linha
  t.split(/(`[^`\n]+`)/).forEach((part) => {
    if (/^`[^`\n]+`$/.test(part)) p.append(h('code', {}, part.slice(1, -1)))
    else if (part) p.append(document.createTextNode(part))
  })
  return p
}

function codeBlock(code) {
  const tab = ctx.activeTab()
  const canType = !!(tab && tab.term && !tab.dead)
  return h('div', { class: 'ai-code' },
    h('pre', {}, code),
    h('div', { class: 'ai-code-bar' },
      h('button', { class: 'mini-t', onclick: () => { navigator.clipboard.writeText(code).catch(() => {}); toast('Copiado') } }, 'Copiar'),
      canType ? h('button', { class: 'mini-t', onclick: () => {
        const t = ctx.activeTab()
        if (!t || t.dead) { toast('Nenhuma sessão ativa.', true); return }
        Api.Write(t.id, code.replace(/\r?\n$/, '')).catch(() => {})
        t.term.focus()
      } }, 'Inserir no terminal') : null))
}

function addMsg(role, content) {
  const box = h('div', { class: 'ai-msg ' + role }, role === 'user' ? h('div', { class: 'ai-text' }, content) : renderRich(content))
  el.log.append(box)
  scrollDown()
  return box
}

function scrollDown() { el.log.scrollTop = el.log.scrollHeight }

function setBusy(on) {
  busy = on
  el.send.classList.toggle('hidden', on)
  el.stop.classList.toggle('hidden', !on)
  el.think.classList.toggle('hidden', !on)
  el.input.disabled = on
  if (!on) el.input.focus()
}

function buildPanel() {
  const root = document.getElementById('panel-ai')
  el.status = h('div', { class: 'ai-status' })
  el.log = h('div', { class: 'ai-log' })
  el.think = h('div', { class: 'ai-think hidden' }, 'Pensando...')
  el.input = h('textarea', { rows: 3, spellcheck: 'false', placeholder: 'Pergunte algo (Enter envia, Shift+Enter quebra a linha)' })
  el.send = h('button', { class: 'primary', onclick: () => submit() }, 'Perguntar')
  el.stop = h('button', { class: 'hidden', onclick: () => { stopToken++; setBusy(false); addNote('Interrompido') } }, 'Parar')
  el.agent = h('input', { type: 'checkbox' })
  el.screen = h('input', { type: 'checkbox' })
  el.agent.addEventListener('change', () => { settings.ai.agent = el.agent.checked; saveSettings() })
  el.screen.addEventListener('change', () => { settings.ai.withScreen = el.screen.checked; saveSettings() })
  el.input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit() }
  })
  root.replaceChildren(
    h('div', { class: 'ai-head' },
      el.status,
      h('button', { class: 'ib', title: 'Nova conversa', onclick: newChat }, icon('refresh', 15)),
      h('button', { class: 'ib', title: 'Configurar o assistente', onclick: () => aiDialog() }, icon('gear', 15))),
    el.log,
    el.think,
    h('div', { class: 'ai-compose' },
      el.input,
      h('div', { class: 'ai-opts' },
        h('label', { class: 'check' }, el.agent, 'Modo agente'),
        h('label', { class: 'check' }, el.screen, 'Incluir a tela do terminal')),
      h('div', { class: 'btns' }, el.stop, el.send)))
  built = true
}

function newChat() {
  stopToken++
  conv = []
  busy = false
  el.log.replaceChildren()
  setBusy(false)
}

function addNote(text, err = false) {
  el.log.append(h('div', { class: 'ai-note' + (err ? ' err' : '') }, text))
  scrollDown()
}

/** Chamado ao abrir a aba "Assistente" da barra lateral. */
export async function renderAiPanel() {
  if (!built) buildPanel()
  await loadKey()
  el.agent.checked = settings.ai.agent
  el.screen.checked = settings.ai.withScreen
  const ready = !!settings.ai.model && !!settings.ai.baseUrl
  el.status.textContent = ready ? settings.ai.model : 'Não configurado'
  el.status.classList.toggle('warn', !ready)
  if (!ready && !el.log.children.length) {
    el.log.append(h('div', { class: 'ai-empty' },
      h('div', {}, 'Escolha o serviço, o modelo e a chave da API.'),
      h('button', { class: 'primary', onclick: () => aiDialog() }, 'Configurar')))
  }
}

async function submit() {
  if (busy) return
  const text = el.input.value.trim()
  if (!text) return
  if (!settings.ai.model || !settings.ai.baseUrl) { aiDialog(); return }
  el.input.value = ''
  el.log.querySelector('.ai-empty')?.remove()
  conv.push({ role: 'user', content: text })
  addMsg('user', text)
  const my = ++stopToken
  setBusy(true)
  try {
    await runTurns(my)
  } catch (e) {
    if (my === stopToken) addNote(cleanErr(e), true)
  }
  if (my === stopToken) setBusy(false)
}

const MAX_STEPS = 12

async function runTurns(my) {
  for (let step = 0; step < MAX_STEPS; step++) {
    const msgs = [{ role: 'system', content: systemPrompt() }, ...conv]
    const reply = await Api.AiChat(cfg(), msgs, settings.ai.agent)
    if (my !== stopToken) return
    const calls = (reply.toolCalls || []).filter((c) => c.name === 'run_command')
    const m = { role: 'assistant', content: reply.content || '' }
    if (reply.toolCalls && reply.toolCalls.length) m.toolCalls = reply.toolCalls
    conv.push(m)
    if (reply.content) addMsg('assistant', reply.content)
    if (!reply.toolCalls || !reply.toolCalls.length) return
    for (const call of reply.toolCalls) {
      let result
      if (call.name !== 'run_command') result = 'ferramenta desconhecida'
      else result = await askAndRun(call, my)
      if (my !== stopToken) return
      conv.push({ role: 'tool', toolCallId: call.id, content: result })
    }
    if (calls.length === 0) return
  }
  addNote('Parei depois de várias etapas seguidas. Envie outra mensagem para continuar.')
}

// ------------------------------------------------------------------ agente: aprovação e execução
const clip = (s, n = 12000) => (s.length <= n ? s : s.slice(0, n / 2) + '\n[... cortado ...]\n' + s.slice(-n / 2))

function formatResult(r) {
  const parts = [`código de saída: ${r.exitCode}`]
  if (r.error) parts.push('erro: ' + r.error)
  if (r.stdout) parts.push('stdout:\n' + clip(r.stdout))
  if (r.stderr) parts.push('stderr:\n' + clip(r.stderr))
  if (!r.stdout && !r.stderr && !r.error) parts.push('(sem saída)')
  return parts.join('\n')
}

async function execOn(tab, command) {
  const s = tab.session
  const ty = typeOf(s)
  if (ty === 'ssh' || ty === 'sftp') return Api.ExecCommand(s, tab.secret || '', command, 60)
  if (ty === 'shell') return Api.ExecLocal(command, 60)
  throw new Error('este tipo de sessão não executa comandos do agente')
}

function askAndRun(call, my) {
  let args = {}
  try { args = JSON.parse(call.arguments || '{}') } catch { /* argumentos inválidos */ }
  const command = String(args.command || '').trim()
  const reason = String(args.reason || '').trim()
  const tab = ctx.activeTab()
  return new Promise((resolve) => {
    if (!command) { resolve('comando vazio'); return }
    if (!tab || tab.dead || !tab.term) { resolve('não há uma sessão ativa para executar o comando'); return }
    const out = h('pre', { class: 'ai-out hidden' })
    const buttons = h('div', { class: 'ai-actions' })
    const card = h('div', { class: 'ai-cmd' },
      h('div', { class: 'ai-cmd-head' }, icon('shell', 14), h('span', {}, tab.label || describeSession(tab.session))),
      reason ? h('div', { class: 'ai-reason' }, reason) : null,
      h('pre', { class: 'ai-cmd-code' }, command),
      buttons, out)
    const finish = (text) => { buttons.remove(); resolve(text) }
    buttons.append(
      h('button', { onclick: () => { buttons.replaceChildren(h('span', { class: 'ai-running' }, 'Recusado')); resolve('o usuário recusou executar este comando') } }, 'Recusar'),
      h('button', { class: 'primary', onclick: async () => {
        buttons.replaceChildren(h('span', { class: 'ai-running' }, 'Executando...'))
        try {
          const r = await execOn(tab, command)
          if (my !== stopToken) { resolve('interrompido'); return }
          out.textContent = clip(`${r.stdout || ''}${r.stderr ? '\n' + r.stderr : ''}${r.error ? '\n' + r.error : ''}`.trim() || '(sem saída)', 4000)
          out.classList.remove('hidden')
          buttons.replaceChildren(h('span', { class: r.exitCode === 0 && !r.error ? 'ai-ok' : 'ai-bad' }, r.error ? 'Falhou' : `Saída ${r.exitCode}`))
          resolve(formatResult(r))
        } catch (e) {
          buttons.replaceChildren(h('span', { class: 'ai-bad' }, 'Falhou'))
          resolve('erro: ' + cleanErr(e))
        }
        scrollDown()
      } }, 'Executar'))
    el.log.append(card)
    scrollDown()
  })
}

// ------------------------------------------------------------------ configuração
export async function aiDialog() {
  await loadKey()
  const ai = settings.ai
  const mc = settings.mcp
  const st = await Api.McpStatus().catch(() => ({ running: false, url: '' }))
  const token = await Api.McpToken().catch(() => '')

  const preset = h('select', {}, ...Object.entries(PRESETS).map(([k, v]) => h('option', { value: k, selected: k === ai.preset }, v[0])))
  const base = h('input', { type: 'text', value: ai.baseUrl, spellcheck: 'false' })
  const model = h('input', { type: 'text', value: ai.model, spellcheck: 'false', placeholder: 'ex.: gpt-4o-mini' })
  const key = h('input', { type: 'password', value: apiKey, spellcheck: 'false', autocomplete: 'off', placeholder: 'Opcional em servidores locais' })
  const remember = h('input', { type: 'checkbox', checked: ai.remember })
  const extra = h('textarea', { rows: 3, spellcheck: 'false', placeholder: 'Instruções adicionais (opcional)' }, ai.extra)
  preset.addEventListener('change', () => { base.value = PRESETS[preset.value][1]; ai.preset = preset.value })
  base.addEventListener('input', () => { preset.value = 'custom' })
  combo(model, async () => Api.AiModels({ baseUrl: base.value.trim(), apiKey: key.value.trim(), model: '' }))

  const mcpOn = h('input', { type: 'checkbox', checked: mc.enabled })
  const mcpPort = h('input', { type: 'number', min: 1024, max: 65535, value: mc.port })
  const url = h('input', { type: 'text', readonly: true, value: st.running ? st.url : `http://127.0.0.1:${mc.port}/mcp` })
  const tok = h('input', { type: 'password', readonly: true, value: token })
  const copy = (v) => { navigator.clipboard.writeText(v).catch(() => {}); toast('Copiado') }

  const save = async () => {
    ai.preset = preset.value
    ai.baseUrl = base.value.trim()
    ai.model = model.value.trim()
    ai.extra = extra.value
    ai.remember = remember.checked
    apiKey = key.value.trim()
    try {
      if (ai.remember) await Api.AiSaveKey(apiKey)
      else await Api.AiForgetKey()
    } catch (e) { toast(cleanErr(e), true) }
    const port = Math.min(65535, Math.max(1024, parseInt(mcpPort.value, 10) || 7399))
    const wasOn = mc.enabled
    mc.enabled = mcpOn.checked
    mc.port = port
    saveSettings()
    try {
      if (mc.enabled) await Api.McpStart(port)
      else if (wasOn || st.running) await Api.McpStop()
    } catch (e) { toast(cleanErr(e), true); mc.enabled = false; saveSettings() }
    closeModal()
    renderAiPanel()
  }

  openModal([
    h('h2', {}, 'Assistente de IA'),
    h('label', {}, 'Serviço'), preset,
    h('label', {}, 'Endereço da API'), base,
    h('label', {}, 'Modelo'), model,
    h('label', {}, 'Chave da API'), key,
    h('label', { class: 'check' }, remember, 'Guardar a chave neste computador'),
    h('label', {}, 'Instruções'), extra,
    h('h2', { class: 'sub' }, 'Servidor MCP local'),
    h('label', { class: 'check' }, mcpOn, 'Ativar (só neste computador)'),
    h('div', { class: 'row' },
      h('div', { class: 'narrow' }, h('label', {}, 'Porta'), mcpPort),
      h('div', {}, h('label', {}, 'Endereço'), url)),
    h('label', {}, 'Token'),
    h('div', { class: 'ai-tok' }, tok,
      h('button', { class: 'mini-t', onclick: () => copy(token) }, 'Copiar'),
      h('button', { class: 'mini-t', onclick: async () => {
        try { const t = await Api.McpResetToken(); tok.value = t; toast('Token novo gerado') } catch (e) { toast(cleanErr(e), true) }
      } }, 'Gerar outro')),
    h('div', { class: 'btns' },
      h('button', { onclick: closeModal }, 'Cancelar'),
      h('button', { class: 'primary', onclick: save }, 'Salvar')),
  ], 'ai-modal')
}

// ------------------------------------------------------------------ MCP: ferramentas executadas aqui
let mcpChain = Promise.resolve()

function approveBox({ title, lines, code, ok }) {
  return new Promise((resolve) => {
    const done = (v) => { closeModal(); resolve(v) }
    openModal([
      h('h2', {}, title),
      ...lines.map((l) => h('p', {}, l)),
      code ? h('pre', { class: 'ai-cmd-code' }, code) : null,
      h('div', { class: 'btns' },
        h('button', { onclick: () => done(false) }, 'Recusar'),
        h('button', { class: 'primary', onclick: () => done(true) }, ok)),
    ])
  })
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function waitFreeModal() {
  for (let i = 0; i < 240 && modalOpen(); i++) await sleep(500)
}

function findTab(id) {
  const all = ctx.allTabs()
  if (id) return all.find((t) => t.id === id)
  return ctx.activeTab()
}

async function mcpTool(tool, args) {
  switch (tool) {
    case 'list_tabs': {
      const act = ctx.activeTab()
      return JSON.stringify(ctx.allTabs().filter((t) => t.term).map((t) => ({
        id: t.id, name: t.label, type: typeOf(t.session), active: act?.id === t.id, closed: !!t.dead,
      })), null, 1)
    }
    case 'read_terminal': {
      const tab = findTab(args.tab)
      if (!tab || !tab.term) throw new Error('aba não encontrada')
      const n = Math.min(500, Math.max(1, parseInt(args.lines, 10) || 50))
      return screenText(tab, n) || '(vazio)'
    }
    case 'send_input': {
      const tab = findTab(args.tab)
      if (!tab || !tab.term || tab.dead) throw new Error('aba não encontrada ou encerrada')
      const text = String(args.text ?? '')
      if (!text) throw new Error('texto vazio')
      const enter = args.enter !== false
      await waitFreeModal()
      const ok = await approveBox({
        title: 'Cliente MCP quer digitar no terminal',
        lines: [`Aba: ${tab.label}${enter ? ' (com Enter no final)' : ''}`],
        code: text, ok: 'Permitir',
      })
      if (!ok) throw new Error('o usuário recusou')
      await Api.Write(tab.id, enter ? text.replace(/\r?\n$/, '') + '\r' : text)
      return 'enviado'
    }
    case 'open_session': {
      const name = String(args.name || '').trim().toLowerCase()
      const list = ctx.sessions()
      const exact = list.filter((s) => (s.name || '').toLowerCase() === name)
      const part = exact.length ? exact : list.filter((s) => (s.name || '').toLowerCase().includes(name))
      if (part.length !== 1) throw new Error(part.length ? 'mais de uma sessão com esse nome' : 'sessão não encontrada')
      await waitFreeModal()
      const ok = await approveBox({
        title: 'Cliente MCP quer abrir uma sessão',
        lines: [describeSession(part[0])], ok: 'Abrir',
      })
      if (!ok) throw new Error('o usuário recusou')
      ctx.openSession(part[0])
      return 'aberta'
    }
    default:
      throw new Error('ferramenta desconhecida')
  }
}

function onMcp(ev) {
  let req
  try { req = JSON.parse(payload(ev)) } catch { return }
  mcpChain = mcpChain.then(async () => {
    try {
      const text = await mcpTool(req.tool, req.args || {})
      await Api.McpRespond(req.id, text, false)
    } catch (e) {
      await Api.McpRespond(req.id, cleanErr(e), true)
    }
  })
}

/** Liga o painel ao app e, se estiver ativado, sobe o servidor MCP. */
export async function initAi(context) {
  ctx = context
  Events.On('mcp:request', onMcp)
  if (settings.mcp.enabled) {
    try { await Api.McpStart(settings.mcp.port) } catch (e) { toast('MCP: ' + cleanErr(e), true) }
  }
}
