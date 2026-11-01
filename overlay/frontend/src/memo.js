// Senhas memorizadas: só na memória desta execução. Nada vai para disco, e tudo some ao fechar o NxATerm.
export const memo = new Map()

/** Chave de uma senha: `pw` (senha), `pp` (senha da chave); a mesma que o backend usa nos servidores de salto. */
export const memoKey = (kind, s) => `${kind}:${s.user}@${s.host}:${s.port}`
