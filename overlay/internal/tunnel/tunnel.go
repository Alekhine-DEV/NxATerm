// Package tunnel implementa o encaminhamento de portas sobre uma conexão SSH: local, remoto e
// dinâmico (proxy SOCKS5). Só depende de um "cliente" capaz de discar e escutar, o que o
// *ssh.Client do golang.org/x/crypto/ssh já é.
package tunnel

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

// Client é o que o túnel precisa da conexão SSH.
type Client interface {
	Dial(network, addr string) (net.Conn, error)
	Listen(network, addr string) (net.Listener, error)
}

const (
	Local   = "local"   // 127.0.0.1:porta (aqui) -> destino (visto do servidor)
	Remote  = "remote"  // 127.0.0.1:porta (servidor) -> destino (visto daqui)
	Dynamic = "dynamic" // proxy SOCKS5 em 127.0.0.1:porta (aqui), saindo pelo servidor
)

// Tunnel é um encaminhamento em andamento.
type Tunnel struct {
	Kind       string
	ListenPort int
	DestHost   string
	DestPort   int
	Desc       string

	ln   net.Listener
	once sync.Once
}

// Stop fecha o ponto de escuta (as conexões em andamento terminam sozinhas).
func (t *Tunnel) Stop() { t.once.Do(func() { t.ln.Close() }) }

// Start abre o túnel. Só escuta em 127.0.0.1.
func Start(c Client, kind string, listenPort int, destHost string, destPort int) (*Tunnel, error) {
	if listenPort < 1 || listenPort > 65535 {
		return nil, errors.New("porta de escuta inválida (1-65535)")
	}
	if kind != Dynamic {
		if destHost == "" {
			return nil, errors.New("informe o host de destino")
		}
		if destPort < 1 || destPort > 65535 {
			return nil, errors.New("porta de destino inválida (1-65535)")
		}
	}
	listen := net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort))
	dest := net.JoinHostPort(destHost, strconv.Itoa(destPort))
	t := &Tunnel{Kind: kind, ListenPort: listenPort, DestHost: destHost, DestPort: destPort}
	var err error
	switch kind {
	case Local:
		t.ln, err = net.Listen("tcp", listen)
		t.Desc = fmt.Sprintf("local 127.0.0.1:%d → %s", listenPort, dest)
	case Remote:
		t.ln, err = c.Listen("tcp", listen)
		t.Desc = fmt.Sprintf("remoto (servidor) 127.0.0.1:%d → %s", listenPort, dest)
	case Dynamic:
		t.ln, err = net.Listen("tcp", listen)
		t.Desc = fmt.Sprintf("SOCKS5 127.0.0.1:%d", listenPort)
	default:
		return nil, errors.New("tipo de túnel desconhecido")
	}
	if err != nil {
		return nil, fmt.Errorf("não foi possível abrir o túnel: %w", err)
	}
	go func() {
		for {
			conn, err := t.ln.Accept()
			if err != nil {
				return
			}
			switch kind {
			case Local:
				go func() {
					rc, err := c.Dial("tcp", dest)
					if err != nil {
						conn.Close()
						return
					}
					pipe(conn, rc)
				}()
			case Remote:
				go func() {
					lc, err := net.DialTimeout("tcp", dest, 10*time.Second)
					if err != nil {
						conn.Close()
						return
					}
					pipe(conn, lc)
				}()
			case Dynamic:
				go socks5(c, conn)
			}
		}
	}()
	return t, nil
}

// pipe copia nos dois sentidos e fecha tudo quando um lado terminar.
func pipe(a, b io.ReadWriteCloser) {
	done := make(chan struct{}, 2)
	cp := func(dst, src io.ReadWriteCloser) {
		io.Copy(dst, src)
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	a.Close()
	b.Close()
	<-done
}

// socks5 implementa o mínimo do SOCKS5 (sem autenticação, só CONNECT).
func socks5(client Client, c net.Conn) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(15 * time.Second))
	buf := make([]byte, 262)

	// saudação: VER NMETHODS METHODS...
	if _, err := io.ReadFull(c, buf[:2]); err != nil || buf[0] != 5 {
		return
	}
	if _, err := io.ReadFull(c, buf[:int(buf[1])]); err != nil {
		return
	}
	c.Write([]byte{5, 0})

	// pedido: VER CMD RSV ATYP ...
	if _, err := io.ReadFull(c, buf[:4]); err != nil || buf[0] != 5 {
		return
	}
	if buf[1] != 1 { // só CONNECT
		c.Write([]byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	var host string
	switch buf[3] {
	case 1:
		if _, err := io.ReadFull(c, buf[:4]); err != nil {
			return
		}
		host = net.IP(append([]byte(nil), buf[:4]...)).String()
	case 4:
		if _, err := io.ReadFull(c, buf[:16]); err != nil {
			return
		}
		host = net.IP(append([]byte(nil), buf[:16]...)).String()
	case 3:
		if _, err := io.ReadFull(c, buf[:1]); err != nil {
			return
		}
		n := int(buf[0])
		if _, err := io.ReadFull(c, buf[:n]); err != nil {
			return
		}
		host = string(buf[:n])
	default:
		c.Write([]byte{5, 8, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return
	}
	port := int(buf[0])<<8 | int(buf[1])

	rc, err := client.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	c.SetDeadline(time.Time{})
	pipe(c, rc)
}
