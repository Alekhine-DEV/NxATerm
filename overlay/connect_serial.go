package main

import (
	"fmt"
	"sort"
	"strings"

	"nxaterm/internal/store"

	"go.bug.st/serial"
)

// parityOf converte o texto salvo no perfil para o tipo da biblioteca.
func parityOf(p string) serial.Parity {
	switch p {
	case "odd":
		return serial.OddParity
	case "even":
		return serial.EvenParity
	case "mark":
		return serial.MarkParity
	case "space":
		return serial.SpaceParity
	}
	return serial.NoParity
}

func stopBitsOf(s string) serial.StopBits {
	switch s {
	case "1.5":
		return serial.OnePointFiveStopBits
	case "2":
		return serial.TwoStopBits
	}
	return serial.OneStopBit
}

// startSerial abre a porta serial. Tudo o que chega vai para o terminal e tudo
// o que é digitado vai para a porta.
func startSerial(s store.Session) (*termSession, error) {
	port, err := serial.Open(s.SerialPort, &serial.Mode{
		BaudRate: s.BaudRate,
		DataBits: s.DataBits,
		Parity:   parityOf(s.Parity),
		StopBits: stopBitsOf(s.StopBits),
	})
	if err != nil {
		return nil, fmt.Errorf("não foi possível abrir %s: %w", s.SerialPort, err)
	}
	return &termSession{
		w:       port,
		out:     port,
		closers: []func(){func() { port.Close() }},
		enter:   "\r",
	}, nil
}

// SerialPorts lista as portas seriais do computador (COM3, /dev/ttyUSB0...).
func (a *TermService) SerialPorts() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return []string{}, err
	}
	sort.Strings(ports)
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out, nil
}
