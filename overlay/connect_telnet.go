package main

import (
	"fmt"
	"net"
	"strconv"

	"nxaterm/internal/store"
	"nxaterm/internal/telnet"
)

// startTelnet abre uma conexão Telnet. O login é feito no próprio terminal.
func (a *TermService) startTelnet(s store.Session, cols, rows int) (*termSession, error) {
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	raw, err := a.dialTCP(s, addr, 0)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar em %s: %w", addr, err)
	}
	conn := telnet.NewConn(raw, termTypeOf(s), cols, rows)
	return &termSession{
		w:       conn,
		wRaw:    binaryWriter{conn},
		setRaw:  conn.SetBinary,
		out:     conn,
		resize:  conn.Resize,
		closers: []func(){func() { conn.Close() }},
		enter:   "\r",
	}, nil
}

// binaryWriter envia bytes pelo Telnet sem converter CR (só duplica o 0xFF).
type binaryWriter struct{ c *telnet.Conn }

func (b binaryWriter) Write(p []byte) (int, error) { return b.c.WriteBinary(p) }
