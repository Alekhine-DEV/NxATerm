// Execução em lote: roda um comando em vários servidores SSH e mostra o resultado de cada um.
import { Api } from './api.js'
import { h, icon, toast, cleanErr, openModal, closeModal, promptBox } from './util.js'
import { memo, memoKey } from './memo.js'
import { typeOf } from './sessiontypes.js'

const field = (label, el) => [h('label', {}, label), el]

export function batchDialog(sessions, preset = {}) {
  const list = sessions.filter((s) => ['ssh', 'sftp'].includes(typeOf(s)))
  if (!list.length) { toast('Não há sessões SSH salvas.', true); return }
  show({
    list,
    picked: new Set(preset.ids || []),
    command: preset.command || '',
    timeout: 60,
    filter: '',
    results: null,
  })
}

function matches(s, q) {
  if (!q) return true
  q = q.toLowerCase()
  if (q.startsWith('#')) return (s.tags || []).some((t) => t.toLowerCase().includes(q.slice(1)))
  return [s.name, s.host, s.user, s.folder, ...(s.tags || [])].some((x) => (x || '').toLowerCase().includes(q))
}

function show(st) {
  if (st.results) return showResults(st)
  const filter = h('input', { type: 'text', value: st.filter, placeholder: 'Filtrar (ou #tag)', spellcheck: 'false' })
  const box = h('div', { class: 'batch-list' })
  const count = h('span', { class: 'org-sub' })
  const render = () => {
    const vis = st.list.filter((s) => matches(s, st.filter))
    box.replaceChildren(...vis.map((s) => {
      const c = h('input', { type: 'checkbox', checked: st.picked.has(s.id) })
      c.addEventListener('change', () => { c.checked ? st.picked.add(s.id) : st.picked.delete(s.id); count.textContent = st.picked.size + ' selecionada(s)' })
      return h('label', { class: 'check batch-item' }, c,
        h('span', { class: 'org-name' }, s.name),
        s.folder ? h('span', { class: 'org-sub' }, s.folder) : null)
    }))
    count.textContent = st.picked.size + ' selecionada(s)'
  }
  filter.addEventListener('input', () => { st.filter = filter.value; render() })
  render()
  const setAll = (on) => {
    for (const s of st.list.filter((x) => matches(x, st.filter))) on ? st.picked.add(s.id) : st.picked.delete(s.id)
    render()
  }
  const cmd = h('textarea', { rows: 8, spellcheck: 'false', class: 'mono', placeholder: 'Um ou mais comandos' }, st.command)
  cmd.addEventListener('input', () => { st.command = cmd.value })
  const to = h('input', { type: 'number', min: 1, max: 3600, value: st.timeout })
  to.addEventListener('input', () => { st.timeout = parseInt(to.value, 10) || 60 })
  const run = () => start(st)
  openModal([
    h('h2', {}, 'Executar em lote'),
    h('div', { class: 'batch-cols' },
      h('div', { class: 'batch-left' },
        filter, box,
        h('div', { class: 'set-actions' },
          h('button', { class: 'mini-t', onclick: () => setAll(true) }, 'Marcar'),
          h('button', { class: 'mini-t', onclick: () => setAll(false) }, 'Desmarcar'),
          count)),
      h('div', { class: 'batch-right' },
        field('Comando', cmd),
        h('div', { class: 'row' }, h('div', { class: 'narrow' }, field('Limite (s)', to)))),
    ),
    h('div', { class: 'btns' },
      h('button', { onclick: closeModal }, 'Fechar'),
      h('button', { class: 'primary', onclick: run }, 'Executar')),
  ], 'batch-modal')
}

