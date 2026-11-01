// Package tomb guarda o registro do que o usuário apagou (sessões, macros e pastas), para que a
// sincronização propague as exclusões em vez de trazer tudo de volta.
package tomb

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"sync"
	"time"

	"nxaterm/internal/secfile"
)

// Tipos de item.
const (
	Session = "session"
	Macro   = "macro"
	Folder  = "folder"
)

// Mantém o registro por um ano: depois disso uma cópia muito antiga pode trazer o item de volta.
const retention = 365 * 24 * time.Hour

// Entry é uma exclusão. Key é o ID (sessão e macro) ou o caminho (pasta).
type Entry struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
	At   int64  `json:"at"` // segundos desde 1970
}

// Log é o registro, salvo em um arquivo (criptografado quando há senha de abertura).
type Log struct {
	mu      sync.Mutex
	path    string
	entries []Entry
	locked  bool
	now     func() time.Time
}

// Open carrega o arquivo (vazio se não existir; bloqueado se estiver criptografado e sem senha ainda).
func Open(path string) (*Log, error) {
	l := &Log{path: path, now: time.Now}
	if err := l.Reload(); err != nil {
		return nil, err
	}
	return l, nil
}

// Reload lê de novo o arquivo.
func (l *Log) Reload() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.locked = false
	l.entries = []Entry{}
	data, err := secfile.Read(l.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case errors.Is(err, secfile.ErrLocked):
		l.locked = true
		return nil
	case err != nil:
		return err
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &l.entries); err != nil {
		return errors.New("registro de exclusões corrompido")
	}
	return nil
}

// Locked diz se o arquivo ainda está protegido.
func (l *Log) Locked() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.locked
}

// Rewrite grava de novo no formato atual (criptografado ou não).
func (l *Log) Rewrite() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.flush()
}

func (l *Log) flush() error {
	if l.locked {
		return secfile.ErrLocked
	}
	l.prune()
	data, err := json.Marshal(l.entries)
	if err != nil {
		return err
	}
	return secfile.Write(l.path, data)
}

func (l *Log) prune() {
	limit := l.now().Add(-retention).Unix()
	kept := l.entries[:0]
	for _, e := range l.entries {
		if e.At >= limit {
			kept = append(kept, e)
		}
	}
	l.entries = kept
}

// Add registra uma exclusão. Chamadas com a lista vazia não fazem nada.
func (l *Log) Add(kind string, keys ...string) error {
	if l == nil || len(keys) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.locked {
		return secfile.ErrLocked
	}
	at := l.now().Unix()
	for _, k := range keys {
		if k == "" {
			continue
		}
		l.setLocked(Entry{Kind: kind, Key: k, At: at})
	}
	return l.flush()
}

// setLocked acrescenta ou atualiza a entrada, ficando com a data mais recente.
func (l *Log) setLocked(e Entry) bool {
	for i := range l.entries {
		if l.entries[i].Kind == e.Kind && l.entries[i].Key == e.Key {
			if e.At > l.entries[i].At {
				l.entries[i].At = e.At
				return true
			}
			return false
		}
	}
	l.entries = append(l.entries, e)
	return true
}

// Revive tira o item do registro (por exemplo, quando o usuário cria uma pasta com o mesmo nome).
func (l *Log) Revive(kind, key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.locked {
		return
	}
	for i := range l.entries {
		if l.entries[i].Kind == kind && l.entries[i].Key == key {
			l.entries = append(l.entries[:i], l.entries[i+1:]...)
			l.flush()
			return
		}
	}
}

// Unmark tira vários itens do registro de uma vez (itens que voltaram a existir).
func (l *Log) Unmark(kind string, keys []string) {
	if l == nil || len(keys) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.locked {
		return
	}
	drop := make(map[string]bool, len(keys))
	for _, k := range keys {
		drop[k] = true
	}
	kept := l.entries[:0]
	changed := false
	for _, e := range l.entries {
		if e.Kind == kind && drop[e.Key] {
			changed = true
			continue
		}
		kept = append(kept, e)
	}
	l.entries = kept
	if changed {
		l.flush()
	}
}

// Has diz se o item foi apagado.
func (l *Log) Has(kind, key string) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if e.Kind == kind && e.Key == key {
			return true
		}
	}
	return false
}

// List devolve uma cópia ordenada.
func (l *Log) List() []Entry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := append([]Entry{}, l.entries...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].At != out[j].At {
			return out[i].At < out[j].At
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// Merge junta as exclusões de outra cópia e grava. Entradas inválidas são ignoradas.
func (l *Log) Merge(in []Entry) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.locked {
		return secfile.ErrLocked
	}
	changed := false
	limit := l.now().Add(-retention).Unix()
	for _, e := range in {
		if e.Key == "" || (e.Kind != Session && e.Kind != Macro && e.Kind != Folder) || e.At < limit || e.At > l.now().Add(48*time.Hour).Unix() {
			continue
		}
		if l.setLocked(e) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return l.flush()
}
