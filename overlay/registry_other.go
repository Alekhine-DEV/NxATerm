//go:build !windows

package main

import (
	"errors"
	"os/exec"
	"strings"

	"nxaterm/internal/store"
)

func puttySessions() ([]store.Session, error) { return nil, errors.New("só existe no Windows") }

func wslDistros() []string { return nil }

// linkHandlerRegistered confere (via xdg-mime) se o NxATerm abre links ssh://.
func linkHandlerRegistered() bool {
	out, err := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/ssh").Output()
	return err == nil && strings.Contains(string(out), "nxaterm")
}

// registerLinkHandler define o NxATerm como programa dos links ssh:// e telnet:// do usuário.
func registerLinkHandler(string) error {
	if _, err := exec.LookPath("xdg-mime"); err != nil {
		return errors.New("o comando xdg-mime não foi encontrado")
	}
	for _, scheme := range []string{"ssh", "telnet"} {
		if err := exec.Command("xdg-mime", "default", "nxaterm.desktop", "x-scheme-handler/"+scheme).Run(); err != nil {
			return err
		}
	}
	return nil
}
