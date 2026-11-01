// Zmodem (rz/sz) no terminal, usando zmodem.js. O protocolo roda aqui; os bytes e os
// arquivos passam pelo backend (WriteBytes, ZPickSend, ZRead, ZPickDir, ZCreate, ZWrite, ZClose).
import Zmodem from 'zmodem.js'
import { Api } from './api.js'
import { h, toast, cleanErr, fmtSize } from './util.js'

const CHUNK = 8192 // tamanho de cada pacote enviado ao servidor
const READ = 256 * 1024 // quanto se lê do disco por vez

function bytesToB64(u8) {
  let s = ''
  for (let i = 0; i < u8.length; i += 0x8000) s += String.fromCharCode.apply(null, u8.subarray(i, i + 0x8000))
  return btoa(s)
}
const b64ToU8 = (b64) => {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

/**
 * Liga o Zmodem a um terminal. Devolve { consume(bytes), active, dispose() }.
 * `consume` recebe tudo que vem do servidor: o que não é Zmodem segue para o terminal.
 */
export function createZmodem({ id, term, pane }) {
  let session = null
  let sendQ = Promise.resolve()
  let bar = null
  const state = { active: false }

  const sender = (octets) => {
    const u8 = octets instanceof Uint8Array ? octets : Uint8Array.from(octets)
    const b64 = bytesToB64(u8)
    sendQ = sendQ.then(() => Api.WriteBytes(id, b64)).catch(() => {})
  }

  const showBar = (text) => {
    if (!bar) {
      const label = h('span', { class: 'zm-text' })
      bar = h('div', { class: 'zm-bar' }, label,
        h('button', { class: 'mini-t', onclick: () => cancel() }, 'Cancelar'))
      bar.label = label
      pane.append(bar)
    }
    bar.label.textContent = text
  }
  const hideBar = () => { bar?.remove(); bar = null }

  const finish = () => { state.active = false; session = null; hideBar() }
  const cancel = () => {
    try { session?.abort() } catch { /* já encerrada */ }
    finish()
    term.write('\r\n\x1b[33mZmodem cancelado\x1b[0m\r\n')
  }

  const sentry = new Zmodem.Sentry({
    to_terminal: (o) => term.write(o instanceof Uint8Array ? o : Uint8Array.from(o)),
    sender,
    on_retract: () => { if (!state.active) hideBar() },
    on_detect: (detection) => {
      if (state.active) { detection.deny(); return }
      handle(detection).catch((e) => { toast('Zmodem: ' + cleanErr(e), true); finish() })
    },
  })

  async function handle(detection) {
    if (detection.is_valid && !detection.is_valid()) return
    // "receive": o servidor vai enviar (sz) -> baixamos; "send": o servidor espera (rz) -> enviamos
    if (detection.get_session_role() === 'receive') await receive(detection)
    else await send(detection)
  }

  async function receive(detection) {
    const dir = await Api.ZPickDir()
    if (!dir) { detection.deny(); return }
    session = detection.confirm()
    state.active = true
    showBar('Zmodem: aguardando arquivos...')
    let count = 0
    session.on('offer', (offer) => {
      const d = offer.get_details()
      Api.ZCreate(dir, d.name).then((f) => {
        let wq = Promise.resolve()
        let got = 0
        let failed = false
        offer.accept({
          on_input: (chunk) => {
            const u8 = chunk instanceof Uint8Array ? chunk : Uint8Array.from(chunk)
            got += u8.length
            const b64 = bytesToB64(u8)
            wq = wq.then(() => Api.ZWrite(f.handle, b64)).catch(() => { failed = true })
            showBar(`Recebendo ${f.name}: ${fmtSize(got)}${d.size ? ' de ' + fmtSize(d.size) : ''}`)
          },
        }).then(async () => {
          await wq
          const mt = d.mtime instanceof Date ? Math.floor(d.mtime.getTime() / 1000) : 0
          await Api.ZClose(f.handle, !failed, mt)
          if (failed) toast(`Falha ao gravar ${f.name}`, true)
          else { count++; toast(`Recebido: ${f.name}`) }
        }).catch(async (e) => { await Api.ZClose(f.handle, false, 0); toast('Zmodem: ' + cleanErr(e), true) })
      }).catch((e) => { offer.skip(); toast('Zmodem: ' + cleanErr(e), true) })
    })
    session.on('session_end', () => {
      finish()
      if (count > 1) toast(`${count} arquivos recebidos`)
    })
    session.start()
  }

  async function send(detection) {
    const files = await Api.ZPickSend()
    if (!files || !files.length) { detection.deny(); return }
    session = detection.confirm()
    state.active = true
    showBar('Zmodem: enviando...')
    let remaining = files.length
    let bytesLeft = files.reduce((n, f) => n + f.size, 0)
    try {
      for (const f of files) {
        const xfer = await session.send_offer({
          name: f.name, size: f.size, mtime: new Date(f.mtime * 1000), mode: f.mode | 0o100000,
          files_remaining: remaining, bytes_remaining: bytesLeft,
        })
        remaining--
        if (!xfer) { bytesLeft -= f.size; await Api.ZClose(f.handle, true, 0); continue } // o servidor pulou
        let sent = 0
        for (;;) {
          const b64 = await Api.ZRead(f.handle, READ)
          if (!b64) break
          const u8 = b64ToU8(b64)
          for (let i = 0; i < u8.length; i += CHUNK) {
            const part = u8.subarray(i, Math.min(i + CHUNK, u8.length))
            xfer.send(part)
            sent += part.length
          }
          showBar(`Enviando ${f.name}: ${fmtSize(sent)} de ${fmtSize(f.size)}`)
          await sendQ // espera esvaziar a fila antes de ler mais
          if (!state.active) throw new Error('cancelado')
        }
        await xfer.end()
        bytesLeft -= f.size
        await Api.ZClose(f.handle, true, 0)
      }
      await session.close()
      toast(files.length > 1 ? `${files.length} arquivos enviados` : `Enviado: ${files[0].name}`)
    } catch (e) {
      for (const f of files) Api.ZClose(f.handle, false, 0).catch(() => {})
      if (state.active) throw e
    } finally {
      finish()
    }
  }

  return {
    state,
    get active() { return state.active },
    consume(bytes) { sentry.consume(bytes) },
    dispose() { try { session?.abort() } catch { /* ok */ } finish() },
  }
}
