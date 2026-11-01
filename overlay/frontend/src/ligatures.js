// Ligaduras de fonte no terminal. O addon oficial do xterm lê o arquivo da fonte pelo Node, o que o
// WebView não tem; aqui o terminal só precisa saber ONDE há sequências que as fontes de programação
// (Fira Code, JetBrains Mono, Cascadia Code, Iosevka...) transformam em ligadura: o desenho é feito
// pelo navegador. Sequência que a fonte escolhida não tem como ligadura aparece normalmente.

const FIXED = [
  ':::', '::=', '::', ':=', '=:', ':>', ':<', '<:', '>:', '!==', '!===', '!=', '===', '==', '=/=', '=!=',
  '/=', '<>', '>=>', '>>=', '>>-', '>->', '>-', '-<<', '-<', '=<<', '=<', '<=<', '<-<', '<<<', '>>>',
  '<<', '>>', '>=', '||=', '|=', '|-', '-|', '|--', '--|', '^=', '~=', '=~', '!~', '*=', '/**', '**/',
  '/*', '*/', '...', '..<', '..=', '.=', '.-', '.?', '?:', '?.', '?=', '#{', '#[', '#(', '#_(', '#_',
  '#?', '#!', '#:', '#=', '>]', '-~', '~-', '&=', '~@', ':?', '@_', '{{', '}}', '[<', ']#', '_|_', '\\/', '/\\', '[|', '|]', '{|', '|}', 'www', '</>', '</', '/>', '<!---', '<!--',
]
// repetições: ===== ----- ~~~~ ___ ### (longas) e ... +++ *** /// ;;; (curtas)
const LONG_RUNS = ['={2,}', '-{2,}(?!\\|)', '~{2,}', '#{2,}', '_{2,}']
const RUNS = ['\\.{2,3}', '\\+{2,3}', '\\*{2,3}', '/{2,3}',
  ';{2,3}', '!{2,3}', '%%', '&{2,3}', '\\|{2,3}', '\\?{2,3}', '\\\\{2,3}', '<{2,3}', '>{2,3}']

const esc = (s) => s.replace(/[.*+?^${}()|[\]\\\/]/g, '\\$&')
const RE = new RegExp([
  '<!-{2,3}',
  '<{1,2}[-=]+>?', '[-=]+>{1,2}', // <- <-- <== <=> -> --> => ==> ->>
  '<\\|{1,3}>?', '\\|{1,3}>', // <| <|> |> ||>
  '<~{1,2}>?', '~{1,2}>', // <~ <~~ <~> ~> ~~>
  '<[$*+]>?', '[$*+]>', // <$ <$> $> <* <*> *> <+ <+> +>
  ...LONG_RUNS,
  ...FIXED.slice().sort((a, b) => b.length - a.length).map(esc),
  ...RUNS,
].join('|'), 'g')

/** Trechos [início, fim) de `text` que podem virar ligadura (formato do registerCharacterJoiner do xterm). */
export function ligatureRanges(text) {
  const out = []
  RE.lastIndex = 0
  let m
  while ((m = RE.exec(text))) {
    if (m[0].length < 2) { RE.lastIndex = m.index + 1; continue }
    out.push([m.index, m.index + m[0].length])
  }
  return out
}

/** Liga ou desliga as ligaduras de um terminal. */
export function setLigatures(term, on) {
  if (on && term._ligId == null) term._ligId = term.registerCharacterJoiner(ligatureRanges)
  else if (!on && term._ligId != null) {
    term.deregisterCharacterJoiner(term._ligId)
    term._ligId = null
  }
  if (term.rows) term.refresh(0, term.rows - 1)
}
