// Perguntas feitas pelo backend durante a conexão (senha de servidor de salto, código de dois
// fatores, senha do proxy) e abertura de links ssh:// ou de sessões pedidas pela linha de comando.
import * as Runtime from '@wailsio/runtime'
import { Api } from './api.js'
import { payload, promptBox, toast, cleanErr } from './util.js'
import { memo } from './memo.js'

const Events = Runtime.Events

async function onAsk(ev) {
  let req
  try { req = JSON.parse(payload(ev)) } catch { return }
  if (req.key && memo.has(req.key)) {
    Api.AnswerPrompt(req.id, memo.get(req.key), true)
    return
  }
  const r = await promptBox({
    title: req.title, label: req.label, secret: req.secret, ok: 'Continuar',
    remember: req.key ? 'Memorizar até fechar o NxATerm' : '',
  })
  if (r === null || r === undefined) { Api.AnswerPrompt(req.id, '', false); return }
  const value = typeof r === 'string' ? r : r.value
  if (typeof r !== 'string' && r.remember && req.key) memo.set(req.key, value)
  Api.AnswerPrompt(req.id, value, true)
}

/** Liga as perguntas do backend e abre o que foi pedido ao iniciar. */
export async function initConnect(openSession) {
  Events.On('ask:request', onAsk)

  const openItems = async (items) => {
    for (const item of items) {
      try { openSession(await Api.ResolveLaunch(item)) } catch (e) { toast(cleanErr(e), true) }
    }
  }
  Events.On('launch:open', (ev) => {
    try { openItems(JSON.parse(payload(ev))) } catch { /* ignora */ }
  })
  try { await openItems(await Api.TakeLaunchItems()) } catch { /* sem argumentos */ }
}
