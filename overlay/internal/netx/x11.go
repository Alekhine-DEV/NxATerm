package netx

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const x11Proto = "MIT-MAGIC-COOKIE-1"

// Display é o servidor X local para onde o X11 do servidor remoto é levado.
type Display struct {
	Network string // "unix" ou "tcp"
	Addr    string
	Num     int
}

// ParseDisplay entende valores como ":0", ":0.0", "localhost:10.0" e "host:1".
// Vazio usa :0 (ou localhost:0 no Windows, onde o servidor X roda como VcXsrv).
func ParseDisplay(s string) (Display, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		if runtime.GOOS == "windows" {
			s = "localhost:0"
		} else {
			s = ":0"
		}
	}
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return Display{}, fmt.Errorf("DISPLAY inválido: %q", s)
	}
	host, rest := s[:i], s[i+1:]
	if j := strings.Index(rest, "."); j >= 0 {
		rest = rest[:j]
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 0 || n > 1000 {
		return Display{}, fmt.Errorf("DISPLAY inválido: %q", s)
	}
	d := Display{Num: n}
	if host == "" || host == "unix" {
		d.Network = "unix"
		d.Addr = "/tmp/.X11-unix/X" + strconv.Itoa(n)
		return d, nil
	}
	d.Network = "tcp"
	d.Addr = fmt.Sprintf("%s:%d", host, 6000+n)
	return d, nil
}

// XAuthority procura o cookie de acesso do display no arquivo Xauthority.
// Devolve nome e dados vazios quando não há (servidores abertos, como o VcXsrv com -ac).
func XAuthority(path string, displayNum int) (name string, data []byte) {
	if path == "" {
		path = os.Getenv("XAUTHORITY")
	}
	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".Xauthority")
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil
	}
	return findCookie(raw, displayNum)
}

func findCookie(raw []byte, displayNum int) (string, []byte) {
	want := strconv.Itoa(displayNum)
	r := bytes.NewReader(raw)
	u16 := func() (int, bool) {
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, false
		}
		return int(binary.BigEndian.Uint16(b[:])), true
	}
	str := func() ([]byte, bool) {
		n, ok := u16()
		if !ok {
			return nil, false
		}
		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			return nil, false
		}
		return b, true
	}
	for {
		if _, ok := u16(); !ok { // família
			return "", nil
		}
		if _, ok := str(); !ok { // endereço
			return "", nil
		}
		num, ok := str()
		if !ok {
			return "", nil
		}
		name, ok := str()
		if !ok {
			return "", nil
		}
		data, ok := str()
		if !ok {
			return "", nil
		}
		if string(num) == want && string(name) == x11Proto {
			return string(name), data
		}
	}
}

func pad4(n int) int { return (n + 3) &^ 3 }

// RewriteSetup lê o pacote inicial de uma conexão X11 vinda do servidor remoto
// (que traz o cookie falso combinado no x11-req) e devolve o mesmo pacote com o
// cookie verdadeiro do display local. O resto da conexão passa sem alteração.
func RewriteSetup(r io.Reader, fake []byte, realName string, realData []byte) ([]byte, error) {
	head := make([]byte, 12)
	if _, err := io.ReadFull(r, head); err != nil {
		return nil, err
	}
	var order binary.ByteOrder
	switch head[0] {
	case 'l':
		order = binary.LittleEndian
	case 'B':
		order = binary.BigEndian
	default:
		return nil, errors.New("pacote X11 inválido")
	}
	nameLen := int(order.Uint16(head[6:8]))
	dataLen := int(order.Uint16(head[8:10]))
	if nameLen > 256 || dataLen > 256 {
		return nil, errors.New("pacote X11 inválido")
	}
	body := make([]byte, pad4(nameLen)+pad4(dataLen))
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	name := body[:nameLen]
	data := body[pad4(nameLen) : pad4(nameLen)+dataLen]
	if string(name) != x11Proto || !bytes.Equal(data, fake) {
		return nil, errors.New("cookie X11 recusado")
	}

	out := make([]byte, 12)
	copy(out, head)
	order.PutUint16(out[6:8], uint16(len(realName)))
	order.PutUint16(out[8:10], uint16(len(realData)))
	out = append(out, []byte(realName)...)
	out = append(out, make([]byte, pad4(len(realName))-len(realName))...)
	out = append(out, realData...)
	out = append(out, make([]byte, pad4(len(realData))-len(realData))...)
	return out, nil
}
