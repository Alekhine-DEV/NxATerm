package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Macro é um comando (ou vários, um por linha) que o usuário dispara com um clique.
type Macro struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Enter   bool   `json:"enter"` // envia Enter ao final

	Pinned   bool     `json:"pinned,omitempty"`   // aparece primeiro na lista
	Order    int      `json:"order,omitempty"`    // posição escolhida pelo usuário
	Sessions []string `json:"sessions,omitempty"` // só aparece nestas sessões (vazio = em todas)
	Tags     []string `json:"tags,omitempty"`     // ... ou nas sessões com alguma destas tags
}

// MacroStore guarda as macros em JSON, no mesmo estilo do Store de sessões.
type MacroStore struct {
	mu     sync.Mutex
	path   string
	macros []Macro
}

// OpenMacros carrega o arquivo (vazio se não existir).
func OpenMacros(path string) (*MacroStore, error) {
	m := &MacroStore{path: path, macros: []Macro{}}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return m, nil
	case err != nil:
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(data, &m.macros); err != nil {
		return nil, fmt.Errorf("arquivo de macros corrompido (%s): %w", path, err)
	}
	return m, nil
}

// List devolve uma cópia ordenada por nome.
func (m *MacroStore) List() []Macro {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Macro, len(m.macros))
	copy(out, m.macros)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Reorder grava a ordem dada (IDs na sequência desejada); o que não estiver na lista vai para o fim.
func (m *MacroStore) Reorder(ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pos := map[string]int{}
	for i, id := range ids {
		pos[id] = i + 1
	}
	for i := range m.macros {
		if p, ok := pos[m.macros[i].ID]; ok {
			m.macros[i].Order = p
		} else {
			m.macros[i].Order = len(ids) + 1
		}
	}
	return m.flush()
}

// Save cria (ID vazio) ou atualiza uma macro.
func (m *MacroStore) Save(in Macro) (Macro, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return Macro{}, errors.New("dê um nome à macro")
	}
	if strings.TrimSpace(in.Command) == "" {
		return Macro{}, errors.New("a macro precisa de um comando")
	}
	in.Tags = CleanTags(in.Tags)
	m.mu.Lock()
	defer m.mu.Unlock()
	if in.ID == "" {
		id, err := newID()
		if err != nil {
			return Macro{}, err
		}
		in.ID = id
		m.macros = append(m.macros, in)
	} else {
		found := false
		for i := range m.macros {
			if m.macros[i].ID == in.ID {
				m.macros[i] = in
				found = true
				break
			}
		}
		if !found {
			return Macro{}, errors.New("macro não encontrada")
		}
	}
	if err := m.flush(); err != nil {
		return Macro{}, err
	}
	return in, nil
}

// Delete remove uma macro.
func (m *MacroStore) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.macros {
		if m.macros[i].ID == id {
			m.macros = append(m.macros[:i], m.macros[i+1:]...)
			return m.flush()
		}
	}
	return errors.New("macro não encontrada")
}

func (m *MacroStore) flush() error {
	data, err := json.MarshalIndent(m.macros, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(m.path, data)
}

// WriteFileAtomic grava em arquivo temporário e renomeia, para nunca deixar
// um arquivo pela metade se o app fechar no meio da escrita.
func WriteFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
