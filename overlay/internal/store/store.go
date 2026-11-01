// Package store guarda os perfis de sessão SSH em um arquivo JSON.
// Não há limite de quantidade de sessões. Senhas NUNCA são gravadas aqui.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"nxaterm/internal/secfile"
	"nxaterm/internal/tomb"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Tipos de sessão.
const (
	TypeSSH    = "ssh"
	TypeSFTP   = "sftp"
	TypeTelnet = "telnet"
	TypeSerial = "serial"
	TypeShell  = "shell"
	TypeWSL    = "wsl"
	TypeFTP    = "ftp"
	TypeRDP    = "rdp"   // área de trabalho remota (cliente do sistema)
	TypeVNC    = "vnc"   // VNC dentro do app
	TypeSpice  = "spice" // Spice (cliente do sistema)
	TypeWeb    = "web"   // página da web em janela própria
)

// Session é um perfil de conexão (SSH, SFTP, Telnet, serial, shell local ou WSL).
type Session struct {
	ID             string `json:"id"`
	Type           string `json:"type"` // um dos Type* acima; vazio = ssh (perfis antigos)
	Name           string `json:"name"`
	URL            string `json:"url,omitempty"` // tipo web: endereço http(s)://
	Folder         string `json:"folder"`        // pasta para organizar na barra lateral, ex: "Produção/Web"
	Host           string `json:"host"`
	Port           int    `json:"port"`
	User           string `json:"user"`
	AuthMethod     string `json:"authMethod"`     // "password" ou "key" (ssh e sftp)
	KeyPath        string `json:"keyPath"`        // caminho da chave privada quando AuthMethod == "key"
	StartupCommand string `json:"startupCommand"` // comando executado logo após conectar
	Notes          string `json:"notes"`

	// Organização
	Color string   `json:"color,omitempty"` // #rrggbb: cor da sessão na lista e nas abas
	Tags  []string `json:"tags,omitempty"`

	// Serial
	SerialPort string `json:"serialPort"` // ex: COM3 ou /dev/ttyUSB0
	BaudRate   int    `json:"baudRate"`
	DataBits   int    `json:"dataBits"` // 5 a 8
	Parity     string `json:"parity"`   // none, odd, even, mark, space
	StopBits   string `json:"stopBits"` // 1, 1.5, 2

	// Shell local e WSL
	Shell  string `json:"shell"`  // programa do shell local (vazio = padrão do sistema)
	Distro string `json:"distro"` // distribuição do WSL (vazio = a padrão)

	// Conexão (SSH, SFTP e Telnet)
	Proxy        string `json:"proxy,omitempty"`        // vazio = o proxy global; "none" = direto; ou socks5://host:porta, http://host:porta
	ProxyCommand string `json:"proxyCommand,omitempty"` // comando cuja entrada/saída vira a conexão (%h %p %r)
	JumpHost     string `json:"jumpHost,omitempty"`     // ID de outra sessão SSH usada como servidor de salto

	// Terminal (todos os tipos com terminal)
	Encoding string `json:"encoding,omitempty"` // vazio = UTF-8
	TermType string `json:"termType,omitempty"` // vazio = xterm-256color
	StartDir string `json:"startDir,omitempty"` // pasta inicial
	Env      string `json:"env,omitempty"`      // variáveis, uma NOME=valor por linha
	X11      bool   `json:"x11,omitempty"`      // encaminhar o X11 (SSH)

	// FTP
	FTPSecurity string `json:"ftpSecurity,omitempty"` // vazio = sem criptografia; "explicit" (AUTH TLS) ou "implicit" (porta 990)
	FTPInsecure bool   `json:"ftpInsecure,omitempty"` // aceita certificado não verificado
}

// Store mantém a lista de sessões em memória e no disco.
type Store struct {
	mu          sync.Mutex
	path        string
	sessions    []Session
	foldersPath string   // pastas criadas pelo usuário (inclusive vazias)
	folders     []string // sem duplicatas, já normalizadas
	locked      bool     // dados criptografados e ainda sem a senha de abertura
	tombs       *tomb.Log
}

// SetTombs liga o registro de exclusões (usado pela sincronização).
func (s *Store) SetTombs(t *tomb.Log) { s.tombs = t }

