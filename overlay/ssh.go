package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"nxaterm/internal/store"
	"nxaterm/internal/xfer"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ErrPassphraseRequired é devolvido quando a chave privada está protegida e
// nenhuma senha foi informada. O frontend usa o texto para pedir a senha.
const errPassphraseRequired = "PASSPHRASE_REQUIRED"

// termSession é uma aba conectada. Pode ser SSH (terminal), SFTP (só arquivos),
// Telnet, porta serial, shell local ou WSL; os campos opcionais dependem do tipo.
type termSession struct {
	id   string
	kind string // um dos store.Type*

	// SSH / SFTP
	client *ssh.Client
	shell  *ssh.Session

	// comum aos terminais
	w       io.Writer // entrada do terminal (nil em SFTP)
	out     io.Reader // saída do terminal (nil em SFTP)
	resize  func(cols, rows int) error
	wait    func() error // bloqueia até a sessão acabar; nil = até a leitura terminar
	closers []func()     // liberam os recursos específicos do tipo
	enter   string       // tecla Enter usada para o comando inicial

	readDone chan struct{}
	readErr  error

	closeOnce sync.Once
	done      chan struct{}

	sftpMu sync.Mutex
	sftp   *sftp.Client

	ftp *ftpPool // sessões FTP

	statsMu   sync.Mutex
	prevStats *rawStats
}

func (t *termSession) close() {
	t.closeOnce.Do(func() {
		close(t.done)
		t.sftpMu.Lock()
		if t.sftp != nil {
			t.sftp.Close()
		}
		t.sftpMu.Unlock()
		if t.shell != nil {
			t.shell.Close()
		}
		if t.client != nil {
			t.client.Close()
		}
		for _, f := range t.closers {
			f()
		}
	})
}

// sshClient devolve a conexão SSH da aba, ou um erro claro nos outros tipos.
func (t *termSession) sshClient() (*ssh.Client, error) {
	if t.client == nil {
		return nil, errors.New("esta operação só existe em conexões SSH/SFTP")
	}
	return t.client, nil
}

// sftpClient abre o canal SFTP sob demanda, reaproveitando a conexão SSH.
func (t *termSession) sftpClient() (*sftp.Client, error) {
	client, err := t.sshClient()
	if err != nil {
		return nil, err
	}
	t.sftpMu.Lock()
	defer t.sftpMu.Unlock()
	if t.sftp != nil {
		return t.sftp, nil
	}
	c, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("não foi possível abrir o SFTP: %w", err)
	}
	t.sftp = c
	return c, nil
}

// files devolve o sistema de arquivos remoto da aba (SFTP).
func (t *termSession) files() (xfer.FS, error) {
	if t.ftp != nil {
		return ftpFS{p: t.ftp}, nil
	}
	c, err := t.sftpClient()
	if err != nil {
		return nil, err
	}
	return sftpFS{c: c}, nil
}

// hostKeyCallback implementa "aceitar na primeira vez" (como o ssh com
// StrictHostKeyChecking=accept-new): hosts novos são gravados no known_hosts
// do app; se a chave de um host conhecido mudar, a conexão é recusada.
func (a *TermService) hostKeyCallback() ssh.HostKeyCallback {
	return func(host string, remote net.Addr, key ssh.PublicKey) error {
		a.khMu.Lock()
		defer a.khMu.Unlock()

		if _, err := os.Stat(a.knownHosts); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(a.knownHosts, nil, 0o600); err != nil {
				return err
			}
		}
		check, err := knownhosts.New(a.knownHosts)
		if err != nil {
			return err
		}
		err = check(host, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			if len(keyErr.Want) > 0 {
				return fmt.Errorf("A CHAVE DO HOST %s MUDOU (possível ataque man-in-the-middle). Conexão recusada. "+
					"Se a mudança é legítima, remova a linha correspondente de %s", host, a.knownHosts)
			}
			f, ferr := os.OpenFile(a.knownHosts, os.O_APPEND|os.O_WRONLY, 0o600)
			if ferr != nil {
				return ferr
			}
			defer f.Close()
			_, ferr = fmt.Fprintln(f, knownhosts.Line([]string{knownhosts.Normalize(host)}, key))
			return ferr
		}
		return err
	}
}

// startSSH conecta por SSH. Com withShell=false (sessão SFTP) só a conexão é
// aberta, sem terminal.
func (a *TermService) startSSH(s store.Session, secret string, cols, rows int, withShell bool) (*termSession, error) {
	client, err := a.connectSSH(s, secret, 0)
	if err != nil {
		return nil, err
	}
	t := &termSession{client: client, enter: "\n"}
	if !withShell {
		t.wait = client.Wait
		return t, nil
	}

	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, err
	}
	for _, kv := range store.EnvPairs(s.Env) {
		sess.Setenv(kv[0], kv[1]) // muitos servidores recusam; tudo bem
	}
	if s.X11 {
		setupX11(client, sess)
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := sess.RequestPty(termTypeOf(s), rows, cols, modes); err != nil {
		client.Close()
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		client.Close()
		return nil, err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		client.Close()
		return nil, err
	}
	if err := sess.Shell(); err != nil {
		client.Close()
		return nil, err
	}
	t.shell = sess
	t.w = stdin
	t.out = stdout
	t.resize = func(c, r int) error { return sess.WindowChange(r, c) }
	t.wait = sess.Wait
	return t, nil
}

