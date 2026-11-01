package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"nxaterm/internal/secfile"
	"nxaterm/internal/tomb"
	"os"
	"sort"
	"strings"
)

// CleanFolder normaliza um caminho de pasta: aceita "\" e "/", tira espaços e
// barras sobrando. "A \ B/" vira "A/B".
func CleanFolder(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	var parts []string
	for _, x := range strings.Split(p, "/") {
		if x = strings.TrimSpace(x); x != "" {
			parts = append(parts, x)
		}
	}
	return strings.Join(parts, "/")
}

func parentFolder(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

// inFolder diz se f é a pasta path ou está dentro dela.
func inFolder(f, path string) bool { return f == path || strings.HasPrefix(f, path+"/") }

func (s *Store) loadFolders() error {
	data, err := secfile.Read(s.foldersPath)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(strings.TrimSpace(string(data))) == 0) {
		s.folders = []string{}
		return nil
	}
	if err != nil {
		return err
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("arquivo de pastas corrompido (%s): %w", s.foldersPath, err)
	}
	s.folders = nil
	for _, f := range list {
		s.addFolderLocked(f)
	}
	if s.folders == nil {
		s.folders = []string{}
	}
	return nil
}

func (s *Store) flushFolders() error {
	if s.locked {
		return secfile.ErrLocked
	}
	data, err := json.MarshalIndent(s.folders, "", "  ")
	if err != nil {
		return err
	}
	return secfile.Write(s.foldersPath, data)
}

func (s *Store) addFolderLocked(p string) bool {
	p = CleanFolder(p)
	if p == "" {
		return false
	}
	for _, f := range s.folders {
		if f == p {
			return false
		}
	}
	s.folders = append(s.folders, p)
	sort.Strings(s.folders)
	return true
}

// Folders devolve as pastas criadas pelo usuário (as pastas das sessões não entram aqui).
func (s *Store) Folders() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.folders...)
}

// CreateFolder cria uma pasta, mesmo sem nenhuma sessão dentro.
func (s *Store) CreateFolder(p string) error {
	p = CleanFolder(p)
	if p == "" {
		return errors.New("informe o nome da pasta")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tombs.Revive(tomb.Folder, p)
	if s.addFolderLocked(p) {
		return s.flushFolders()
	}
	return nil
}

// RenameFolder muda o caminho de uma pasta, das subpastas e das sessões dentro dela.
func (s *Store) RenameFolder(from, to string) error {
	from, to = CleanFolder(from), CleanFolder(to)
	if from == "" || to == "" {
		return errors.New("informe o nome da pasta")
	}
	if from == to {
		return nil
	}
	if inFolder(to, from) {
		return errors.New("não dá para mover uma pasta para dentro dela mesma")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	remap := func(f string) string {
		if inFolder(f, from) {
			return to + f[len(from):]
		}
		return f
	}
	for i := range s.sessions {
		s.sessions[i].Folder = remap(s.sessions[i].Folder)
	}
	old := s.folders
	s.folders = nil
	gone := []string{}
	for _, f := range old {
		if inFolder(f, from) {
			gone = append(gone, f)
		}
		s.addFolderLocked(remap(f))
	}
	gone = append(gone, from)
	s.tombs.Revive(tomb.Folder, to)
	s.addFolderLocked(to) // a pasta renomeada passa a existir mesmo que só tivesse sessões
	if err := s.flush(); err != nil {
		return err
	}
	if err := s.flushFolders(); err != nil {
		return err
	}
	// o caminho antigo deixou de existir (outros computadores precisam saber)
	return s.tombs.Add(tomb.Folder, gone...)
}

// DeleteFolder apaga uma pasta. Com withSessions=false as sessões dela sobem para
// a pasta de cima; com true elas são apagadas também. Devolve quantas sessões foram apagadas.
func (s *Store) DeleteFolder(path string, withSessions bool) (int, error) {
	path = CleanFolder(path)
	if path == "" {
		return 0, errors.New("informe a pasta")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parent := parentFolder(path)
	lift := func(f string) string { // tira o último nível "path" do caminho
		rest := strings.TrimPrefix(strings.TrimPrefix(f, path), "/")
		if parent == "" {
			return rest
		}
		if rest == "" {
			return parent
		}
		return parent + "/" + rest
	}

	deleted := 0
	var goneSessions, goneFolders []string
	goneFolders = append(goneFolders, path)
	kept := s.sessions[:0:0]
	for _, x := range s.sessions {
		if inFolder(x.Folder, path) {
			if withSessions {
				deleted++
				goneSessions = append(goneSessions, x.ID)
				continue
			}
			x.Folder = lift(x.Folder)
		}
		kept = append(kept, x)
	}
	s.sessions = kept

	old := s.folders
	s.folders = nil
	for _, f := range old {
		switch {
		case f == path:
		case inFolder(f, path) && withSessions:
			goneFolders = append(goneFolders, f)
		case inFolder(f, path):
			goneFolders = append(goneFolders, f)
			s.addFolderLocked(lift(f))
		default:
			s.addFolderLocked(f)
		}
	}
	if err := s.flush(); err != nil {
		return deleted, err
	}
	if err := s.flushFolders(); err != nil {
		return deleted, err
	}
	if err := s.tombs.Add(tomb.Session, goneSessions...); err != nil {
		return deleted, err
	}
	return deleted, s.tombs.Add(tomb.Folder, goneFolders...)
}

// exportFile é o formato do arquivo de exportação do NxATerm.
type exportFile struct {
	App      string    `json:"app"`
	Version  int       `json:"version"`
	Folders  []string  `json:"folders"`
	Sessions []Session `json:"sessions"`
}

// Export gera o JSON com todas as sessões e pastas. Senhas nunca são guardadas
// pelo NxATerm, então não existem no arquivo.
func (s *Store) Export() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := exportFile{App: "NxATerm", Version: 1, Folders: append([]string{}, s.folders...), Sessions: make([]Session, len(s.sessions))}
	copy(out.Sessions, s.sessions)
	for i := range out.Sessions {
		out.Sessions[i].ID = ""
	}
	return json.MarshalIndent(out, "", "  ")
}

// ImportJSON lê um arquivo do NxATerm (ou uma lista simples de sessões) e adiciona
// o que ainda não existe. Devolve quantas sessões entraram e quantas já existiam/eram inválidas.
func (s *Store) ImportJSON(data []byte) (added, skipped int, err error) {
	var file exportFile
	trimmed := strings.TrimSpace(string(data))
	switch {
	case strings.HasPrefix(trimmed, "["):
		if err := json.Unmarshal(data, &file.Sessions); err != nil {
			return 0, 0, fmt.Errorf("arquivo inválido: %w", err)
		}
	default:
		if err := json.Unmarshal(data, &file); err != nil {
			return 0, 0, fmt.Errorf("arquivo inválido: %w", err)
		}
	}
	return s.ImportAll(file.Sessions, file.Folders)
}

// ImportAll adiciona sessões e pastas (sem duplicar) e devolve os totais.
func (s *Store) ImportAll(list []Session, folders []string) (added, skipped int, err error) {
	added, err = s.Import(list)
	if err != nil {
		return added, 0, err
	}
	skipped = len(list) - added
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, f := range folders {
		if s.addFolderLocked(f) {
			changed = true
		}
	}
	if changed {
		err = s.flushFolders()
	}
	return added, skipped, err
}
