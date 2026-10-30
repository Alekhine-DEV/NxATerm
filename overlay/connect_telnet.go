package main

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"nxaterm/internal/store"
	"nxaterm/internal/telnet"
)

// startTelnet abre uma conexão Telnet. O login é feito no próprio terminal.
func startTelnet(s store.Session, cols, rows int) (*termSession, error) {
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	raw, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar em %s: %w", addr, err)
	}
	conn := telnet.NewConn(raw, "xterm-256color", cols, rows)
	return &termSession{
		w:       conn,
		out:     conn,
		resize:  conn.Resize,
		closers: []func(){func() { conn.Close() }},
		enter:   "\r",
	}, nil
}
