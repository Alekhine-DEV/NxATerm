// Package netx reúne as formas de abrir uma conexão TCP além da direta:
// proxy SOCKS5, proxy HTTP (CONNECT) e ProxyCommand.
package netx

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Proxy descreve um proxy SOCKS5 ou HTTP.
type Proxy struct {
	Scheme   string // "socks5" ou "http"
	Addr     string // host:porta
	User     string
	Password string
}

// ParseProxy lê endereços como socks5://host:1080, socks5://usuario@host:1080
// ou http://host:3128. A senha não é guardada em disco: quem chama a informa em SetPassword.
func ParseProxy(raw string) (*Proxy, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "socks5://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("endereço de proxy inválido: %w", err)
	}
	p := &Proxy{}
	switch strings.ToLower(u.Scheme) {
	case "socks5", "socks5h", "socks":
		p.Scheme = "socks5"
	case "http", "https":
		p.Scheme = "http"
	default:
		return nil, fmt.Errorf("tipo de proxy não suportado: %q (use socks5 ou http)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, errors.New("proxy sem endereço")
	}
	port := u.Port()
	if port == "" {
		if p.Scheme == "socks5" {
			port = "1080"
		} else {
			port = "8080"
		}
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return nil, fmt.Errorf("porta de proxy inválida: %q", port)
	}
	p.Addr = net.JoinHostPort(host, port)
	if u.User != nil {
		p.User = u.User.Username()
		p.Password, _ = u.User.Password()
	}
	return p, nil
}

// String devolve o endereço sem a senha.
func (p *Proxy) String() string {
	u := p.User
	if u != "" {
		u += "@"
	}
	return p.Scheme + "://" + u + p.Addr
}

// NeedsPassword diz se o proxy tem usuário mas ainda falta a senha.
func (p *Proxy) NeedsPassword() bool { return p.User != "" && p.Password == "" }

// Dial abre uma conexão até addr (host:porta) passando pelo proxy.
func (p *Proxy) Dial(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", p.Addr)
	if err != nil {
		return nil, fmt.Errorf("não consegui falar com o proxy %s: %w", p.Addr, err)
	}
	if timeout > 0 {
		conn.SetDeadline(time.Now().Add(timeout))
	}
	switch p.Scheme {
	case "socks5":
		err = socks5Connect(conn, addr, p.User, p.Password)
	default:
		err = httpConnect(conn, addr, p.User, p.Password)
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	return conn, nil
}

func socks5Connect(c net.Conn, addr, user, pass string) error {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return err
	}
	methods := []byte{0x00}
	if user != "" {
		methods = []byte{0x00, 0x02}
	}
	if _, err := c.Write(append([]byte{0x05, byte(len(methods))}, methods...)); err != nil {
		return err
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(c, reply); err != nil {
		return fmt.Errorf("proxy SOCKS5 não respondeu: %w", err)
	}
	if reply[0] != 0x05 {
		return errors.New("o endereço não é de um proxy SOCKS5")
	}
	switch reply[1] {
	case 0x00:
	case 0x02:
		if len(user) > 255 || len(pass) > 255 {
			return errors.New("usuário ou senha do proxy longos demais")
		}
		msg := []byte{0x01, byte(len(user))}
		msg = append(msg, user...)
		msg = append(msg, byte(len(pass)))
		msg = append(msg, pass...)
		if _, err := c.Write(msg); err != nil {
			return err
		}
		ok := make([]byte, 2)
		if _, err := io.ReadFull(c, ok); err != nil {
			return err
		}
		if ok[1] != 0x00 {
			return errors.New("o proxy recusou o usuário ou a senha")
		}
	default:
		return errors.New("o proxy SOCKS5 exige uma autenticação que o NxATerm não suporta")
	}

	req := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			req = append(req, 0x01)
			req = append(req, v4...)
		} else {
			req = append(req, 0x04)
			req = append(req, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return errors.New("nome de host longo demais")
		}
		req = append(req, 0x03, byte(len(host)))
		req = append(req, host...)
	}
	req = append(req, byte(port>>8), byte(port))
	if _, err := c.Write(req); err != nil {
		return err
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(c, head); err != nil {
		return err
	}
	if head[1] != 0x00 {
		return fmt.Errorf("o proxy não conseguiu chegar em %s (%s)", addr, socksError(head[1]))
	}
	var skip int
	switch head[3] {
	case 0x01:
		skip = 4 + 2
	case 0x04:
		skip = 16 + 2
	case 0x03:
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return err
		}
		skip = int(l[0]) + 2
	default:
		return errors.New("resposta inválida do proxy SOCKS5")
	}
	_, err = io.CopyN(io.Discard, c, int64(skip))
	return err
}

func socksError(code byte) string {
	switch code {
	case 0x01:
		return "falha geral"
	case 0x02:
		return "conexão não permitida pelo proxy"
	case 0x03:
		return "rede inalcançável"
	case 0x04:
		return "host inalcançável"
	case 0x05:
		return "conexão recusada"
	case 0x06:
		return "tempo esgotado"
	case 0x07:
		return "comando não suportado"
	case 0x08:
		return "tipo de endereço não suportado"
	}
	return "erro " + strconv.Itoa(int(code))
}

func httpConnect(c net.Conn, addr, user, pass string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
	if user != "" {
		b.WriteString("Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)) + "\r\n")
	}
	b.WriteString("\r\n")
	if _, err := io.WriteString(c, b.String()); err != nil {
		return err
	}
	br := bufio.NewReader(c)
	status, err := br.ReadString('\n')
	if err != nil {
		return fmt.Errorf("proxy HTTP não respondeu: %w", err)
	}
	parts := strings.SplitN(strings.TrimSpace(status), " ", 3)
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "HTTP/") {
		return errors.New("o endereço não é de um proxy HTTP")
	}
	if parts[1] != "200" {
		if parts[1] == "407" {
			return errors.New("o proxy HTTP recusou o usuário ou a senha")
		}
		return fmt.Errorf("o proxy HTTP recusou a conexão: %s", strings.TrimSpace(status))
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return err
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	if br.Buffered() > 0 {
		return errors.New("o proxy HTTP enviou dados antes da hora")
	}
	return nil
}
