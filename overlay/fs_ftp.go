package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/textproto"
	pathpkg "path"
	"strconv"
	"strings"
	"sync"
	"time"

	"nxaterm/internal/store"
	"nxaterm/internal/xfer"

	"github.com/jlaffaye/ftp"
)

// O protocolo FTP usa uma conexão de controle por vez; cada transferência ocupa a sua.
// Por isso o acesso passa por um pequeno conjunto de conexões (até 4), emprestadas a cada operação.
type ftpPool struct {
	dial   func() (*ftp.ServerConn, error)
	mu     sync.Mutex
	idle   []pooledConn
	sem    chan struct{}
	closed bool
	gone   chan struct{}
	home   string

	cacheMu sync.Mutex
	cache   map[string]dirCache
}

type pooledConn struct {
	c  *ftp.ServerConn
	at time.Time
}

type dirCache struct {
	at      time.Time
	entries []xfer.Entry
}

func newFTPPool(dial func() (*ftp.ServerConn, error)) *ftpPool {
	return &ftpPool{dial: dial, sem: make(chan struct{}, 4), gone: make(chan struct{}), cache: map[string]dirCache{}}
}

func (p *ftpPool) get() (*ftp.ServerConn, error) {
	p.sem <- struct{}{}
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			<-p.sem
			return nil, errors.New("conexão encerrada")
		}
		if n := len(p.idle); n > 0 {
			pc := p.idle[n-1]
			p.idle = p.idle[:n-1]
			p.mu.Unlock()
			if time.Since(pc.at) < 20*time.Second || pc.c.NoOp() == nil {
				return pc.c, nil
			}
			go pc.c.Quit()
			continue
		}
		p.mu.Unlock()
		break
	}
	c, err := p.dial()
	if err != nil {
		<-p.sem
		return nil, err
	}
	return c, nil
}

func (p *ftpPool) put(c *ftp.ServerConn, bad bool) {
	p.mu.Lock()
	if bad || p.closed {
		p.mu.Unlock()
		go c.Quit()
		<-p.sem
		return
	}
	p.idle = append(p.idle, pooledConn{c, time.Now()})
	p.mu.Unlock()
	<-p.sem
}

func (p *ftpPool) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	close(p.gone)
	idle := p.idle
	p.idle = nil
	p.mu.Unlock()
	for _, pc := range idle {
		go pc.c.Quit()
	}
}

func (p *ftpPool) invalidate() {
	p.cacheMu.Lock()
	p.cache = map[string]dirCache{}
	p.cacheMu.Unlock()
}

// isConnErr diz se o erro quebrou a conexão (e não é só uma recusa do servidor, como "550").
func isConnErr(err error) bool {
	if err == nil {
		return false
	}
	var te *textproto.Error
	return !errors.As(err, &te)
}

// ftpFS adapta o FTP para o motor de transferência.
type ftpFS struct{ p *ftpPool }

func (f ftpFS) with(fn func(c *ftp.ServerConn) error) error {
	c, err := f.p.get()
	if err != nil {
		return err
	}
	err = fn(c)
	f.p.put(c, isConnErr(err))
	return err
}

func ftpEntry(e *ftp.Entry) xfer.Entry {
	return xfer.Entry{
		Name: e.Name, Size: int64(e.Size), IsDir: e.Type == ftp.EntryTypeFolder,
		IsLink: e.Type == ftp.EntryTypeLink, ModTime: e.Time,
	}
}

