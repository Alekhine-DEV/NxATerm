package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// TunnelInfo descreve um túnel SSH ativo (aparece na janela "Túneis").
type TunnelInfo struct {
	ID         string `json:"id"`
	TermID     string `json:"termId"`
	Kind       string `json:"kind"` // "local", "remote" ou "dynamic"
	ListenPort int    `json:"listenPort"`
	DestHost   string `json:"destHost"`
	DestPort   int    `json:"destPort"`
	Desc       string `json:"desc"`
}

type tunnel struct {
	info TunnelInfo
	ln   net.Listener
	once sync.Once
}

func (t *tunnel) stop() { t.once.Do(func() { t.ln.Close() }) }

// TunnelStart abre um túnel usando a conexão SSH da aba termID.
//
//	local   : 127.0.0.1:listenPort (aqui)  ->  destHost:destPort (visto do servidor)
//	remote  : 127.0.0.1:listenPort (servidor) -> destHost:destPort (visto daqui)
//	dynamic : proxy SOCKS5 em 127.0.0.1:listenPort (aqui), saindo pelo servidor
//
// Os túneis só escutam em 127.0.0.1 e acabam junto com a aba.
func (a *TermService) TunnelStart(termID, kind string, listenPort int, destHost string, destPort int) (TunnelInfo, error) {
	ts, err := a.term(termID)
	if err != nil {
		return TunnelInfo{}, err
	}
	client, err := ts.sshClient()
	if err != nil {
		return TunnelInfo{}, errors.New("túneis só funcionam em sessões SSH e SFTP")
	}
	if listenPort < 1 || listenPort > 65535 {
		return TunnelInfo{}, errors.New("porta de escuta inválida (1-65535)")
	}
	if kind != "dynamic" {
		if destHost == "" {
			return TunnelInfo{}, errors.New("informe o host de destino")
		}
		if destPort < 1 || destPort > 65535 {
			return TunnelInfo{}, errors.New("porta de destino inválida (1-65535)")
		}
	}
	listen := net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort))
	dest := net.JoinHostPort(destHost, strconv.Itoa(destPort))

	tn := &tunnel{info: TunnelInfo{
		TermID: termID, Kind: kind, ListenPort: listenPort, DestHost: destHost, DestPort: destPort,
	}}
	switch kind {
	case "local":
		tn.ln, err = net.Listen("tcp", listen)
		tn.info.Desc = fmt.Sprintf("local 127.0.0.1:%d → %s", listenPort, dest)
	case "remote":
		tn.ln, err = client.Listen("tcp", listen)
		tn.info.Desc = fmt.Sprintf("remoto (servidor) 127.0.0.1:%d → %s", listenPort, dest)
	case "dynamic":
		tn.ln, err = net.Listen("tcp", listen)
		tn.info.Desc = fmt.Sprintf("SOCKS5 127.0.0.1:%d", listenPort)
	default:
		return TunnelInfo{}, errors.New("tipo de túnel desconhecido")
	}
	if err != nil {
		return TunnelInfo{}, fmt.Errorf("não foi possível abrir o túnel: %w", err)
	}
	b := make([]byte, 6)
	rand.Read(b)
	tn.info.ID = hex.EncodeToString(b)

	a.tunMu.Lock()
	a.tunnels[tn.info.ID] = tn
	a.tunMu.Unlock()

	go func() {
		for {
			c, err := tn.ln.Accept()
			if err != nil {
				return
			}
			switch kind {
			case "local":
				go func() {
					rc, err := client.Dial("tcp", dest)
					if err != nil {
						c.Close()
						return
					}
					pipe(c, rc)
				}()
			case "remote":
				go func() {
					lc, err := net.DialTimeout("tcp", dest, 10*time.Second)
					if err != nil {
						c.Close()
						return
					}
					pipe(c, lc)
				}()
			case "dynamic":
				go socks5(client, c)
			}
		}
	}()
	return tn.info, nil
}

// TunnelList devolve os túneis ativos.
func (a *TermService) TunnelList() []TunnelInfo {
	a.tunMu.Lock()
	defer a.tunMu.Unlock()
	out := make([]TunnelInfo, 0, len(a.tunnels))
	for _, t := range a.tunnels {
		out = append(out, t.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ListenPort < out[j].ListenPort })
	return out
}

// TunnelStop encerra um túnel.
func (a *TermService) TunnelStop(id string) error {
	a.tunMu.Lock()
	t, ok := a.tunnels[id]
	delete(a.tunnels, id)
	a.tunMu.Unlock()
	if !ok {
		return errors.New("túnel não encontrado")
	}
	t.stop()
	return nil
}

func (a *TermService) stopTunnelsOf(termID string) {
	a.tunMu.Lock()
	var list []*tunnel
	for id, t := range a.tunnels {
		if t.info.TermID == termID {
			list = append(list, t)
			delete(a.tunnels, id)
		}
	}
	a.tunMu.Unlock()
	for _, t := range list {
		t.stop()
	}
}

func (a *TermService) stopAllTunnels() {
	a.tunMu.Lock()
	list := make([]*tunnel, 0, len(a.tunnels))
	for id, t := range a.tunnels {
		list = append(list, t)
		delete(a.tunnels, id)
	}
	a.tunMu.Unlock()
	for _, t := range list {
		t.stop()
	}
}

// pipe copia nos dois sentidos e fecha tudo quando um lado terminar.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
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
func socks5(client *ssh.Client, c net.Conn) {
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
