package netx

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseProxy(t *testing.T) {
	p, err := ParseProxy("socks5://ana@10.0.0.1:9050")
	if err != nil || p.Scheme != "socks5" || p.Addr != "10.0.0.1:9050" || p.User != "ana" || !p.NeedsPassword() {
		t.Fatalf("%+v %v", p, err)
	}
	if p.String() != "socks5://ana@10.0.0.1:9050" {
		t.Fatal(p.String())
	}
	p, _ = ParseProxy("proxy.local:3128")
	if p.Scheme != "socks5" || p.Addr != "proxy.local:3128" {
		t.Fatalf("%+v", p)
	}
	p, _ = ParseProxy("http://proxy.local")
	if p.Scheme != "http" || p.Addr != "proxy.local:8080" {
		t.Fatalf("%+v", p)
	}
	if p, err := ParseProxy(""); p != nil || err != nil {
		t.Fatal("vazio deve dar nil")
	}
	for _, bad := range []string{"ftp://x:1", "socks5://:1080", "socks5://h:99999"} {
		if _, err := ParseProxy(bad); err == nil {
			t.Errorf("%q deveria falhar", bad)
		}
	}
}

// destino que ecoa o que recebe
func echoServer(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()
	return l.Addr().String()
}

func fakeSocks(t *testing.T, user, pass string) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				h := make([]byte, 2)
				io.ReadFull(c, h)
				m := make([]byte, h[1])
				io.ReadFull(c, m)
				if user != "" {
					c.Write([]byte{5, 2})
					b := make([]byte, 2)
					io.ReadFull(c, b)
					u := make([]byte, b[1])
					io.ReadFull(c, u)
					pl := make([]byte, 1)
					io.ReadFull(c, pl)
					pw := make([]byte, pl[0])
					io.ReadFull(c, pw)
					if string(u) != user || string(pw) != pass {
						c.Write([]byte{1, 1})
						return
					}
					c.Write([]byte{1, 0})
				} else {
					c.Write([]byte{5, 0})
				}
				req := make([]byte, 4)
				io.ReadFull(c, req)
				var host string
				switch req[3] {
				case 1:
					ip := make([]byte, 4)
					io.ReadFull(c, ip)
					host = net.IP(ip).String()
				case 3:
					l := make([]byte, 1)
					io.ReadFull(c, l)
					n := make([]byte, l[0])
					io.ReadFull(c, n)
					host = string(n)
				}
				pb := make([]byte, 2)
				io.ReadFull(c, pb)
				dst, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb)))))
				if err != nil {
					c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				defer dst.Close()
				c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
				go io.Copy(dst, c)
				io.Copy(c, dst)
			}()
		}
	}()
	return l.Addr().String()
}

func roundTrip(t *testing.T, c net.Conn) {
	t.Helper()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write([]byte("olá")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("olá"))
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "olá" {
		t.Fatalf("eco: %q %v", buf, err)
	}
}

func TestSocks5(t *testing.T) {
	dst := echoServer(t)
	for _, tc := range []struct{ user, pass string }{{"", ""}, {"ana", "segredo"}} {
		proxy := fakeSocks(t, tc.user, tc.pass)
		p := &Proxy{Scheme: "socks5", Addr: proxy, User: tc.user, Password: tc.pass}
		c, err := p.Dial(context.Background(), dst, 3*time.Second)
		if err != nil {
			t.Fatalf("%q: %v", tc.user, err)
		}
		roundTrip(t, c)
		c.Close()
	}
	p := &Proxy{Scheme: "socks5", Addr: fakeSocks(t, "ana", "segredo"), User: "ana", Password: "errada"}
	if _, err := p.Dial(context.Background(), dst, 3*time.Second); err == nil {
		t.Fatal("senha errada deveria falhar")
	}
}

func TestHTTPConnect(t *testing.T) {
	dst := echoServer(t)
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				var req []byte
				b := make([]byte, 1)
				for !bytes.HasSuffix(req, []byte("\r\n\r\n")) {
					if _, err := c.Read(b); err != nil {
						return
					}
					req = append(req, b...)
				}
				target := strings.Fields(string(req))[1]
				if !strings.Contains(string(req), "Proxy-Authorization: Basic YW5hOnB3") { // ana:pw
					c.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\n"))
					return
				}
				d, err := net.Dial("tcp", target)
				if err != nil {
					c.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
					return
				}
				defer d.Close()
				c.Write([]byte("HTTP/1.1 200 Connection established\r\nX: y\r\n\r\n"))
				go io.Copy(d, c)
				io.Copy(c, d)
			}()
		}
	}()
	ok := &Proxy{Scheme: "http", Addr: l.Addr().String(), User: "ana", Password: "pw"}
	c, err := ok.Dial(context.Background(), dst, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, c)
	c.Close()
	bad := &Proxy{Scheme: "http", Addr: l.Addr().String()}
	if _, err := bad.Dial(context.Background(), dst, 3*time.Second); err == nil || !strings.Contains(err.Error(), "recusou") {
		t.Fatalf("esperava recusa: %v", err)
	}
}