// Open carrega o arquivo (criando-o vazio se não existir). Se os dados estiverem criptografados e
// a senha de abertura ainda não foi digitada, devolve o Store vazio e bloqueado: Reload carrega
// depois do desbloqueio, e enquanto isso nada é gravado.
func Open(path string) (*Store, error) {
	s := &Store{path: path, foldersPath: filepath.Join(filepath.Dir(path), "folders.json")}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s, nil
}

// Locked diz se os dados ainda estão protegidos (aguardando a senha de abertura).
func (s *Store) Locked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.locked
}

// Reload lê de novo os arquivos (usado depois de desbloquear).
func (s *Store) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.locked = false
	s.sessions = []Session{}
	s.folders = []string{}
	if err := s.loadFolders(); err != nil {
		if errors.Is(err, secfile.ErrLocked) {
			s.locked = true
			return nil
		}
		return err
	}
	data, err := secfile.Read(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case errors.Is(err, secfile.ErrLocked):
		s.locked = true
		s.folders = []string{}
		return nil
	case err != nil:
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &s.sessions); err != nil {
		return fmt.Errorf("arquivo de sessões corrompido (%s): %w", s.path, err)
	}
	return nil
}

// Rewrite grava de novo todos os arquivos, no formato atual (criptografado ou não).
func (s *Store) Rewrite() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.flush(); err != nil {
		return err
	}
	return s.flushFolders()
}