async function start(st) {
  const targets = st.list.filter((s) => st.picked.has(s.id))
  if (!targets.length) { toast('Marque pelo menos um servidor.', true); return }
  if (!st.command.trim()) { toast('Digite o comando.', true); return }
  // senhas que faltam: pergunta antes de começar
  const secrets = new Map()
  for (const s of targets) {
    if (s.authMethod === 'password') {
      const key = memoKey('pw', s)
      if (memo.has(key)) { secrets.set(s.id, memo.get(key)); continue }
      const r = await promptBox({ title: 'Senha', label: `Senha de ${s.user}@${s.host}`, secret: true, ok: 'Continuar', remember: 'Memorizar até fechar o NxATerm' })
      if (!r) { show(st); return }
      secrets.set(s.id, r.value)
      if (r.remember) memo.set(key, r.value)
    } else if (s.authMethod === 'key') {
      secrets.set(s.id, memo.get(memoKey('pp', s)) || '')
    }
  }
  st.results = targets.map((s) => ({ s, state: 'wait', res: null }))
  show(st)
  const queue = [...st.results]
  const worker = async () => {
    for (let r = queue.shift(); r; r = queue.shift()) {
      r.state = 'run'
      st.update?.()
      try {
        r.res = await Api.ExecCommand(r.s, secrets.get(r.s.id) || '', st.command, st.timeout)
      } catch (e) {
        r.res = { exitCode: -1, stdout: '', stderr: '', error: cleanErr(e), millis: 0 }
      }
      r.state = 'done'
      st.update?.()
    }
  }
  await Promise.all([worker(), worker(), worker(), worker(), worker()])
  st.finished = true
  st.update?.()
}

function statusOf(r) {
  if (r.state === 'wait') return { cls: 'wait', text: 'Na fila' }
  if (r.state === 'run') return { cls: 'run', text: 'Executando...' }
  const x = r.res
  if (x.error) {
    const text = x.error === 'PASSPHRASE_REQUIRED' ? 'Chave protegida: abra a sessão uma vez e memorize a senha da chave' : x.error
    return { cls: 'err', text }
  }
  return x.exitCode === 0 ? { cls: 'ok', text: `Concluído (${(x.millis / 1000).toFixed(1)} s)` } : { cls: 'err', text: `Código de saída ${x.exitCode} (${(x.millis / 1000).toFixed(1)} s)` }
}

function reportText(st) {
  return st.results.map((r) => {
    const x = r.res || {}
    const stt = statusOf(r)
    return `=== ${r.s.name} (${r.s.user}@${r.s.host}) - ${stt.text}\n${x.stdout || ''}${x.stderr ? '\n[stderr]\n' + x.stderr : ''}`
  }).join('\n\n')
}

function showResults(st) {
  const box = h('div', { class: 'batch-results' })
  const open = st.open || (st.open = new Set())
  const summary = h('span', { class: 'org-sub' })
  const render = () => {
    box.replaceChildren(...st.results.map((r) => {
      const stt = statusOf(r)
      const x = r.res
      const body = open.has(r.s.id) && x
        ? h('pre', { class: 'batch-out' }, [x.stdout, x.stderr ? '[stderr]\n' + x.stderr : ''].filter(Boolean).join('\n') || '(sem saída)')
        : null
      return h('div', { class: 'batch-res' },
        h('div', {
          class: 'batch-head',
          onclick: () => { open.has(r.s.id) ? open.delete(r.s.id) : open.add(r.s.id); render() },
        },
        h('span', { class: 'dot ' + stt.cls }),
        h('span', { class: 'org-name' }, r.s.name),
        h('span', { class: 'org-sub' }, stt.text)),
        body)
    }))
    const done = st.results.filter((r) => r.state === 'done')
    const ok = done.filter((r) => !r.res.error && r.res.exitCode === 0).length
    summary.textContent = `${done.length} de ${st.results.length} · ${ok} com sucesso`
  }
  st.update = render
  render()
  openModal([
    h('h2', {}, 'Resultado do lote'),
    h('div', { class: 'batch-cmd' }, st.command.split('\n')[0] + (st.command.includes('\n') ? ' ...' : '')),
    box,
    h('div', { class: 'btns' },
      summary,
      h('span', { class: 'spacer' }),
      h('button', { onclick: () => navigator.clipboard.writeText(reportText(st)).then(() => toast('Resultado copiado')).catch(() => {}) }, 'Copiar'),
      h('button', {
        onclick: async () => {
          try { const p = await Api.ExportText('resultado-lote.txt', reportText(st)); if (p) toast('Resultado salvo') } catch (e) { toast(cleanErr(e), true) }
        },
      }, 'Salvar...'),
      h('button', { onclick: () => { st.update = null; st.results = null; st.finished = false; show(st) } }, 'Nova execução'),
      h('button', { class: 'primary', onclick: () => { st.update = null; closeModal() } }, 'Fechar')),
  ], 'batch-modal')
}
