// Package launch entende o que o sistema entrega ao abrir o NxATerm: links
// ssh://usuario@host:porta, telnet://host e sftp://... ou "--session nome".
package launch

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Item é algo para abrir: uma URL ("ssh://...") ou o nome de uma sessão salva ("session:Servidor").
type Item = string

// Collect pega, nos argumentos do programa (sem o nome do executável), o que deve ser aberto.
func Collect(args []string) []Item {
	var out []Item
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		switch {
		case a == "--session" || a == "-s" || a == "--open":
			if i+1 < len(args) {
				i++
				if n := strings.TrimSpace(args[i]); n != "" {
					out = append(out, "session:"+n)
				}
			}
		case strings.HasPrefix(a, "--session="):
			if n := strings.TrimSpace(strings.TrimPrefix(a, "--session=")); n != "" {
				out = append(out, "session:"+n)
			}
		case isLink(a):
			out = append(out, a)
		}
	}
	return out
}

func isLink(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "ssh://") || strings.HasPrefix(l, "telnet://") || strings.HasPrefix(l, "sftp://")
}

// Target é o destino descrito por um link.
type Target struct {
	Type string // ssh, telnet ou sftp
	User string
	Host string
	Port int
}

// FromURL lê um link ssh://usuario@host:porta, telnet://host:porta ou sftp://usuario@host.
func FromURL(raw string) (Target, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Target{}, fmt.Errorf("link inválido: %w", err)
	}
	t := Target{Type: strings.ToLower(u.Scheme)}
	if t.Type != "ssh" && t.Type != "telnet" && t.Type != "sftp" {
		return Target{}, fmt.Errorf("tipo de link não suportado: %q", u.Scheme)
	}
	t.Host = u.Hostname()
	if t.Host == "" {
		return Target{}, fmt.Errorf("o link não tem host: %q", raw)
	}
	if u.User != nil {
		t.User = u.User.Username()
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return Target{}, fmt.Errorf("porta inválida no link: %q", p)
		}
		t.Port = n
	}
	if t.Type != "telnet" && t.User == "" {
		return Target{}, fmt.Errorf("o link precisa do usuário (%s://usuario@host)", t.Type)
	}
	return t, nil
}