// List devolve uma cópia ordenada por pasta e nome.
func (s *Store) List() []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Session, len(s.sessions))
	copy(out, s.sessions)
	sort.SliceStable(out, func(i, j int) bool {
		fi, fj := strings.ToLower(out[i].Folder), strings.ToLower(out[j].Folder)
		if fi != fj {
			return fi < fj
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Get devolve a sessão com o ID informado.
func (s *Store) Get(id string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.sessions {
		if x.ID == id {
			return x, true
		}
	}
	return Session{}, false
}

// Normalize valida e preenche os valores padrão de uma sessão.
func Normalize(in Session) (Session, error) {
	in.Host = strings.TrimSpace(in.Host)
	in.User = strings.TrimSpace(in.User)
	in.Name = strings.TrimSpace(in.Name)
	in.Folder = CleanFolder(in.Folder)
	in.Type = strings.ToLower(strings.TrimSpace(in.Type))
	if in.Type == "" {
		in.Type = TypeSSH
	}

	switch in.Type {
	case TypeSSH, TypeSFTP:
		if err := normalizeRemote(&in, 22, true); err != nil {
			return in, err
		}
		if in.Name == "" {
			in.Name = in.User + "@" + in.Host
		}
		switch in.AuthMethod {
		case "password", "key", "agent":
		case "":
			in.AuthMethod = "password"
		default:
			return in, fmt.Errorf("método de autenticação inválido: %q", in.AuthMethod)
		}
		if in.AuthMethod == "key" && strings.TrimSpace(in.KeyPath) == "" {
			return in, errors.New("informe o caminho da chave privada")
		}
		if in.JumpHost == in.ID && in.ID != "" {
			return in, errors.New("o servidor de salto não pode ser a própria sessão")
		}

	case TypeFTP:
		switch in.FTPSecurity {
		case "", "explicit", "implicit":
		default:
			return in, fmt.Errorf("segurança de FTP inválida: %q", in.FTPSecurity)
		}
		def := 21
		if in.FTPSecurity == "implicit" {
			def = 990
		}
		if err := normalizeRemote(&in, def, false); err != nil {
			return in, err
		}
		in.AuthMethod, in.KeyPath = "password", ""
		in.X11 = false
		in.Encoding, in.TermType, in.StartDir, in.Env, in.StartupCommand = "", "", "", "", ""
		if in.Name == "" {
			if in.User != "" {
				in.Name = in.User + "@" + in.Host
			} else {
				in.Name = in.Host
			}
		}

	case TypeTelnet:
		if err := normalizeRemote(&in, 23, false); err != nil {
			return in, err
		}
		in.User, in.AuthMethod, in.KeyPath = "", "password", ""
		in.X11 = false
		if in.Name == "" {
			in.Name = in.Host
		}

	case TypeSerial:
		in.SerialPort = strings.TrimSpace(in.SerialPort)
		if in.SerialPort == "" {
			return in, errors.New("escolha a porta serial (ex: COM3)")
		}
		if in.BaudRate == 0 {
			in.BaudRate = 9600
		}
		if in.BaudRate < 50 || in.BaudRate > 4000000 {
			return in, fmt.Errorf("velocidade (baud rate) inválida: %d", in.BaudRate)
		}
		if in.DataBits == 0 {
			in.DataBits = 8
		}
		if in.DataBits < 5 || in.DataBits > 8 {
			return in, fmt.Errorf("bits de dados inválido: %d (use 5 a 8)", in.DataBits)
		}
		switch in.Parity {
		case "":
			in.Parity = "none"
		case "none", "odd", "even", "mark", "space":
		default:
			return in, fmt.Errorf("paridade inválida: %q", in.Parity)
		}
		switch in.StopBits {
		case "":
			in.StopBits = "1"
		case "1", "1.5", "2":
		default:
			return in, fmt.Errorf("bits de parada inválido: %q", in.StopBits)
		}
		in.Host, in.User, in.Port = "", "", 0
		in.clearConnection()
		in.TermType, in.StartDir, in.Env = "", "", ""
		if in.Name == "" {
			in.Name = fmt.Sprintf("%s (%d)", in.SerialPort, in.BaudRate)
		}

	case TypeShell:
		in.Shell = strings.TrimSpace(in.Shell)
		in.Host, in.User, in.Port = "", "", 0
		in.clearConnection()
		if in.Name == "" {
			in.Name = "Shell local"
		}

	case TypeWSL:
		in.Distro = strings.TrimSpace(in.Distro)
		in.Host, in.User, in.Port = "", "", 0
		in.clearConnection()
		if in.Name == "" {
			in.Name = "WSL"
			if in.Distro != "" {
				in.Name = "WSL: " + in.Distro
			}
		}

	case TypeRDP, TypeVNC, TypeSpice:
		def := map[string]int{TypeRDP: 3389, TypeVNC: 5900, TypeSpice: 5900}[in.Type]
		if err := normalizeRemote(&in, def, false); err != nil {
			return in, err
		}
		if in.Type != TypeRDP {
			in.User = ""
		}
		if in.Type == TypeVNC { // o VNC passa pelo app: aceita proxy e servidor de salto
			in.AuthMethod, in.KeyPath, in.X11 = "password", "", false
		} else {
			in.clearConnection()
		}
		in.Encoding, in.TermType, in.StartDir, in.Env, in.StartupCommand = "", "", "", "", ""
		if in.Name == "" {
			if in.User != "" {
				in.Name = in.User + "@" + in.Host
			} else {
				in.Name = in.Host
			}
		}

	case TypeWeb:
		u, err := CleanWebURL(in.URL)
		if err != nil {
			return in, err
		}
		in.URL = u
		in.Host, in.User, in.Port = "", "", 0
		in.clearConnection()
		in.Encoding, in.TermType, in.StartDir, in.Env, in.StartupCommand = "", "", "", "", ""
		if in.Name == "" {
			in.Name = webHost(u)
		}

	default:
		return in, fmt.Errorf("tipo de sessão inválido: %q", in.Type)
	}

	if in.Type != TypeFTP {
		in.FTPSecurity, in.FTPInsecure = "", false
	}
	in.Proxy = strings.TrimSpace(in.Proxy)
	in.ProxyCommand = strings.TrimSpace(in.ProxyCommand)
	in.Encoding = strings.ToLower(strings.TrimSpace(in.Encoding))
	if in.Encoding == "utf-8" || in.Encoding == "utf8" {
		in.Encoding = ""
	}
	in.TermType = strings.TrimSpace(in.TermType)
	if in.TermType != "" && !termTypeRe.MatchString(in.TermType) {
		return in, fmt.Errorf("tipo de terminal inválido: %q", in.TermType)
	}
	in.StartDir = strings.TrimSpace(in.StartDir)
	env, err := CleanEnv(in.Env)
	if err != nil {
		return in, err
	}
	in.Env = env
	if in.Type != TypeSSH {
		in.X11 = false
	}
	in.Color = CleanColor(in.Color)
	in.Tags = CleanTags(in.Tags)
	return in, nil
}

var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// CleanColor devolve a cor em minúsculas ou "" se não for #rrggbb.
func CleanColor(c string) string {
	c = strings.TrimSpace(c)
	if !colorRe.MatchString(c) {
		return ""
	}
	return strings.ToLower(c)
}

// CleanTags tira espaços, repetidas (sem diferenciar maiúsculas) e vazias; no máximo 20 de 32 caracteres.
func CleanTags(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(t), "#"), ",", " "))
		if t == "" {
			continue
		}
		if r := []rune(t); len(r) > 32 {
			t = string(r[:32])
		}
		k := strings.ToLower(t)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, t)
		if len(out) == 20 {
			break
		}
	}
	return out
}

var termTypeRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)
var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// clearConnection limpa o que só faz sentido para conexões de rede.
func (s *Session) clearConnection() {
	s.Proxy, s.ProxyCommand, s.JumpHost, s.X11 = "", "", "", false
	s.AuthMethod, s.KeyPath = "password", ""
}

// CleanEnv confere as linhas NOME=valor e devolve o texto normalizado.
func CleanEnv(text string) (string, error) {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, _, ok := strings.Cut(line, "=")
		if !ok || !envNameRe.MatchString(strings.TrimSpace(k)) {
			return "", fmt.Errorf("variável inválida: %q (use NOME=valor)", line)
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n"), nil
}

// EnvPairs devolve as variáveis já separadas.
func EnvPairs(text string) [][2]string {
	var out [][2]string
	for _, line := range strings.Split(text, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && envNameRe.MatchString(k) {
			out = append(out, [2]string{k, v})
		}
	}
	return out
}

// normalizeRemote valida host, porta e (opcionalmente) usuário das sessões de rede.
func normalizeRemote(in *Session, defPort int, needUser bool) error {
	if in.Host == "" {
		return errors.New("o host é obrigatório")
	}
	if needUser && in.User == "" {
		return errors.New("o usuário é obrigatório")
	}
	if in.Port == 0 {
		in.Port = defPort
	}
	if in.Port < 1 || in.Port > 65535 {
		return fmt.Errorf("porta inválida: %d", in.Port)
	}
	return nil
}

// Save cria (ID vazio) ou atualiza (ID existente) uma sessão.
func (s *Store) Save(in Session) (Session, error) {
	in, err := Normalize(in)
	if err != nil {
		return in, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if in.ID == "" {
		id, err := newID()
		if err != nil {
			return in, err
		}
		in.ID = id
		s.sessions = append(s.sessions, in)
	} else {
		found := false
		for i := range s.sessions {
			if s.sessions[i].ID == in.ID {
				s.sessions[i] = in
				found = true
				break
			}
		}
		if !found {
			return in, fmt.Errorf("sessão %q não encontrada", in.ID)
		}
	}
	return in, s.flush()
}

// Delete remove uma sessão pelo ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sessions {
		if s.sessions[i].ID == id {
			s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
			if err := s.flush(); err != nil {
				return err
			}
			return s.tombs.Add(tomb.Session, id)
		}
	}
	return fmt.Errorf("sessão %q não encontrada", id)
}

// Import adiciona sessões novas, ignorando as que já existem com o mesmo
// usuário@host:porta. Devolve quantas foram adicionadas.
func (s *Store) Import(list []Session) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	exists := map[string]bool{}
	for _, e := range s.sessions {
		exists[key(e)] = true
	}
	added := 0
	for _, in := range list {
		n, err := Normalize(in)
		if err != nil || exists[key(n)] {
			continue
		}
		id, err := newID()
		if err != nil {
			return added, err
		}
		n.ID = id
		s.sessions = append(s.sessions, n)
		exists[key(n)] = true
		added++
	}
	if added == 0 {
		return 0, nil
	}
	return added, s.flush()
}

// key identifica "o mesmo destino" para não importar duplicatas.
func key(s Session) string {
	t := s.Type
	if t == "" {
		t = TypeSSH
	}
	switch t {
	case TypeSerial:
		return fmt.Sprintf("serial|%s|%d", strings.ToLower(s.SerialPort), s.BaudRate)
	case TypeShell:
		return "shell|" + strings.ToLower(s.Shell)
	case TypeWSL:
		return "wsl|" + strings.ToLower(s.Distro)
	case TypeWeb:
		return "web|" + strings.ToLower(s.URL)
	}
	return fmt.Sprintf("%s|%s@%s:%d", t, strings.ToLower(s.User), strings.ToLower(s.Host), s.Port)
}

func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// flush grava de forma atômica (arquivo temporário + rename), criptografando se houver senha de abertura.
func (s *Store) flush() error {
	if s.locked {
		return secfile.ErrLocked
	}
	data, err := json.MarshalIndent(s.sessions, "", "  ")
	if err != nil {
		return err
	}
	return secfile.Write(s.path, data)
}

