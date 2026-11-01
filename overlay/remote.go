package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"nxaterm/internal/store"
	"nxaterm/internal/wsbridge"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// OpenRemote abre uma sessão que roda fora do terminal: RDP e Spice no cliente instalado no sistema,
// e Web numa janela própria do NxATerm. password só é usado pelo cliente RDP do Linux (por stdin, nunca em argumento).
func (a *TermService) OpenRemote(s store.Session, password string) error {
	s, err := store.Normalize(s)
	if err != nil {
		return err
	}
	switch s.Type {
	case store.TypeRDP:
		return openRDP(s, password)
	case store.TypeSpice:
		return openSpice(s)
	case store.TypeWeb:
		return a.openWeb(s)
	}
	return fmt.Errorf("tipo de sessão sem suporte aqui: %q", s.Type)
}

// VncOpen abre uma ponte WebSocket local (válida para UMA conexão) até o servidor VNC da sessão
// e devolve a URL que o noVNC deve usar. Respeita proxy e servidor de salto da sessão.
func (a *TermService) VncOpen(s store.Session) (string, error) {
	s, err := store.Normalize(s)
	if err != nil {
		return "", err
	}
	if s.Type != store.TypeVNC {
		return "", errors.New("a sessão não é VNC")
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	return wsbridge.Start(context.Background(), 60*time.Second, func(ctx context.Context) (net.Conn, error) {
		c, err := a.dialTCP(s, addr, 0)
		if err != nil {
			return nil, fmt.Errorf("falha ao conectar em %s: %w", addr, err)
		}
		return c, nil
	})
}

// ---------------------------------------------------------------- web

func (a *TermService) openWeb(s store.Session) error {
	if a.app == nil {
		return errors.New("o aplicativo não está pronto")
	}
	a.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  s.Name,
		Width:  1100,
		Height: 760,
		URL:    s.URL,
	})
	return nil
}

// ---------------------------------------------------------------- RDP

func openRDP(s store.Session, password string) error {
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	switch runtime.GOOS {
	case "windows":
		// o mstsc lê um arquivo .rdp; ele mesmo pede a senha
		content := "full address:s:" + addr + "\r\nprompt for credentials:i:1\r\n"
		if s.User != "" {
			content += "username:s:" + s.User + "\r\n"
		}
		f, err := os.CreateTemp("", "nxaterm-*.rdp")
		if err != nil {
			return err
		}
		path := f.Name()
		_, werr := f.WriteString(content)
		f.Close()
		if werr != nil {
			os.Remove(path)
			return werr
		}
		cmd := exec.Command("mstsc.exe", path)
		if err := cmd.Start(); err != nil {
			os.Remove(path)
			return fmt.Errorf("não foi possível abrir o mstsc: %w", err)
		}
		go func() { cmd.Wait() }()
		time.AfterFunc(2*time.Minute, func() { os.Remove(path) })
		return nil
	case "darwin":
		q := "full%20address=s:" + addr
		if s.User != "" {
			q += "&username=s:" + strings.ReplaceAll(s.User, " ", "%20")
		}
		return startDetached(exec.Command("open", "rdp://"+q), nil)
	}
	// Linux e outros: FreeRDP (ou rdesktop)
	for _, name := range []string{"xfreerdp3", "xfreerdp", "wlfreerdp3", "wlfreerdp"} {
		if p, err := exec.LookPath(name); err == nil {
			args := []string{"/v:" + addr, "/cert:tofu", "/dynamic-resolution", "+clipboard"}
			if s.User != "" {
				args = append(args, "/u:"+s.User)
			}
			if password == "" {
				return startDetached(exec.Command(p, args...), nil)
			}
			// a senha vai pela entrada padrão (um argumento por linha), não pela linha de comando
			cmd := exec.Command(p, "/args-from:stdin")
			return startDetached(cmd, []byte(strings.Join(append(args, "/p:"+password), "\n")+"\n"))
		}
	}
	if p, err := exec.LookPath("rdesktop"); err == nil {
		args := []string{addr}
		if s.User != "" {
			args = append(args, "-u", s.User)
		}
		return startDetached(exec.Command(p, args...), nil)
	}
	return errors.New("nenhum cliente RDP encontrado: instale o FreeRDP (xfreerdp)")
}

// ---------------------------------------------------------------- Spice

func openSpice(s store.Session) error {
	uri := "spice://" + net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	for _, name := range []string{"remote-viewer", "remote-viewer.exe"} {
		if p, err := exec.LookPath(name); err == nil {
			return startDetached(exec.Command(p, uri), nil)
		}
	}
	if runtime.GOOS == "windows" {
		for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
			if root == "" {
				continue
			}
			m, _ := filepath.Glob(filepath.Join(root, "VirtViewer*", "bin", "remote-viewer.exe"))
			if len(m) > 0 {
				return startDetached(exec.Command(m[len(m)-1], uri), nil)
			}
		}
	}
	if p, err := exec.LookPath("spicy"); err == nil {
		return startDetached(exec.Command(p, "-h", s.Host, "-p", strconv.Itoa(s.Port)), nil)
	}
	return errors.New("nenhum cliente Spice encontrado: instale o virt-viewer (remote-viewer)")
}

// startDetached inicia o programa sem esperar por ele. stdin, se houver, é entregue e fechado.
func startDetached(cmd *exec.Cmd, stdin []byte) error {
	var w interface {
		Write([]byte) (int, error)
		Close() error
	}
	if stdin != nil {
		p, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		w = p
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("não foi possível abrir %s: %w", filepath.Base(cmd.Path), err)
	}
	if w != nil {
		w.Write(stdin)
		w.Close()
	}
	go func() { cmd.Wait() }()
	return nil
}