// Connect abre uma aba com o ID escolhido pelo frontend. O ID vem de fora para
// o frontend registrar os listeners ANTES de conectar e não perder as primeiras
// linhas (o prompt) emitidas logo após o login. O tipo da conexão vem de s.Type
// (ssh, sftp, telnet, serial, shell ou wsl).
// Eventos emitidos para o frontend:
//
//	term:data:<id>   (string base64 com os bytes do terminal)
//	term:closed:<id> (string com o motivo, pode ser vazia)
func (a *TermService) Connect(id string, s store.Session, secret string, cols, rows int) error {
	if id == "" {
		return errors.New("ID de aba vazio")
	}
	a.termMu.Lock()
	_, dup := a.terms[id]
	a.termMu.Unlock()
	if dup {
		return fmt.Errorf("já existe uma aba com o ID %q", id)
	}

	s, err := store.Normalize(s)
	if err != nil {
		return err
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	var t *termSession
	switch s.Type {
	case store.TypeSSH:
		t, err = a.startSSH(s, secret, cols, rows, true)
	case store.TypeSFTP:
		t, err = a.startSSH(s, secret, cols, rows, false)
	case store.TypeFTP:
		t, err = a.startFTP(s, secret)
	case store.TypeTelnet:
		t, err = a.startTelnet(s, cols, rows)
	case store.TypeSerial:
		t, err = startSerial(s)
	case store.TypeShell:
		t, err = startShell(s, cols, rows)
	case store.TypeWSL:
		t, err = startWSL(s, cols, rows)
	default:
		err = fmt.Errorf("tipo de sessão sem suporte: %q", s.Type)
	}
	if err != nil {
		return err
	}
	t.id = id
	t.kind = s.Type
	t.done = make(chan struct{})
	t.readDone = make(chan struct{})

	a.termMu.Lock()
	a.terms[id] = t
	a.termMu.Unlock()

	if err := wrapEncoding(t, s.Encoding); err != nil {
		t.close()
		a.termMu.Lock()
		delete(a.terms, id)
		a.termMu.Unlock()
		return err
	}
	go runStartup(t, s)

	// saída do terminal -> frontend
	if t.out != nil {
		go func() {
			defer close(t.readDone)
			buf := make([]byte, 32*1024)
			for {
				n, err := t.out.Read(buf)
				if n > 0 {
					a.app.Event.Emit("term:data:"+id, base64.StdEncoding.EncodeToString(buf[:n]))
				}
				if err != nil {
					t.readErr = err
					return
				}
			}
		}()
	} else {
		close(t.readDone)
	}

	// espera a sessão terminar
	go func() {
		var werr error
		if t.wait != nil {
			werr = t.wait()
		} else {
			<-t.readDone
			werr = t.readErr
		}
		t.close()
		a.stopTunnelsOf(id)
		a.stopTransfersOf(id)
		a.termMu.Lock()
		delete(a.terms, id)
		a.termMu.Unlock()
		a.app.Event.Emit("term:closed:"+id, endReason(werr))
	}()

	// keepalive para a conexão SSH não cair por inatividade
	if t.client != nil {
		go func() {
			tk := time.NewTicker(30 * time.Second)
			defer tk.Stop()
			for {
				select {
				case <-t.done:
					return
				case <-tk.C:
					if _, _, err := t.client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
						return
					}
				}
			}
		}()
	}
	return nil
}

// endReason transforma o erro de fim de sessão no texto mostrado ao usuário
// (vazio quando foi um encerramento normal).
func endReason(err error) string {
	if err == nil {
		return ""
	}
	var exit *ssh.ExitError
	if errors.As(err, &exit) || errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, net.ErrClosed) {
		return ""
	}
	return err.Error()
}

// Write envia o que o usuário digitou (string UTF-8) para a sessão.
func (a *TermService) Write(id, data string) error {
	t, err := a.term(id)
	if err != nil {
		return err
	}
	if t.w == nil {
		return errors.New("esta aba não tem terminal")
	}
	_, err = io.WriteString(t.w, data)
	return err
}

// Resize avisa a sessão que a janela do terminal mudou de tamanho.
func (a *TermService) Resize(id string, cols, rows int) error {
	t, err := a.term(id)
	if err != nil {
		return err
	}
	if cols <= 0 || rows <= 0 || t.resize == nil {
		return nil
	}
	return t.resize(cols, rows)
}

// Close encerra uma aba.
func (a *TermService) Close(id string) {
	if t, err := a.term(id); err == nil {
		t.close()
	}
}

func (a *TermService) term(id string) (*termSession, error) {
	a.termMu.Lock()
	defer a.termMu.Unlock()
	t, ok := a.terms[id]
	if !ok {
		return nil, fmt.Errorf("terminal %q não está conectado", id)
	}
	return t, nil
}

// closeAll é chamado ao fechar a janela.
func (a *TermService) closeAll() {
	a.termMu.Lock()
	list := make([]*termSession, 0, len(a.terms))
	for _, t := range a.terms {
		list = append(list, t)
	}
	a.termMu.Unlock()
	for _, t := range list {
		t.close()
	}
	a.stopAllTunnels()
	a.stopAllTransfers()
}

// dataDir devolve (e cria) a pasta de dados do app.
func dataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "NxATerm")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// o app se chamava GoTerm: leva as sessões e configurações junto
		os.Rename(filepath.Join(base, "GoTerm"), dir)
	}
	return dir, os.MkdirAll(dir, 0o700)
}
