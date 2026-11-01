package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"nxaterm/internal/netx"
	"nxaterm/internal/secfile"
	"nxaterm/internal/store"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

const defaultTerm = "xterm-256color"

func termTypeOf(s store.Session) string {
	if s.TermType != "" {
		return s.TermType
	}
	return defaultTerm
}

// ------------------------------------------------------------------ perguntas ao usuário

// promptReq é uma pergunta que o servidor ou o proxy fez durante a conexão.
type promptReq struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Label  string `json:"label"`
	Key    string `json:"key"` // chave da senha memorizada (vazio = não memorizável)
	Secret bool   `json:"secret"`
}

type promptAnswer struct {
	value string
	ok    bool
}

// ask mostra uma pergunta no app e espera a resposta (senha de um servidor de
// salto, código de verificação em dois fatores, senha do proxy...).
func (a *TermService) ask(title, label, key string, secret bool) (string, error) {
	b := make([]byte, 8)
	rand.Read(b)
	id := hex.EncodeToString(b)
	ch := make(chan promptAnswer, 1)
	a.promptMu.Lock()
	a.prompts[id] = ch
	a.promptMu.Unlock()
	defer func() {
		a.promptMu.Lock()
		delete(a.prompts, id)
		a.promptMu.Unlock()
	}()

	js, _ := json.Marshal(promptReq{ID: id, Title: title, Label: label, Key: key, Secret: secret})
	a.app.Event.Emit("ask:request", string(js))
	select {
	case r := <-ch:
		if !r.ok {
			return "", errors.New("conexão cancelada")
		}
		return r.value, nil
	case <-time.After(5 * time.Minute):
		return "", errors.New("tempo esgotado esperando a resposta")
	}
}

// AnswerPrompt entrega ao backend a resposta de uma pergunta (ok=false = cancelou).
func (a *TermService) AnswerPrompt(id, value string, ok bool) {
	a.promptMu.Lock()
	ch := a.prompts[id]
	a.promptMu.Unlock()
	if ch != nil {
		select {
		case ch <- promptAnswer{value, ok}:
		default:
		}
	}
}

// ------------------------------------------------------------------ rede

// linkedConn fecha, junto com a conexão, o que a sustenta (ex.: o servidor de salto).
type linkedConn struct {
	net.Conn
	after []func()
}

func (c *linkedConn) Close() error {
	err := c.Conn.Close()
	for _, f := range c.after {
		f()
	}
	return err
}

// globalProxy lê o proxy geral das configurações do app.
func (a *TermService) globalProxy() string {
	data, err := secfile.Read(a.settingsPath)
	if err != nil {
		return ""
	}
	var v struct {
		Proxy string `json:"proxy"`
	}
	json.Unmarshal(data, &v)
	return strings.TrimSpace(v.Proxy)
}

func (a *TermService) resolveProxy(s store.Session) (*netx.Proxy, error) {
	raw := s.Proxy
	if raw == "none" {
		return nil, nil
	}
	if raw == "" {
		raw = a.globalProxy()
	}
	px, err := netx.ParseProxy(raw)
	if err != nil || px == nil {
		return px, err
	}
	if px.NeedsPassword() {
		pw, err := a.ask("Proxy", "Senha do proxy "+px.String(), "px:"+px.String(), true)
		if err != nil {
			return nil, err
		}
		px.Password = pw
	}
	return px, nil
}

// dialTCP abre a conexão com addr do jeito que a sessão pede: servidor de
// salto, ProxyCommand, proxy ou direta.
func (a *TermService) dialTCP(s store.Session, addr string, depth int) (net.Conn, error) {
	if s.JumpHost != "" {
		if depth >= 4 {
			return nil, errors.New("cadeia de servidores de salto longa demais")
		}
		j, ok := a.store.Get(s.JumpHost)
		if !ok {
			return nil, errors.New("o servidor de salto escolhido não existe mais")
		}
		if j.Type != store.TypeSSH && j.Type != store.TypeSFTP {
			return nil, errors.New("o servidor de salto precisa ser uma sessão SSH")
		}
		secret, err := a.secretFor(j)
		if err != nil {
			return nil, err
		}
		jc, err := a.connectSSH(j, secret, depth+1)
		if err != nil {
			return nil, fmt.Errorf("servidor de salto %q: %w", j.Name, err)
		}
		c, err := jc.Dial("tcp", addr)
		if err != nil {
			jc.Close()
			return nil, fmt.Errorf("o servidor de salto não chegou em %s: %w", addr, err)
		}
		return &linkedConn{Conn: c, after: []func(){func() { jc.Close() }}}, nil
	}
	if s.ProxyCommand != "" {
		return netx.DialCommand(netx.ExpandProxyCommand(s.ProxyCommand, s.Host, s.Port, s.User))
	}
	px, err := a.resolveProxy(s)
	if err != nil {
		return nil, err
	}
	if px != nil {
		return px.Dial(context.Background(), addr, 15*time.Second)
	}
	return net.DialTimeout("tcp", addr, 15*time.Second)
}

