//go:build windows

package main

import (
	"errors"
	"io"
	"net"
	"os"
)

// dialAgent conecta ao ssh-agent: um soquete (Git para Windows) ou o serviço
// "OpenSSH Authentication Agent" do próprio Windows.
func dialAgent() (io.ReadWriteCloser, error) {
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		return net.Dial("unix", sock)
	}
	f, err := os.OpenFile(`\\.\pipe\openssh-ssh-agent`, os.O_RDWR, 0)
	if err != nil {
		return nil, errors.New(`o ssh-agent do Windows não está em execução (inicie o serviço "OpenSSH Authentication Agent")`)
	}
	return f, nil
}
