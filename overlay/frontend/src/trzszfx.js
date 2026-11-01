// Trzsz (trz/tsz) no terminal, usando a biblioteca trzsz. O protocolo roda aqui; os bytes seguem
// por WriteBytes e os arquivos pelo backend (trzszfs.js).
import { TrzszFilter } from 'trzsz'
import { Api } from './api.js'
import { installTrzszFs, setOwnerResolver } from './trzszfs.js'

const live = new Set()
setOwnerResolver(() => {
  for (const t of live) if (t.filter.isTransferringFiles()) return t.id
  return ''
})

// "TRZSZ:TRANSFER:" — só os pedaços que trazem isso precisam passar pelo filtro
const NEEDLE = Uint8Array.from('TRZSZ:TRANSFER:', (c) => c.charCodeAt(0))
function hasMagic(u8) {
  for (let i = u8.indexOf(0x54); i >= 0; i = u8.indexOf(0x54, i + 1)) { // 'T'
    if (i + NEEDLE.length > u8.length) return false
    let j = 0
    while (j < NEEDLE.length && u8[i + j] === NEEDLE[j]) j++
    if (j === NEEDLE.length) return true
  }
  return false
}

function bytesOf(input) {
  if (typeof input !== 'string') return input
  const out = new Uint8Array(input.length)
  for (let i = 0; i < input.length; i++) out[i] = input.charCodeAt(i) & 0xff
  return out
}
function b64Of(u8) {
  let s = ''
  for (let i = 0; i < u8.length; i += 0x8000) s += String.fromCharCode.apply(null, u8.subarray(i, i + 0x8000))
  return btoa(s)
}

/**
 * Liga o Trzsz a um terminal. Devolve { output(bytes), input(texto), active, dispose() }.
 * `write` recebe o que vai para a tela (bytes, já passando pela codificação da sessão).
 */
export function createTrzsz({ id, term, write, windowsShell = false }) {
  installTrzszFs()
  let sendQ = Promise.resolve()
  let busy = false
  let timer = 0
  let idle = 0

  const toScreen = (o) => {
    if (typeof o === 'string') term.write(o)
    else if (o instanceof Uint8Array) write(o)
    else if (o instanceof ArrayBuffer) write(new Uint8Array(o))
  }
  const send = (input) => {
    const b64 = b64Of(bytesOf(input))
    sendQ = sendQ.then(() => Api.WriteBytes(id, b64)).catch(() => {})
  }
  const filter = new TrzszFilter({
    writeToTerminal: toScreen,
    sendToServer: send,
    terminalColumns: term.cols,
    isWindowsShell: windowsShell,
  })
  const self = { id, filter }
  live.add(self)

  // acompanha o início e o fim da transferência (modo binário do Telnet e limpeza das fichas)
  const watch = () => {
    const now = filter.isTransferringFiles()
    if (now) idle = 0
    if (now && !busy) {
      busy = true
      Api.SetRawMode(id, true).catch(() => {})
    } else if (!now && busy) {
      busy = false
      Api.SetRawMode(id, false).catch(() => {})
      Api.TzRelease(id).catch(() => {})
    }
    if (!now && ++idle > 20) { clearInterval(timer); timer = 0 }
  }
  const poke = () => {
    idle = 0
    if (!timer) timer = setInterval(watch, 100)
  }

  return {
    get active() { return filter.isTransferringFiles() },
    output(bytes) {
      if (filter.isTransferringFiles() || hasMagic(bytes)) {
        filter.processServerOutput(bytes)
        poke()
      } else write(bytes)
    },
    /** Teclas durante a transferência: só Ctrl+C faz algo (cancela). */
    input(text) { filter.processTerminalInput(text) },
    resize(cols) { filter.setTerminalColumns(cols) },
    dispose() {
      live.delete(self)
      clearInterval(timer)
      timer = 0
      try { filter.stopTransferringFiles() } catch { /* nada em andamento */ }
      Api.TzRelease(id).catch(() => {})
    },
  }
}
