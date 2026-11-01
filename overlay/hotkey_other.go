//go:build !windows

package main

import "errors"

// setGlobalHotkey: o atalho global só existe no Windows (no Linux depende do ambiente gráfico).
func setGlobalHotkey(spec string, onPress func()) error {
	if spec == "" {
		return nil
	}
	return errors.New("o atalho global só está disponível no Windows")
}
