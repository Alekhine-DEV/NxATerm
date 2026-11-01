// VNC dentro do app: o noVNC (cliente VNC em JavaScript, MPL-2.0) fala com uma ponte WebSocket local
// aberta pelo backend (só 127.0.0.1, uso único), que repassa os bytes ao servidor VNC.
import RFB from '@novnc/novnc/core/rfb.js'
import { Api } from './api.js'
import { h, toast, promptBox } from './util.js'
import { memo, memoKey } from './memo.js'

/** Painel de uma aba VNC. */
export function createVncView(label) {
  const host = h('div', { class: 'vnc-host', tabindex: '0' })
  const ctl = { rfb: null }
  const paste = async () => {
    try {
      const t = await navigator.clipboard.readText()
      if (t && ctl.rfb) { ctl.rfb.clipboardPasteFrom(t); toast('Texto enviado') }
    } catch { /* sem permissão de leitura */ }
  }
  const pane = h('div', { class: 'pane vnc hidden' },
    h('div', { class: 'pane-head' }, h('span', { class: 'pane-title' }, label)),
    host,
    h('div', { class: 'vnc-bar' },
      h('button', { class: 'mini-t', onclick: () => ctl.rfb?.sendCtrlAltDel() }, 'Ctrl+Alt+Del'),
      h('button', { class: 'mini-t', onclick: paste }, 'Colar')))
  return { pane, host, ctl }
}

/**
 * Conecta. Resolve quando a conexão abre; rejeita se não conseguir.
 * onClosed(motivo) é chamado quando a sessão cair depois de aberta.
 */
export async function connectVnc({ session, host, ctl, onClosed }) {
  const url = await Api.VncOpen(session)
  const rfb = new RFB(host, url)
  rfb.scaleViewport = true
  rfb.resizeSession = false
  rfb.clipViewport = false
  ctl.rfb = rfb
  rfb.addEventListener('clipboard', (e) => { navigator.clipboard.writeText(e.detail.text).catch(() => {}) })
  rfb.addEventListener('credentialsrequired', async (e) => {
    const types = (e.detail && e.detail.types) || ['password']
    const cred = {}
    if (types.includes('username')) {
      const u = await promptBox({ title: 'Usuário', label: 'Usuário', value: session.user || '', ok: 'Continuar' })
      if (u === null) { rfb.disconnect(); return }
      cred.username = u
    }
    if (types.includes('password')) {
      const key = memoKey('pw', { user: '', host: session.host, port: session.port })
      let pw = memo.get(key)
      if (pw === undefined) {
        const r = await promptBox({ title: 'Senha', label: `Senha de ${session.host}`, secret: true, ok: 'Continuar', remember: 'Memorizar até fechar o NxATerm' })
        if (r === null) { rfb.disconnect(); return }
        pw = r.value
        if (r.remember) memo.set(key, pw)
      }
      cred.password = pw
    }
    rfb.sendCredentials(cred)
  })
  return new Promise((resolve, reject) => {
    let opened = false
    rfb.addEventListener('connect', () => { opened = true; resolve(); rfb.focus() })
    rfb.addEventListener('securityfailure', (e) => {
      memo.delete(memoKey('pw', { user: '', host: session.host, port: session.port }))
      toast('Falha de autenticação' + (e.detail && e.detail.reason ? ': ' + e.detail.reason : ''), true)
    })
    rfb.addEventListener('disconnect', (e) => {
      ctl.rfb = null
      const reason = e.detail && e.detail.clean ? '' : 'conexão perdida'
      if (opened) onClosed(reason)
      else reject(new Error('não foi possível conectar ao servidor VNC'))
    })
  })
}

export function disposeVnc(ctl) {
  try { ctl.rfb?.disconnect() } catch { /* já fechado */ }
  ctl.rfb = null
}
