//go:build !windows

package netx

import "os/exec"

func hideWindow(*exec.Cmd) {}
