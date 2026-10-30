// Único arquivo que conhece o caminho dos bindings gerados pelo Wails v3.
//
// `wails3 generate bindings` (ou `wails3 dev` / `wails3 build`) cria a pasta
// frontend/bindings/<módulo-go>/ . Como o módulo se chama "nxaterm" e o serviço
// TermService fica no pacote main, o import abaixo é o esperado.
//
// Se a versão do Wails que você instalou gerar outro formato, só este arquivo
// precisa mudar. Alternativas comuns:
//   import * as Api from '../bindings/nxaterm/termservice.js'
//   import { TermService as Api } from '../bindings/nxaterm/index.js'
import { TermService } from '../bindings/nxaterm'

export const Api = TermService
