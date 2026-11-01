package main

import (
	"os/exec"
	"syscall"
)

// hideWindow evita que um console pisque ao abrir programas auxiliares.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
