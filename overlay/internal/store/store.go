// Package store guarda os perfis de sessão SSH em um arquivo JSON.
// Não há limite de quantidade de sessões. Senhas NUNCA são gravadas aqui.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
)

// Session é um perfil de conexão (SSH, SFTP, Telnet, serial, shell local ou WSL).
type Session struct {
	ID             string `json:"id"`
	Type           string `json:"type"` // um dos Type* acima; vazio = ssh (perfis antigos)
	Name           string `json:"name"`
	Folder         string `json:"folder"` // pasta para organizar na barra lateral, ex: "Produção/Web"
	Host           string `json:"host"`
	Port           int    `json:"port"`
	User           string `json:"user"`
	AuthMethod     string `json:"authMethod"`     // "password" ou "key" (ssh e sftp)
	KeyPath        string `json:"keyPath"`        // caminho da chave privada quando AuthMethod == "key"
	StartupCommand string `json:"startupCommand"` // comando executado logo após conectar
	Notes          string `json:"notes"`

	// Serial
	SerialPort string `json:"serialPort"` // ex: COM3 ou /dev/ttyUSB0
	BaudRate   int    `json:"baudRate"`
	DataBits   int    `json:"dataBits"` // 5 a 8
	Parity     string `json:"parity"`   // none, odd, even, mark, space
	StopBits   string `json:"stopBits"` // 1, 1.5, 2

	// Shell local e WSL
	Shell  string `json:"shell"`  // programa do shell local (vazio = padrão do sistema)
	Distro string `json:"distro"` // distribuição do WSL (vazio = a padrão)
}

// Store mantém a lista de sessões em memória e no disco.
type Store struct {
	mu          sync.Mutex
	path        string
	sessions    []Session
	foldersPath string   // pastas criadas pelo usuário (inclusive vazias)
	folders     []string // sem duplicatas, já normalizadas
}

// Open carrega o arquivo (criando-o vazio se não existir).
func Open(path string) (*Store, error) {
	s := &Store{path: path, foldersPath: filepath.Join(filepath.Dir(path), "folders.json")}
	if err := s.loadFolders(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.sessions = []Session{}
		return s, nil
	case err != nil:
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		s.sessions = []Session{}
		return s, nil
	}
	if err := json.Unmarshal(data, &s.sessions); err != nil {
		return nil, fmt.Errorf("arquivo de sessões corrompido (%s): %w", path, err)
	}
	return s, nil
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
		case "password", "key":
		case "":
			in.AuthMethod = "password"
		default:
			return in, fmt.Errorf("método de autenticação inválido: %q", in.AuthMethod)
		}
		if in.AuthMethod == "key" && strings.TrimSpace(in.KeyPath) == "" {
			return in, errors.New("informe o caminho da chave privada")
		}

	case TypeTelnet:
		if err := normalizeRemote(&in, 23, false); err != nil {
			return in, err
		}
		in.User, in.AuthMethod, in.KeyPath = "", "password", ""
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
		if in.Name == "" {
			in.Name = fmt.Sprintf("%s (%d)", in.SerialPort, in.BaudRate)
		}

	case TypeShell:
		in.Shell = strings.TrimSpace(in.Shell)
		in.Host, in.User, in.Port = "", "", 0
		if in.Name == "" {
			in.Name = "Shell local"
		}

	case TypeWSL:
		in.Distro = strings.TrimSpace(in.Distro)
		in.Host, in.User, in.Port = "", "", 0
		if in.Name == "" {
			in.Name = "WSL"
			if in.Distro != "" {
				in.Name = "WSL: " + in.Distro
			}
		}

	default:
		return in, fmt.Errorf("tipo de sessão inválido: %q", in.Type)
	}
	return in, nil
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
			return s.flush()
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

// flush grava de forma atômica (arquivo temporário + rename).
func (s *Store) flush() error {
	data, err := json.MarshalIndent(s.sessions, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".sessions-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
