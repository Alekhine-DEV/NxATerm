package tunnel

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// servidor SSH mínimo: aceita qualquer login e atende direct-tcpip e tcpip-forward
func sshServer(t *testing.T) (addr string) {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				go func() {
					for r := range reqs {
						if r.Type != "tcpip-forward" {
							r.Reply(false, nil)
							continue
						}
						var p struct {
							Host string
							Port uint32
						}
						ssh.Unmarshal(r.Payload, &p)
						l, err := net.Listen("tcp", net.JoinHostPort(p.Host, strconv.Itoa(int(p.Port))))
						if err != nil {
							r.Reply(false, nil)
							continue
						}
						r.Reply(true, nil)
						go func() {
							for {
								c, err := l.Accept()
								if err != nil {
									return
								}
								la := c.LocalAddr().(*net.TCPAddr)
								ra := c.RemoteAddr().(*net.TCPAddr)
								payload := ssh.Marshal(struct {
									Host  string
									Port  uint32
									OHost string
									OPort uint32
								}{p.Host, uint32(la.Port), ra.IP.String(), uint32(ra.Port)})
								ch, creqs, err := sc.OpenChannel("forwarded-tcpip", payload)
								if err != nil {
									c.Close()
									continue
								}
								go ssh.DiscardRequests(creqs)
								go func() { pipe(c, ch) }()
							}
						}()
					}
				}()
				for nch := range chans {
					if nch.ChannelType() != "direct-tcpip" {
						nch.Reject(ssh.UnknownChannelType, "no")
						continue
					}
					var p struct {
						Host  string
						Port  uint32
						OHost string
						OPort uint32
					}
					ssh.Unmarshal(nch.ExtraData(), &p)
					dc, err := net.Dial("tcp", net.JoinHostPort(p.Host, strconv.Itoa(int(p.Port))))
					if err != nil {
						nch.Reject(ssh.ConnectionFailed, err.Error())
						continue
					}
					ch, creqs, _ := nch.Accept()
					go ssh.DiscardRequests(creqs)
					go pipe(dc, ch)
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func client(t *testing.T) *ssh.Client {
	addr := sshServer(t)
	c, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{User: "u", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// serviço de eco, o "alvo" dos túneis
func echo(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func freePort(t *testing.T) int {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func roundtrip(t *testing.T, conn net.Conn, msg string) {
	t.Helper()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != msg {
		t.Fatalf("eco: %q %v", buf, err)
	}
}

func TestLocal(t *testing.T) {
	c := client(t)
	target := echo(t)
	lp := freePort(t)
	tn, err := Start(c, Local, lp, "127.0.0.1", target)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", lp))
	if err != nil {
		t.Fatal(err)
	}
	roundtrip(t, conn, "olá túnel local")
	conn.Close()
	tn.Stop()
	tn.Stop() // idempotente
	if _, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", lp), time.Second); err == nil {
		t.Fatal("a porta devia estar fechada")
	}
}

func TestRemote(t *testing.T) {
	c := client(t)
	target := echo(t)
	rp := freePort(t)
	tn, err := Start(c, Remote, rp, "127.0.0.1", target)
	if err != nil {
		t.Fatal(err)
	}
	defer tn.Stop()
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", rp))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	roundtrip(t, conn, "olá túnel remoto")
}

func TestDynamic(t *testing.T) {
	c := client(t)
	target := echo(t)
	lp := freePort(t)
	tn, err := Start(c, Dynamic, lp, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer tn.Stop()
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", lp))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	conn.Write([]byte{5, 1, 0})
	rep := make([]byte, 2)
	if _, err := io.ReadFull(conn, rep); err != nil || rep[0] != 5 || rep[1] != 0 {
		t.Fatalf("saudação: %v %v", rep, err)
	}
	req := []byte{5, 1, 0, 3, byte(len("localhost"))}
	req = append(req, "localhost"...)
	pb := make([]byte, 2)
	binary.BigEndian.PutUint16(pb, uint16(target))
	conn.Write(append(req, pb...))
	rep = make([]byte, 10)
	if _, err := io.ReadFull(conn, rep); err != nil || rep[1] != 0 {
		t.Fatalf("resposta: %v %v", rep, err)
	}
	roundtrip(t, conn, "olá socks")
	// destino que não existe: erro 5 (conexão recusada)
	c2, _ := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", lp))
	defer c2.Close()
	c2.SetDeadline(time.Now().Add(5 * time.Second))
	c2.Write([]byte{5, 1, 0})
	io.ReadFull(c2, make([]byte, 2))
	bad := append([]byte{5, 1, 0, 1, 127, 0, 0, 1}, 0, 1) // 127.0.0.1:1
	c2.Write(bad)
	r2 := make([]byte, 10)
	if _, err := io.ReadFull(c2, r2); err != nil || r2[1] == 0 {
		t.Fatalf("devia recusar: %v %v", r2, err)
	}
}

func TestValidation(t *testing.T) {
	c := client(t)
	for _, tc := range []struct {
		k    string
		p    int
		h    string
		dp   int
		want bool
	}{
		{Local, 0, "x", 1, true}, {Local, 70000, "x", 1, true}, {Local, 1000, "", 80, true}, {Local, 1000, "x", 0, true},
		{"estranho", 1000, "x", 80, true},
	} {
		if _, err := Start(c, tc.k, tc.p, tc.h, tc.dp); (err != nil) != tc.want {
			t.Errorf("%+v: err=%v", tc, err)
		}
	}
	// porta ocupada
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	if _, err := Start(c, Local, l.Addr().(*net.TCPAddr).Port, "x", 80); err == nil {
		t.Error("porta ocupada devia falhar")
	}
}

func TestBindAndInUse(t *testing.T) {
	c := client(t)
	target := echo(t)
	lp := freePort(t)
	tn, err := StartBind(c, Local, "0.0.0.0", lp, "127.0.0.1", target)
	if err != nil {
		t.Fatal(err)
	}
	defer tn.Stop()
	if addr := tn.ln.Addr().(*net.TCPAddr); !addr.IP.IsUnspecified() {
		t.Fatalf("devia escutar em todas as interfaces, escuta em %v", addr)
	}
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", lp))
	if err != nil {
		t.Fatal(err)
	}
	roundtrip(t, conn, "olá rede")
	conn.Close()

	// mesma porta e mesmo endereço de novo: mensagem clara (no Windows 127.0.0.1 convive com 0.0.0.0, por isso o mesmo endereço)
	if _, err := StartBind(c, Local, "0.0.0.0", lp, "127.0.0.1", target); err == nil || !strings.Contains(err.Error(), "já está em uso") {
		t.Fatalf("esperava 'já está em uso', veio %v", err)
	}
	// o padrão continua sendo só 127.0.0.1
	lp2 := freePort(t)
	t2, err := Start(c, Dynamic, lp2, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer t2.Stop()
	if addr := t2.ln.Addr().(*net.TCPAddr); !addr.IP.IsLoopback() {
		t.Fatalf("devia escutar só em 127.0.0.1, escuta em %v", addr)
	}
	if _, err := StartBind(c, Local, "10.1.2.3", lp2, "h", 1); err == nil {
		t.Fatal("endereço de escuta arbitrário devia ser recusado")
	}
}
