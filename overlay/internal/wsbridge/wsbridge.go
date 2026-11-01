// Package wsbridge liga um WebSocket local a uma conexão TCP (para o noVNC falar com um servidor VNC).
// Implementa só o necessário do RFC 6455: handshake, quadros binários, ping/pong e fechamento.
// Cada ponte atende UMA conexão, num caminho secreto aleatório, só em 127.0.0.1.
package wsbridge

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// maxFrame limita um quadro recebido do navegador (teclas e cliques são pequenos).
const maxFrame = 1 << 20

// Start abre a ponte. dial é chamado quando o WebSocket conecta. Devolve a URL ws:// a usar.
// A ponte se encerra sozinha se ninguém conectar em waitConn, ou quando a conexão terminar.
func Start(ctx context.Context, waitConn time.Duration, dial func(context.Context) (net.Conn, error)) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	tokBytes := make([]byte, 16)
	if _, err := rand.Read(tokBytes); err != nil {
		ln.Close()
		return "", err
	}
	path := "/" + hex.EncodeToString(tokBytes)
	url := fmt.Sprintf("ws://%s%s", ln.Addr().String(), path)
	go serve(ctx, ln, path, waitConn, dial)
	return url, nil
}

func serve(ctx context.Context, ln net.Listener, path string, waitConn time.Duration, dial func(context.Context) (net.Conn, error)) {
	defer ln.Close()
	if tl, ok := ln.(*net.TCPListener); ok {
		tl.SetDeadline(time.Now().Add(waitConn))
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		ok := handle(ctx, c, path, dial)
		if ok {
			return // uma conexão só
		}
	}
}

// handle devolve true se o handshake foi aceito (e a ponte rodou até o fim).
func handle(ctx context.Context, c net.Conn, path string, dial func(context.Context) (net.Conn, error)) bool {
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return false
	}
	key := req.Header.Get("Sec-WebSocket-Key")
	if req.URL.Path != path || !strings.EqualFold(req.Header.Get("Upgrade"), "websocket") || key == "" {
		c.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
		return false
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
		base64.StdEncoding.EncodeToString(sum[:]) + "\r\n"
	for _, p := range strings.Split(req.Header.Get("Sec-WebSocket-Protocol"), ",") {
		if strings.TrimSpace(p) == "binary" {
			resp += "Sec-WebSocket-Protocol: binary\r\n"
			break
		}
	}
	if _, err := c.Write([]byte(resp + "\r\n")); err != nil {
		return true
	}
	c.SetDeadline(time.Time{})

	dctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	tcp, err := dial(dctx)
	cancel()
	if err != nil {
		writeClose(c, 1011, "falha ao conectar: "+err.Error())
		return true
	}
	defer tcp.Close()

	var wmu sync.Mutex
	done := make(chan struct{}, 2)

	// servidor VNC -> navegador
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := tcp.Read(buf)
			if n > 0 {
				wmu.Lock()
				werr := writeFrame(c, 0x2, buf[:n])
				wmu.Unlock()
				if werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		wmu.Lock()
		writeClose(c, 1000, "")
		wmu.Unlock()
		done <- struct{}{}
	}()
	// navegador -> servidor VNC
	go func() {
		readLoop(br, c, &wmu, tcp)
		done <- struct{}{}
	}()
	<-done
	tcp.Close()
	c.Close()
	<-done
	return true
}

func readLoop(br *bufio.Reader, c net.Conn, wmu *sync.Mutex, tcp net.Conn) {
	var msg []byte
	for {
		op, payload, fin, err := readFrame(br)
		if err != nil {
			return
		}
		switch op {
		case 0x8: // close
			wmu.Lock()
			writeClose(c, 1000, "")
			wmu.Unlock()
			return
		case 0x9: // ping
			wmu.Lock()
			writeFrame(c, 0xA, payload)
			wmu.Unlock()
		case 0xA: // pong
		case 0x0, 0x1, 0x2:
			if op != 0x0 {
				msg = msg[:0]
			}
			msg = append(msg, payload...)
			if len(msg) > maxFrame {
				return
			}
			if fin {
				if _, err := tcp.Write(msg); err != nil {
					return
				}
				msg = msg[:0]
			}
		default:
			return
		}
	}
}

// readFrame lê um quadro do cliente (sempre mascarado).
func readFrame(r io.Reader) (op byte, payload []byte, fin bool, err error) {
	var h [2]byte
	if _, err = io.ReadFull(r, h[:]); err != nil {
		return
	}
	fin = h[0]&0x80 != 0
	op = h[0] & 0x0F
	masked := h[1]&0x80 != 0
	n := uint64(h[1] & 0x7F)
	switch n {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if !masked {
		err = errors.New("quadro sem máscara")
		return
	}
	if n > maxFrame {
		err = errors.New("quadro grande demais")
		return
	}
	var mask [4]byte
	if _, err = io.ReadFull(r, mask[:]); err != nil {
		return
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(r, payload); err != nil {
		return
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return
}

func writeFrame(w io.Writer, op byte, payload []byte) error {
	hdr := []byte{0x80 | op}
	switch n := len(payload); {
	case n < 126:
		hdr = append(hdr, byte(n))
	case n < 65536:
		hdr = append(hdr, 126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 127)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		hdr = append(hdr, b[:]...)
	}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func writeClose(w io.Writer, code uint16, reason string) {
	if len(reason) > 100 {
		reason = reason[:100]
	}
	p := []byte{byte(code >> 8), byte(code)}
	p = append(p, reason...)
	writeFrame(w, 0x8, p)
}