// secretFor obtém a senha (ou a senha da chave) de uma sessão usada como salto.
func (a *TermService) secretFor(j store.Session) (string, error) {
	who := fmt.Sprintf("%s@%s:%d", j.User, j.Host, j.Port)
	switch j.AuthMethod {
	case "key":
		_, closeAuth, err := a.authMethods(j, "")
		if closeAuth != nil {
			closeAuth()
		}
		if err != nil && err.Error() == errPassphraseRequired {
			return a.ask("Chave protegida", "Senha da chave privada de "+who, "pp:"+who, true)
		}
		return "", nil
	case "agent":
		return "", nil
	default:
		return a.ask("Senha", "Senha de "+who, "pw:"+who, true)
	}
}

// ------------------------------------------------------------------ SSH

func looksLikePassword(q string) bool {
	q = strings.ToLower(q)
	for _, w := range []string{"password", "senha", "contraseña", "mot de passe", "passwort"} {
		if strings.Contains(q, w) {
			return true
		}
	}
	return false
}

// kbdInteractive responde ao "keyboard-interactive": a senha vai para a pergunta
// de senha; qualquer outra (código do autenticador, PIN...) é perguntada ao usuário.
func (a *TermService) kbdInteractive(secret string) ssh.AuthMethod {
	used := false
	return ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		out := make([]string, len(questions))
		for i, q := range questions {
			if secret != "" && !used && looksLikePassword(q) {
				out[i] = secret
				used = true
				continue
			}
			label := strings.TrimSpace(strings.TrimSpace(instruction) + "\n" + strings.TrimSpace(q))
			secretInput := i >= len(echos) || !echos[i]
			v, err := a.ask("Verificação em duas etapas", label, "", secretInput)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	})
}

// authMethods monta os métodos de autenticação de acordo com o perfil. secret é a
// senha (AuthMethod "password") ou a senha da chave (AuthMethod "key"). O retorno
// func() libera a conexão com o ssh-agent, quando houve.
func (a *TermService) authMethods(s store.Session, secret string) ([]ssh.AuthMethod, func(), error) {
	switch s.AuthMethod {
	case "key":
		pem, err := os.ReadFile(s.KeyPath)
		if err != nil {
			return nil, nil, fmt.Errorf("não consegui ler a chave %s: %w", s.KeyPath, err)
		}
		var signer ssh.Signer
		if secret == "" {
			signer, err = ssh.ParsePrivateKey(pem)
		} else {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(pem, []byte(secret))
		}
		if err != nil {
			var missing *ssh.PassphraseMissingError
			if errors.As(err, &missing) {
				return nil, nil, errors.New(errPassphraseRequired)
			}
			return nil, nil, fmt.Errorf("chave inválida ou senha da chave incorreta: %w", err)
		}
		// certificado OpenSSH ao lado da chave (arquivo-cert.pub)
		if certPEM, err := os.ReadFile(s.KeyPath + "-cert.pub"); err == nil {
			if pub, _, _, _, err := ssh.ParseAuthorizedKey(certPEM); err == nil {
				if cert, ok := pub.(*ssh.Certificate); ok {
					if cs, err := ssh.NewCertSigner(cert, signer); err == nil {
						signer = cs
					}
				}
			}
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer), a.kbdInteractive("")}, nil, nil

	case "agent":
		conn, err := dialAgent()
		if err != nil {
			return nil, nil, err
		}
		ac := agent.NewClient(conn)
		return []ssh.AuthMethod{ssh.PublicKeysCallback(ac.Signers), a.kbdInteractive("")}, func() { conn.Close() }, nil

	default: // password
		return []ssh.AuthMethod{ssh.Password(secret), a.kbdInteractive(secret)}, nil, nil
	}
}

