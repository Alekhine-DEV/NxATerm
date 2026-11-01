//go:build windows

package netx

import (
	"os/exec"
	"syscall"
)

// hideWindow evita que o console do comando apareça piscando.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
