package store

import (
	"net/url"
	"strconv"
	"strings"
)

// PuTTYSession converte os valores de uma sessão salva do PuTTY (HostName, Protocol, UserName,
// PublicKeyFile e PortNumber, este em hexadecimal) em uma sessão do NxATerm.
// Devolve false para tipos sem equivalente (raw, serial, rlogin) ou sem host.
func PuTTYSession(name string, vals map[string]string) (Session, bool) {
	host := vals["HostName"]
	if host == "" {
		return Session{}, false
	}
	proto := strings.ToLower(vals["Protocol"])
	if proto == "" {
		proto = "ssh"
	}
	s := Session{Name: name, Folder: "PuTTY", Host: host, AuthMethod: "password"}
	if p, err := strconv.ParseInt(strings.TrimPrefix(vals["PortNumber"], "0x"), 16, 32); err == nil {
		s.Port = int(p)
	}
	switch proto {
	case "ssh":
		s.Type = TypeSSH
		s.User = vals["UserName"]
		if s.User == "" {
			s.User = "root"
			s.Notes = "Usuário não informado no PuTTY: ajuste antes de conectar."
		}
		if k := vals["PublicKeyFile"]; k != "" {
			if strings.HasSuffix(strings.ToLower(k), ".ppk") {
				s.Notes = strings.TrimSpace(s.Notes + "\nChave original do PuTTY: " + k + " (.ppk: converta para OpenSSH e configure aqui).")
			} else {
				s.AuthMethod, s.KeyPath = "key", k
			}
		}
	case "telnet":
		s.Type = TypeTelnet
	default:
		return Session{}, false
	}
	return s, true
}

// ParsePuTTYReg interpreta a saída de
//
//	reg query HKCU\Software\SimonTatham\PuTTY\Sessions /s
//
// e devolve as sessões SSH e Telnet. As sessões ficam na pasta "PuTTY".
func ParsePuTTYReg(out string) []Session {
	var res []Session
	var cur map[string]string
	var curName string
	flush := func() {
		if cur == nil {
			return
		}
		if s, ok := PuTTYSession(curName, cur); ok {
			res = append(res, s)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") { // chave do registro
			flush()
			cur = map[string]string{}
			base := line[strings.LastIndex(line, `\`)+1:]
			if n, err := url.PathUnescape(base); err == nil {
				base = n
			}
			curName = base
			if strings.EqualFold(base, "Sessions") {
				cur = nil // a própria raiz, sem valores
			}
			continue
		}
		if cur == nil {
			continue
		}
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) >= 3 && strings.HasPrefix(f[1], "REG_") {
			cur[f[0]] = strings.Join(f[2:], " ")
		}
	}
	flush()
	return res
}
