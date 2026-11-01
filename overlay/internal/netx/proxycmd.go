package netx

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ExpandProxyCommand troca %h (host), %p (porta) e %r (usuário) no comando.
func ExpandProxyCommand(cmdline, host string, port int, user string) string {
	r := strings.NewReplacer("%h", host, "%p", fmt.Sprint(port), "%r", user, "%%", "%")
	return r.Replace(cmdline)
}

// pipeConn faz um processo (stdin/stdout) se comportar como uma net.Conn.
type pipeConn struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    io.ReadCloser
	once   sync.Once
	closed chan struct{}
}

type pipeAddr string

func (pipeAddr) Network() string  { return "proxycommand" }
func (a pipeAddr) String() string { return string(a) }

// DialCommand executa o comando e usa a entrada/saída dele como conexão.
func DialCommand(cmdline string) (net.Conn, error) {
	cmdline = strings.TrimSpace(cmdline)
	if cmdline == "" {
		return nil, errors.New("ProxyCommand vazio")
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", cmdline)
	} else {
		cmd = exec.Command("sh", "-c", cmdline)
	}
	hideWindow(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("não consegui executar o ProxyCommand: %w", err)
	}
	c := &pipeConn{cmd: cmd, in: in, out: out, closed: make(chan struct{})}
	go func() { cmd.Wait(); c.Close() }()
	return c, nil
}

func (c *pipeConn) Read(p []byte) (int, error)  { return c.out.Read(p) }
func (c *pipeConn) Write(p []byte) (int, error) { return c.in.Write(p) }
func (c *pipeConn) Close() error {
	c.once.Do(func() {
		close(c.closed)
		c.in.Close()
		if c.cmd.Process != nil {
			c.cmd.Process.Kill()
		}
	})
	return nil
}
func (c *pipeConn) LocalAddr() net.Addr              { return pipeAddr("proxycommand") }
func (c *pipeConn) RemoteAddr() net.Addr             { return pipeAddr("proxycommand") }
func (c *pipeConn) SetDeadline(time.Time) error      { return nil }
func (c *pipeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *pipeConn) SetWriteDeadline(time.Time) error { return nil }
