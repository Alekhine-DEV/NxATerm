package store

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ParseSSHConfig lê um arquivo no formato do OpenSSH (~/.ssh/config) e devolve
// um perfil para cada bloco "Host" que não use curingas (* ? !).
// Os perfis importados usam autenticação por chave quando há IdentityFile e
// por senha caso contrário.
func ParseSSHConfig(r io.Reader, home string) []Session {
	var (
		out     []Session
		current []*Session // um bloco "Host a b c" gera uma sessão por alias
		hosts   []string
	)

	flush := func() {
		for _, c := range current {
			out = append(out, *c)
		}
		current = nil
	}

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v := splitKeyword(line)
		if k == "" {
			continue
		}
		switch strings.ToLower(k) {
		case "host":
			flush()
			hosts = strings.Fields(v)
			for _, h := range hosts {
				if strings.ContainsAny(h, "*?!") {
					continue
				}
				current = append(current, &Session{
					Name:       h,
					Host:       h, // HostName sobrescreve abaixo
					Port:       22,
					AuthMethod: "password",
					Folder:     "ssh_config",
				})
			}
		case "match":
			flush()
			hosts = nil
		case "hostname":
			for _, c := range current {
				c.Host = unquote(v)
			}
		case "user":
			for _, c := range current {
				c.User = unquote(v)
			}
		case "port":
			if p, err := strconv.Atoi(unquote(v)); err == nil {
				for _, c := range current {
					c.Port = p
				}
			}
		case "identityfile":
			path := expandHome(unquote(v), home)
			for _, c := range current {
				if c.KeyPath == "" { // vale o primeiro IdentityFile, como no ssh
					c.KeyPath = path
					c.AuthMethod = "key"
				}
			}
		}
	}
	flush()

	// Descarta blocos sem usuário: não dá para conectar sem saber o usuário.
	res := out[:0]
	for _, s := range out {
		if s.User != "" {
			res = append(res, s)
		}
	}
	return res
}

// DefaultSSHConfigPath devolve ~/.ssh/config.
func DefaultSSHConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

// splitKeyword separa "Chave valor" ou "Chave=valor".
func splitKeyword(line string) (string, string) {
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return line, ""
	}
	k := line[:i]
	v := strings.TrimSpace(strings.TrimLeft(line[i:], " \t="))
	return k, v
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}
