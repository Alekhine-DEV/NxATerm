// O Trzsz usa a API de arquivos do navegador (showOpenFilePicker, showDirectoryPicker e os
// "handles"). O WebView do app não a oferece (ou exige um gesto do usuário que aqui não existe),
// então este módulo põe no lugar uma versão que conversa com o backend: o diálogo de escolha é o
// do sistema e o disco é acessado por fichas (internal/tzfs), nunca por caminhos do servidor.
import { Api } from './api.js'

const READ = 1 << 20 // Go devolve no máximo isso por chamada
const WRITE = 1 << 20

let ownerOf = () => ''
/** Define quem é o "dono" (aba) das fichas: a aba que está transferindo. */
export function setOwnerResolver(fn) { ownerOf = fn }

function domError(name, msg) {
  const e = new Error(msg)
  e.name = name
  return e
}

function b64ToU8(b64) {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}
function u8ToB64(u8) {
  let s = ''
  for (let i = 0; i < u8.length; i += 0x8000) s += String.fromCharCode.apply(null, u8.subarray(i, i + 0x8000))
  return btoa(s)
}

/** Imita File: só o que o Trzsz usa (name, size, slice().arrayBuffer()). */
class DiskFile {
  constructor(node) {
    this.name = node.name
    this.size = node.size
    this._tok = node.token
  }
  slice(a, b) { return { arrayBuffer: () => this._read(a, b) } }
  async _read(a, b) {
    const out = new Uint8Array(Math.max(0, b - a))
    let got = 0
    while (got < out.length) {
      const b64 = await Api.TzRead(this._tok, a + got, Math.min(READ, out.length - got))
      if (!b64) break
      const u = b64ToU8(b64)
      out.set(u, got)
      got += u.length
    }
    return out.buffer.slice(0, got)
  }
}

class Writable {
  constructor(handle) { this._h = handle }
  async write(buf) {
    const u8 = buf instanceof Uint8Array ? buf : new Uint8Array(buf)
    for (let i = 0; i < u8.length; i += WRITE) await Api.TzWrite(this._h, u8ToB64(u8.subarray(i, i + WRITE)))
  }
  async close() { await Api.TzCloseWrite(this._h) }
}

class FileHandle {
  constructor(name, node, parentTok) {
    this.kind = 'file'
    this.name = name
    this._node = node // null se o arquivo ainda não existe
    this._parent = parentTok
  }
  async getFile() { return new DiskFile(this._node) }
  async createWritable() {
    if (!this._parent) throw domError('NoModificationAllowedError', 'sem pasta de destino')
    return new Writable(await Api.TzOpenWrite(this._parent, this.name, true))
  }
}

class DirHandle {
  constructor(node) {
    this.kind = 'directory'
    this.name = node.name
    this._node = node
    this._kids = null
  }
  async _list() {
    this._kids = (await Api.TzList(this._node.token)) || []
    return this._kids
  }
  async *values() {
    for (const n of await this._list()) yield this._wrap(n)
  }
  _wrap(n) { return n.kind === 'directory' ? new DirHandle(n) : new FileHandle(n.name, n, this._node.token) }
  async _find(name, kind) {
    const look = (l) => l.find((k) => k.name === name && k.kind === kind)
    return (this._kids && look(this._kids)) || look(await this._list())
  }
  async getFileHandle(name, opts) {
    const n = await this._find(name, 'file')
    if (n) return this._wrap(n)
    if (opts && opts.create) return new FileHandle(name, null, this._node.token)
    throw domError('NotFoundError', name)
  }
  async getDirectoryHandle(name, opts) {
    const n = await this._find(name, 'directory')
    if (n) return this._wrap(n)
    if (opts && opts.create) return new DirHandle(await Api.TzMkdir(this._node.token, name))
    throw domError('NotFoundError', name)
  }
}

let installed = false
/** Troca showOpenFilePicker e showDirectoryPicker pela versão do app. */
export function installTrzszFs() {
  if (installed) return
  installed = true
  const def = (k, v) => Object.defineProperty(window, k, { value: v, configurable: true, writable: true })
  def('showOpenFilePicker', async () => {
    const nodes = await Api.TzPickFiles(ownerOf())
    if (!nodes || !nodes.length) throw domError('AbortError', 'cancelado')
    return nodes.map((n) => new FileHandle(n.name, n, ''))
  })
  def('showDirectoryPicker', async (opts) => {
    const save = !!opts && (opts.mode === 'readwrite' || opts.id === 'trzsz_download')
    const n = await Api.TzPickDir(ownerOf(), save)
    if (!n || !n.token) throw domError('AbortError', 'cancelado')
    return new DirHandle(n)
  })
}
