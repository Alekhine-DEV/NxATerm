//go:build !windows && !linux && !darwin

package main

import "errors"

// setGlobalHotkey: sem suporte neste sistema.
func setGlobalHotkey(spec string, onPress func()) error {
	if spec == "" {
		return nil
	}
	return errors.New("o atalho global não está disponível neste sistema")
}