// MergeSync junta sessões vindas de outro computador, mantendo os IDs (os servidores de salto
// apontam para IDs). Sessões que já existem (mesmo ID ou mesmo destino) ficam como estão aqui.
func (s *Store) MergeSync(list []Session, folders []string) (added int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := map[string]bool{}
	keys := map[string]bool{}
	for _, e := range s.sessions {
		ids[e.ID] = true
		keys[key(e)] = true
	}
	for _, in := range list {
		n, nerr := Normalize(in)
		if nerr != nil || (n.ID != "" && ids[n.ID]) || keys[key(n)] || (n.ID != "" && s.tombs.Has(tomb.Session, n.ID)) {
			continue
		}
		if n.ID == "" || ids[n.ID] {
			if n.ID, err = newID(); err != nil {
				return added, err
			}
		}
		s.sessions = append(s.sessions, n)
		ids[n.ID], keys[key(n)] = true, true
		added++
	}
	foldersChanged := false
	for _, f := range folders {
		if s.tombs.Has(tomb.Folder, CleanFolder(f)) {
			continue
		}
		if s.addFolderLocked(f) {
			foldersChanged = true
		}
	}
	if added > 0 {
		if err = s.flush(); err != nil {
			return added, err
		}
	}
	if foldersChanged {
		err = s.flushFolders()
	}
	return added, err
}

// ReplaceAll troca tudo pelo que veio de outro computador (IDs preservados). Sessões inválidas são ignoradas.
func (s *Store) ReplaceAll(list []Session, folders []string) (kept int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Session
	ids := map[string]bool{}
	for _, in := range list {
		n, nerr := Normalize(in)
		if nerr != nil {
			continue
		}
		if n.ID == "" || ids[n.ID] {
			if n.ID, err = newID(); err != nil {
				return 0, err
			}
		}
		ids[n.ID] = true
		out = append(out, n)
	}
	if out == nil {
		out = []Session{}
	}
	s.sessions = out
	s.folders = nil
	for _, f := range folders {
		s.addFolderLocked(f)
	}
	if err = s.flush(); err != nil {
		return len(out), err
	}
	// o que veio da cópia existe de novo: não pode continuar marcado como apagado
	s.tombs.Unmark(tomb.Session, ids2slice(ids))
	s.tombs.Unmark(tomb.Folder, s.folders)
	return len(out), s.flushFolders()
}

func ids2slice(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ApplyTombstones apaga aqui o que foi apagado em outro computador (registro já unido ao local).
// Pastas só saem se estiverem vazias. Devolve quantas sessões e pastas foram removidas.
func (s *Store) ApplyTombstones() (sessions, folders int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return 0, 0, secfile.ErrLocked
	}
	kept := s.sessions[:0:0]
	for _, x := range s.sessions {
		if s.tombs.Has(tomb.Session, x.ID) {
			sessions++
			continue
		}
		kept = append(kept, x)
	}
	s.sessions = kept
	for {
		removed := false
		for _, f := range s.folders {
			if !s.tombs.Has(tomb.Folder, f) {
				continue
			}
			busy := false
			for _, x := range s.sessions {
				if inFolder(x.Folder, f) {
					busy = true
					break
				}
			}
			for _, g := range s.folders {
				if g != f && inFolder(g, f) {
					busy = true
					break
				}
			}
			if busy {
				continue
			}
			old := s.folders
			s.folders = nil
			for _, g := range old {
				if g != f {
					s.addFolderLocked(g)
				}
			}
			folders++
			removed = true
			break
		}
		if !removed {
			break
		}
	}
	if s.folders == nil {
		s.folders = []string{}
	}
	if sessions > 0 {
		if err = s.flush(); err != nil {
			return
		}
	}
	if folders > 0 {
		err = s.flushFolders()
	}
	return
}

// CleanWebURL confere o endereço de uma sessão web: só http:// ou https://, com host e sem usuário/senha embutidos.
func CleanWebURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("informe o endereço da página")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return "", errors.New("endereço inválido")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("use um endereço http:// ou https://")
	}
	if u.User != nil {
		return "", errors.New("não coloque usuário e senha no endereço")
	}
	return u.String(), nil
}

func webHost(u string) string {
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return u
	}
	return p.Host
}
