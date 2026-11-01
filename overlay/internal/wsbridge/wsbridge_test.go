package wsbridge

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// cliente WebSocket mínimo para o teste
func wsDial(t *testing.T, url string) (net.Conn, *bufio.Reader) {
	t.Helper()
	u := strings.TrimPrefix(url, "ws://")
	i := strings.Index(u, "/")
	c, err := net.Dial("tcp", u[:i])
	if err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))
	c.Write([]byte("GET " + u[i:] + " HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: " + key + "\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: binary\r\n\r\n"))
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != 101 {
		t.Fatalf("handshake: %v %v", err, resp)
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	if resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) || resp.Header.Get("Sec-WebSocket-Protocol") != "binary" {
		t.Fatalf("cabeçalhos: %v", resp.Header)
	}
	return c, br
}

func maskedFrame(op byte, payload []byte) []byte {
	mask := []byte{1, 2, 3, 4}
	var b bytes.Buffer
	b.WriteByte(0x80 | op)
	switch n := len(payload); {
	case n < 126:
		b.WriteByte(0x80 | byte(n))
	default:
		b.WriteByte(0x80 | 126)
		b.WriteByte(byte(n >> 8))
		b.WriteByte(byte(n))
	}
	b.Write(mask)
	for i, c := range payload {
		b.WriteByte(c ^ mask[i%4])
	}
	return b.Bytes()
}

func TestBridgeEcho(t *testing.T) {
	// servidor TCP que devolve o que recebe, em maiúsculas
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.Write([]byte("RFB 003.008\n"))
		buf := make([]byte, 4096)
		for {
			n, err := c.Read(buf)
			if err != nil {
				return
			}
			c.Write(bytes.ToUpper(buf[:n]))
		}
	}()
	url, err := Start(context.Background(), 5*time.Second, func(ctx context.Context) (net.Conn, error) {
		return net.Dial("tcp", ln.Addr().String())
	})
	if err != nil {
		t.Fatal(err)
	}
	// caminho errado é recusado e não gasta a ponte
	bad := strings.TrimSuffix(url, url[strings.LastIndex(url, "/")+1:]) + "errado"
	u := strings.TrimPrefix(bad, "ws://")
	c0, _ := net.Dial("tcp", u[:strings.Index(u, "/")])
	c0.Write([]byte("GET " + u[strings.Index(u, "/"):] + " HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nSec-WebSocket-Key: abc\r\n\r\n"))
	line, _ := bufio.NewReader(c0).ReadString('\n')
	c0.Close()
	if !strings.Contains(line, "404") {
		t.Fatalf("caminho errado: %q", line)
	}

	c, br := wsDial(t, url)
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	op, p, _, err := readServerFrame(br)
	if err != nil || op != 0x2 || string(p) != "RFB 003.008\n" {
		t.Fatalf("banner: %v %x %q", err, op, p)
	}
	c.Write(maskedFrame(0x2, []byte("ola vnc")))
	op, p, _, err = readServerFrame(br)
	if err != nil || string(p) != "OLA VNC" {
		t.Fatalf("eco: %v %q", err, p)
	}
	big := bytes.Repeat([]byte("a"), 3000)
	c.Write(maskedFrame(0x2, big))
	var got []byte
	for len(got) < len(big) {
		_, p, _, err = readServerFrame(br)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, p...)
	}
	if string(got) != strings.ToUpper(string(big)) {
		t.Fatal("quadro grande diferente")
	}
	c.Write(maskedFrame(0x9, []byte("p")))
	op, _, _, _ = readServerFrame(br)
	if op != 0xA {
		t.Fatalf("pong esperado, veio %x", op)
	}
	c.Write(maskedFrame(0x8, []byte{0x03, 0xe8}))
	op, _, _, _ = readServerFrame(br)
	if op != 0x8 {
		t.Fatalf("close esperado, veio %x", op)
	}
	// a ponte atendeu uma conexão e se encerrou: uma segunda não conecta
	time.Sleep(100 * time.Millisecond)
	if c2, err := net.DialTimeout("tcp", strings.TrimPrefix(url[:strings.LastIndex(url, "/")], "ws://"), time.Second); err == nil {
		c2.Close()
		t.Fatal("a ponte deveria ter fechado o listener")
	}
}

func TestDialFailureClosesSocket(t *testing.T) {
	url, _ := Start(context.Background(), 5*time.Second, func(ctx context.Context) (net.Conn, error) {
		return nil, io.ErrUnexpectedEOF
	})
	c, br := wsDial(t, url)
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	op, p, _, err := readServerFrame(br)
	if err != nil || op != 0x8 || !strings.Contains(string(p), "falha") {
		t.Fatalf("%v %x %q", err, op, p)
	}
}

func TestNobodyConnects(t *testing.T) {
	url, _ := Start(context.Background(), 150*time.Millisecond, func(ctx context.Context) (net.Conn, error) { return nil, io.EOF })
	time.Sleep(400 * time.Millisecond)
	if c, err := net.DialTimeout("tcp", strings.TrimPrefix(url[:strings.LastIndex(url, "/")], "ws://"), time.Second); err == nil {
		c.Close()
		t.Fatal("deveria ter expirado")
	}
}

// quadros do servidor não têm máscara
func readServerFrame(r io.Reader) (op byte, payload []byte, fin bool, err error) {
	var h [2]byte
	if _, err = io.ReadFull(r, h[:]); err != nil {
		return
	}
	fin = h[0]&0x80 != 0
	op = h[0] & 0x0F
	n := int(h[1] & 0x7F)
	if n == 126 {
		var b [2]byte
		io.ReadFull(r, b[:])
		n = int(b[0])<<8 | int(b[1])
	} else if n == 127 {
		var b [8]byte
		io.ReadFull(r, b[:])
		n = int(b[6])<<8 | int(b[7])
	}
	payload = make([]byte, n)
	_, err = io.ReadFull(r, payload)
	return
}