func (f ftpFS) List(dir string) ([]xfer.Entry, error) {
	f.p.cacheMu.Lock()
	if c, ok := f.p.cache[dir]; ok && time.Since(c.at) < 3*time.Second {
		out := append([]xfer.Entry(nil), c.entries...)
		f.p.cacheMu.Unlock()
		return out, nil
	}
	f.p.cacheMu.Unlock()
	var out []xfer.Entry
	err := f.with(func(c *ftp.ServerConn) error {
		es, err := c.List(dir)
		if err != nil {
			return err
		}
		for _, e := range es {
			if e.Name == "." || e.Name == ".." {
				continue
			}
			out = append(out, ftpEntry(e))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	f.p.cacheMu.Lock()
	f.p.cache[dir] = dirCache{time.Now(), append([]xfer.Entry(nil), out...)}
	f.p.cacheMu.Unlock()
	return out, nil
}

func (f ftpFS) Stat(p string) (xfer.Entry, error) {
	if p == "/" {
		return xfer.Entry{Name: "/", IsDir: true}, nil
	}
	var got xfer.Entry
	err := f.with(func(c *ftp.ServerConn) error {
		e, err := c.GetEntry(p)
		if err != nil {
			return err
		}
		got = ftpEntry(e)
		got.Name = pathpkg.Base(p)
		return nil
	})
	if err == nil {
		return got, nil
	}
	// servidor sem MLST: procura na listagem da pasta
	list, lerr := f.List(pathpkg.Dir(p))
	if lerr != nil {
		var te *textproto.Error
		if errors.As(lerr, &te) && te.Code == 550 {
			return xfer.Entry{}, fmt.Errorf("%s: %w", p, fs.ErrNotExist)
		}
		return xfer.Entry{}, lerr
	}
	for _, e := range list {
		if e.Name == pathpkg.Base(p) {
			return e, nil
		}
	}
	return xfer.Entry{}, fmt.Errorf("%s: %w", p, fs.ErrNotExist)
}

// Lstat no FTP é a própria listagem da pasta (que não segue links).
func (f ftpFS) Lstat(p string) (xfer.Entry, error) {
	list, err := f.List(pathpkg.Dir(p))
	if err != nil {
		return xfer.Entry{}, err
	}
	for _, e := range list {
		if e.Name == pathpkg.Base(p) {
			return e, nil
		}
	}
	return xfer.Entry{}, fmt.Errorf("%s: %w", p, fs.ErrNotExist)
}

type ftpReader struct {
	r    *ftp.Response
	p    *ftpPool
	c    *ftp.ServerConn
	once sync.Once
}

func (r *ftpReader) Read(b []byte) (int, error) { return r.r.Read(b) }
func (r *ftpReader) Close() error {
	var err error
	r.once.Do(func() {
		err = r.r.Close()
		r.p.put(r.c, err != nil) // interrompido no meio: descarta a conexão
	})
	return err
}

func (f ftpFS) Open(p string, offset int64) (io.ReadCloser, error) {
	c, err := f.p.get()
	if err != nil {
		return nil, err
	}
	var r *ftp.Response
	if offset > 0 {
		r, err = c.RetrFrom(p, uint64(offset))
	} else {
		r, err = c.Retr(p)
	}
	if err != nil {
		f.p.put(c, isConnErr(err))
		return nil, err
	}
	return &ftpReader{r: r, p: f.p, c: c}, nil
}

type ftpWriter struct {
	pw   *io.PipeWriter
	done chan error
	p    *ftpPool
	c    *ftp.ServerConn
	once sync.Once
}

func (w *ftpWriter) Write(b []byte) (int, error) { return w.pw.Write(b) }
func (w *ftpWriter) Close() error {
	var err error
	w.once.Do(func() {
		w.pw.Close()
		err = <-w.done
		w.p.put(w.c, err != nil)
		w.p.invalidate()
	})
	return err
}

func (f ftpFS) Create(p string, offset int64) (io.WriteCloser, error) {
	c, err := f.p.get()
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		var err error
		if offset > 0 {
			err = c.StorFrom(p, pr, uint64(offset))
		} else {
			err = c.Stor(p, pr)
		}
		pr.CloseWithError(err)
		done <- err
	}()
	return &ftpWriter{pw: pw, done: done, p: f.p, c: c}, nil
}

func (f ftpFS) mutate(fn func(c *ftp.ServerConn) error) error {
	defer f.p.invalidate()
	return f.with(fn)
}

func (f ftpFS) Mkdir(p string) error {
	return f.mutate(func(c *ftp.ServerConn) error { return c.MakeDir(p) })
}
func (f ftpFS) Remove(p string) error {
	return f.mutate(func(c *ftp.ServerConn) error { return c.Delete(p) })
}
func (f ftpFS) RemoveDir(p string) error {
	return f.mutate(func(c *ftp.ServerConn) error { return c.RemoveDir(p) })
}
func (f ftpFS) Rename(from, to string) error {
	return f.mutate(func(c *ftp.ServerConn) error { return c.Rename(from, to) })
}
func (f ftpFS) Join(elem ...string) string { return pathpkg.Join(elem...) }

// Real resolve "", "." e caminhos relativos a partir da pasta inicial do login.
func (f ftpFS) Real(p string) (string, error) {
	home := f.p.home
	if home == "" {
		home = "/"
	}
	switch {
	case p == "" || p == ".":
		return home, nil
	case strings.HasPrefix(p, "/"):
		return pathpkg.Clean(p), nil
	}
	return pathpkg.Join(home, p), nil
}

// ------------------------------------------------------------------ conexão

// tcpAddrConn faz o RemoteAddr parecer um endereço TCP (a biblioteca FTP exige isso),
// mesmo quando a conexão passa por um proxy ou servidor de salto.
type tcpAddrConn struct{ net.Conn }

// o IP de verdade vem do nome do host; esse marcador é trocado na hora de abrir as conexões de dados
var ftpPlaceholderIP = net.IPv4(0, 0, 0, 1)

func (tcpAddrConn) RemoteAddr() net.Addr { return &net.TCPAddr{IP: ftpPlaceholderIP, Port: 21} }

// usesTunnel diz se a conexão da sessão passa por salto, ProxyCommand ou proxy.
func (a *TermService) usesTunnel(s store.Session) bool {
	if s.JumpHost != "" || s.ProxyCommand != "" {
		return true
	}
	px := s.Proxy
	if px == "" {
		px = a.globalProxy()
	}
	return px != "" && px != "none"
}

func (a *TermService) startFTP(s store.Session, secret string) (*termSession, error) {
	user, pass := s.User, secret
	if user == "" {
		user, pass = "anonymous", "anonymous@"
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	secure := s.FTPSecurity == "explicit" || s.FTPSecurity == "implicit"
	tlsCfg := &tls.Config{
		ServerName: s.Host, InsecureSkipVerify: s.FTPInsecure, MinVersion: tls.VersionTLS12,
		ClientSessionCache: tls.NewLRUClientSessionCache(8),
	}

	opts := []ftp.DialOption{ftp.DialWithTimeout(20 * time.Second)}
	if a.usesTunnel(s) {
		opts = append(opts, ftp.DialWithDialFunc(func(network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			control := address == addr
			if ip := net.ParseIP(host); ip != nil && ip.Equal(ftpPlaceholderIP) {
				address = net.JoinHostPort(s.Host, port)
			}
			conn, err := a.dialTCP(s, address, 0)
			if err != nil {
				return nil, err
			}
			if control {
				conn = tcpAddrConn{conn}
				if s.FTPSecurity == "implicit" {
					tc := tls.Client(conn, tlsCfg)
					if err := tc.Handshake(); err != nil {
						conn.Close()
						return nil, err
					}
					return tc, nil
				}
				return conn, nil
			}
			if secure {
				return tls.Client(conn, tlsCfg), nil // o handshake acontece no primeiro uso
			}
			return conn, nil
		}))
		switch s.FTPSecurity {
		case "explicit":
			opts = append(opts, ftp.DialWithExplicitTLS(tlsCfg))
		case "implicit":
			opts = append(opts, ftp.DialWithTLS(tlsCfg))
		}
	} else {
		switch s.FTPSecurity {
		case "explicit":
			opts = append(opts, ftp.DialWithExplicitTLS(tlsCfg))
		case "implicit":
			opts = append(opts, ftp.DialWithTLS(tlsCfg))
		}
	}

	dial := func() (*ftp.ServerConn, error) {
		c, err := ftp.Dial(addr, opts...)
		if err != nil {
			return nil, fmt.Errorf("falha ao conectar em %s: %w", addr, err)
		}
		if err := c.Login(user, pass); err != nil {
			go c.Quit()
			return nil, fmt.Errorf("login recusado: %w", err)
		}
		return c, nil
	}
	first, err := dial()
	if err != nil {
		return nil, err
	}
	pool := newFTPPool(dial)
	if home, err := first.CurrentDir(); err == nil {
		pool.home = home
	}
	pool.sem <- struct{}{} // o get/put abaixo devolve a vaga
	pool.put(first, false)

	t := &termSession{ftp: pool, enter: "\n"}
	t.closers = []func(){pool.close}
	t.wait = func() error {
		tk := time.NewTicker(30 * time.Second)
		defer tk.Stop()
		for {
			select {
			case <-pool.gone:
				return nil
			case <-tk.C:
				c, err := pool.get()
				if err != nil {
					return err
				}
				err = c.NoOp()
				pool.put(c, err != nil)
				if err != nil {
					return fmt.Errorf("conexão FTP perdida: %w", err)
				}
			}
		}
	}
	return t, nil
}