func TestExpandProxyCommand(t *testing.T) {
	got := ExpandProxyCommand("nc %h %p # %r %%", "srv", 2222, "ana")
	if got != "nc srv 2222 # ana %" {
		t.Fatal(got)
	}
}

func TestDialCommand(t *testing.T) {
	c, err := DialCommand("cat")
	if err != nil {
		t.Skip("sem sh/cat:", err)
	}
	defer c.Close()
	roundTrip(t, c)
}

func TestParseDisplay(t *testing.T) {
	cases := map[string]Display{
		":0":              {"unix", "/tmp/.X11-unix/X0", 0},
		":1.0":            {"unix", "/tmp/.X11-unix/X1", 1},
		"localhost:10.0":  {"tcp", "localhost:6010", 10},
		"unix:2":          {"unix", "/tmp/.X11-unix/X2", 2},
		"192.168.0.5:0.0": {"tcp", "192.168.0.5:6000", 0},
	}
	for in, want := range cases {
		got, err := ParseDisplay(in)
		if err != nil || got != want {
			t.Errorf("%q: %+v %v", in, got, err)
		}
	}
	if _, err := ParseDisplay("semdoispontos"); err == nil {
		t.Error("deveria falhar")
	}
}

func xauthEntry(num, name string, data []byte) []byte {
	var b bytes.Buffer
	w16 := func(n int) { binary.Write(&b, binary.BigEndian, uint16(n)) }
	w16(256)
	w16(4)
	b.WriteString("host")
	w16(len(num))
	b.WriteString(num)
	w16(len(name))
	b.WriteString(name)
	w16(len(data))
	b.Write(data)
	return b.Bytes()
}

func TestFindCookie(t *testing.T) {
	raw := append(xauthEntry("1", x11Proto, []byte{9, 9}), xauthEntry("0", x11Proto, []byte{1, 2, 3, 4})...)
	name, data := findCookie(raw, 0)
	if name != x11Proto || !bytes.Equal(data, []byte{1, 2, 3, 4}) {
		t.Fatalf("%q %v", name, data)
	}
	if n, d := findCookie(raw, 5); n != "" || d != nil {
		t.Fatal("display inexistente deveria vir vazio")
	}
}

func setupPacket(order binary.ByteOrder, bo byte, name string, data []byte, tail []byte) []byte {
	p := make([]byte, 12)
	p[0] = bo
	order.PutUint16(p[2:], 11)
	order.PutUint16(p[6:], uint16(len(name)))
	order.PutUint16(p[8:], uint16(len(data)))
	p = append(p, name...)
	p = append(p, make([]byte, pad4(len(name))-len(name))...)
	p = append(p, data...)
	p = append(p, make([]byte, pad4(len(data))-len(data))...)
	return append(p, tail...)
}

func TestRewriteSetup(t *testing.T) {
	fake := bytes.Repeat([]byte{0xAA}, 16)
	real := bytes.Repeat([]byte{0xBB}, 16)
	for _, c := range []struct {
		order binary.ByteOrder
		bo    byte
	}{{binary.LittleEndian, 'l'}, {binary.BigEndian, 'B'}} {
		in := setupPacket(c.order, c.bo, x11Proto, fake, []byte("resto"))
		r := bytes.NewReader(in)
		got, err := RewriteSetup(r, fake, x11Proto, real)
		if err != nil {
			t.Fatal(err)
		}
		if want := setupPacket(c.order, c.bo, x11Proto, real, nil); !bytes.Equal(got, want) {
			t.Fatalf("pacote diferente:\n%v\n%v", got, want)
		}
		rest, _ := io.ReadAll(r)
		if string(rest) != "resto" {
			t.Fatalf("resto = %q", rest)
		}
	}
	in := setupPacket(binary.LittleEndian, 'l', x11Proto, fake, nil)
	got, err := RewriteSetup(bytes.NewReader(in), fake, "", nil)
	if err != nil || !bytes.Equal(got, setupPacket(binary.LittleEndian, 'l', "", nil, nil)) {
		t.Fatalf("sem cookie: %v %v", got, err)
	}
	bad := setupPacket(binary.LittleEndian, 'l', x11Proto, bytes.Repeat([]byte{1}, 16), nil)
	if _, err := RewriteSetup(bytes.NewReader(bad), fake, x11Proto, real); err == nil {
		t.Fatal("cookie falso errado deveria ser recusado")
	}
}
