//go:build !windows

package main

import (
	"errors"
	"io"
	"net"
	"os"
)

// dialAgent conecta ao ssh-agent do usuário.
func dialAgent() (io.ReadWriteCloser, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, errors.New("o ssh-agent não está em execução (SSH_AUTH_SOCK vazio)")
	}
	return net.Dial("unix", sock)
}
