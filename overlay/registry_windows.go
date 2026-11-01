//go:build windows

package main

import (
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

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

// linkHandlerRegistered confere se ssh:// aponta para este executável.
func linkHandlerRegistered() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\ssh\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	return err == nil && strings.Contains(strings.ToLower(v), strings.ToLower(exe))
}

// registerLinkHandler grava, só para o usuário atual, o programa que abre ssh:// e telnet://.
func registerLinkHandler(exe string) error {
	for _, scheme := range []string{"ssh", "telnet"} {
		base := `Software\Classes\` + scheme
		k, _, err := registry.CreateKey(registry.CURRENT_USER, base, registry.ALL_ACCESS)
		if err != nil {
			return err
		}
		k.SetStringValue("", "URL:"+scheme)
		k.SetStringValue("URL Protocol", "")
		k.Close()
		cmd, _, err := registry.CreateKey(registry.CURRENT_USER, base+`\shell\open\command`, registry.ALL_ACCESS)
		if err != nil {
			return err
		}
		err = cmd.SetStringValue("", `"`+exe+`" "%1"`)
		cmd.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
