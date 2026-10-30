//go:build windows

package main

import (
	"net/url"
	"sort"
	"strconv"

	"nxaterm/internal/store"

	"golang.org/x/sys/windows/registry"
)

// subKeyNames lista as subchaves. A assinatura de ReadSubKeyNames mudou entre versões do
// x/sys (com e sem o argumento de quantidade); aceitar as duas evita quebrar ao atualizar.
func subKeyNames(k registry.Key) ([]string, error) {
	if f, ok := any(k).(interface{ ReadSubKeyNames() ([]string, error) }); ok {
		return f.ReadSubKeyNames()
	}
	return any(k).(interface{ ReadSubKeyNames(int) ([]string, error) }).ReadSubKeyNames(-1)
}

// puttySessions lê as sessões do PuTTY direto do registro do usuário.
func puttySessions() ([]store.Session, error) {
	root, err := registry.OpenKey(registry.CURRENT_USER, `Software\SimonTatham\PuTTY\Sessions`, registry.READ)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	names, err := subKeyNames(root)
	if err != nil {
		return nil, err
	}
	var out []store.Session
	for _, n := range names {
		k, err := registry.OpenKey(root, n, registry.READ)
		if err != nil {
			continue
		}
		vals := map[string]string{}
		for _, v := range []string{"HostName", "Protocol", "UserName", "PublicKeyFile"} {
			if s, _, err := k.GetStringValue(v); err == nil {
				vals[v] = s
			}
		}
		if p, _, err := k.GetIntegerValue("PortNumber"); err == nil {
			vals["PortNumber"] = strconv.FormatUint(p, 16)
		}
		k.Close()
		name := n
		if u, err := url.PathUnescape(n); err == nil {
			name = u
		}
		if s, ok := store.PuTTYSession(name, vals); ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// wslDistros lê as distribuições instaladas do registro (a distro padrão vem primeiro).
func wslDistros() []string {
	root, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Lxss`, registry.READ)
	if err != nil {
		return nil
	}
	defer root.Close()
	def, _, _ := root.GetStringValue("DefaultDistribution")
	ids, err := subKeyNames(root)
	if err != nil {
		return nil
	}
	var first string
	var rest []string
	for _, id := range ids {
		k, err := registry.OpenKey(root, id, registry.READ)
		if err != nil {
			continue
		}
		name, _, err := k.GetStringValue("DistributionName")
		k.Close()
		if err != nil || name == "" {
			continue
		}
		if id == def {
			first = name
		} else {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	if first != "" {
		return append([]string{first}, rest...)
	}
	return rest
}