// connectSSH conecta e autentica, passando por salto/proxy conforme a sessão.
func (a *TermService) connectSSH(s store.Session, secret string, depth int) (*ssh.Client, error) {
	methods, closeAuth, err := a.authMethods(s, secret)
	if err != nil {
		return nil, err
	}
	if closeAuth != nil {
		defer closeAuth()
	}
	cfg := &ssh.ClientConfig{
		User:            s.User,
		Auth:            methods,
		HostKeyCallback: a.hostKeyCallback(),
		Timeout:         15 * time.Second,
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	conn, err := a.dialTCP(s, addr, depth)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar em %s: %w", addr, err)
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("falha ao conectar em %s: %w", addr, err)
	}
	return ssh.NewClient(c, chans, reqs), nil
}

// ------------------------------------------------------------------ X11

// setupX11 pede ao servidor o encaminhamento do X11 e liga cada janela dele ao
// servidor X local (DISPLAY). O cookie combinado com o servidor é falso: o
// verdadeiro só é colocado na conexão local.
func setupX11(client *ssh.Client, sess *ssh.Session) {
	disp, err := netx.ParseDisplay(os.Getenv("DISPLAY"))
	if err != nil {
		return
	}
	fake := make([]byte, 16)
	rand.Read(fake)
	realName, realData := netx.XAuthority("", disp.Num)

	chans := client.HandleChannelOpen("x11")
	if chans == nil {
		return
	}
	go func() {
		for nc := range chans {
			go func(nc ssh.NewChannel) {
				local, err := net.DialTimeout(disp.Network, disp.Addr, 5*time.Second)
				if err != nil {
					nc.Reject(ssh.ConnectionFailed, "sem servidor X local")
					return
				}
				ch, reqs, err := nc.Accept()
				if err != nil {
					local.Close()
					return
				}
				go ssh.DiscardRequests(reqs)
				prefix, err := netx.RewriteSetup(ch, fake, realName, realData)
				if err == nil {
					_, err = local.Write(prefix)
				}
				if err != nil {
					ch.Close()
					local.Close()
					return
				}
				pipe(local, ch)
			}(nc)
		}
	}()

	payload := ssh.Marshal(struct {
		Single   bool
		Protocol string
		Cookie   string
		Screen   uint32
	}{false, "MIT-MAGIC-COOKIE-1", hex.EncodeToString(fake), 0})
	sess.SendRequest("x11-req", true, payload)
}

// ------------------------------------------------------------------ codificação

// wrapEncoding prepara a codificação escolhida (GBK, Shift-JIS, Latin-1...). O que o usuário digita é
// convertido aqui; a saída do servidor segue em bytes crus e a interface a decodifica (TextDecoder),
// assim o Zmodem recebe os dados sem nenhuma alteração.
func wrapEncoding(t *termSession, name string) error {
	if name == "" || t.out == nil {
		return nil
	}
	enc, err := htmlindex.Get(name)
	if err != nil {
		return fmt.Errorf("codificação não suportada: %q", name)
	}
	if t.w != nil {
		if t.wRaw == nil {
			t.wRaw = t.w
		}
		t.w = &lockedWriter{w: transform.NewWriter(t.w, enc.NewEncoder())}
	}
	return nil
}

// lockedWriter serializa as escritas (o conversor de codificação guarda estado interno).
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// CheckEncoding diz se o nome da codificação é conhecido (para a janela de sessão).
func (a *TermService) CheckEncoding(name string) bool {
	if strings.TrimSpace(name) == "" {
		return true
	}
	_, err := htmlindex.Get(strings.TrimSpace(name))
	return err == nil
}

// ------------------------------------------------------------------ scripts de início

// runStartup envia a pasta inicial e os comandos de "ao conectar". Em cada linha,
// "@sleep 500" espera 500 ms antes da próxima.
func runStartup(t *termSession, s store.Session) {
	if t.w == nil {
		return
	}
	var lines []string
	if s.StartDir != "" && s.Type != store.TypeShell {
		lines = append(lines, "cd -- "+shellQuote(s.StartDir))
	}
	for _, l := range strings.Split(strings.ReplaceAll(s.StartupCommand, "\r", ""), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	for _, l := range lines {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(l), "@sleep "); ok {
			if ms, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil && ms > 0 && ms <= 60000 {
				select {
				case <-time.After(time.Duration(ms) * time.Millisecond):
				case <-t.done:
					return
				}
			}
			continue
		}
		if _, err := io.WriteString(t.w, l+t.enter); err != nil {
			return
		}
		select {
		case <-time.After(80 * time.Millisecond):
		case <-t.done:
			return
		}
	}
}

func shellQuote(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return "\"$HOME\"" + shellQuoteRest(strings.TrimPrefix(p, "~"))
	}
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}

func shellQuoteRest(p string) string {
	if p == "" {
		return ""
	}
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
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
