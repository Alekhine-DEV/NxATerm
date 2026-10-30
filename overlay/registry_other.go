//go:build !windows

package main

import (
	"errors"

	"nxaterm/internal/store"
)

func puttySessions() ([]store.Session, error) { return nil, errors.New("só existe no Windows") }

func wslDistros() []string { return nil }
