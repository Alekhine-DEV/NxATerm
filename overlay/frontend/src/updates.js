// Verificação de atualização (consulta a última versão publicada no GitHub).
import { Api } from './api.js'
import { h, toast, cleanErr, openModal, closeModal } from './util.js'
import { settings, saveSettings } from './settings.js'

/** silent: só mostra algo se houver versão nova (e que o usuário não tenha ignorado). */
export async function checkUpdates({ silent, openURL }) {
  let info
  try {
    info = await Api.CheckUpdate()
  } catch (e) {
    if (!silent) toast(cleanErr(e), true)
    return
  }
  if (!info.newer) {
    if (!silent) toast(`Você está na versão mais recente (${info.current})`)
    return
  }
  if (silent && settings.skipVersion === info.latest) return
  const notes = (info.notes || '').trim()
  openModal([
    h('h2', {}, 'Nova versão disponível'),
    h('p', {}, `NxATerm ${info.latest} (você tem a ${info.current})`),
    notes ? h('pre', { class: 'upd-notes' }, notes) : null,
    h('div', { class: 'btns' },
      h('button', { onclick: () => { settings.skipVersion = info.latest; saveSettings(); closeModal() } }, 'Ignorar esta versão'),
      h('span', { class: 'spacer' }),
      h('button', { onclick: closeModal }, 'Depois'),
      h('button', { class: 'primary', onclick: () => { openURL(info.url); closeModal() } }, 'Abrir a página')),
  ], 'wide')
}
