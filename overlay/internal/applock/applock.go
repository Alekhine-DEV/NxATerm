// Package applock guarda a senha de abertura do app (bloqueio da tela). Só o hash fica no disco.
// Atenção: isto bloqueia o acesso à interface; os arquivos de sessões não são criptografados.
package applock

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"nxaterm/internal/kdf"
)

const (
	iterations = 200_000
	minLen     = 4
)

type record struct {
	Salt string `json:"salt"`
	Hash string `json:"hash"`
	Iter int    `json:"iter"`
}

// Lock é o bloqueio salvo em um arquivo.
type Lock struct {
	mu       sync.Mutex
	path     string
	rec      *record
	failures int
}

// Open carrega o arquivo (sem arquivo = sem senha).
func Open(path string) (*Lock, error) {
	l := &Lock{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	var r record
	if err := json.Unmarshal(data, &r); err != nil || r.Hash == "" || r.Salt == "" || r.Iter < 1000 {
		return nil, errors.New("arquivo da senha de abertura corrompido")
	}
	l.rec = &r
	return l, nil
}

// Enabled diz se existe senha de abertura.
func (l *Lock) Enabled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rec != nil
}

// Check confere a senha. Erros seguidos deixam a próxima tentativa mais lenta.
func (l *Lock) Check(pw string) bool {
	l.mu.Lock()
	rec := l.rec
	wait := time.Duration(min(l.failures, 8)) * 250 * time.Millisecond
	l.mu.Unlock()
	if rec == nil {
		return true
	}
	time.Sleep(wait)
	salt, err := hex.DecodeString(rec.Salt)
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(rec.Hash)
	if err != nil {
		return false
	}
	ok := subtle.ConstantTimeCompare(kdf.PBKDF2([]byte(pw), salt, rec.Iter, len(want)), want) == 1
	l.mu.Lock()
	if ok {
		l.failures = 0
	} else {
		l.failures++
	}
	l.mu.Unlock()
	return ok
}

// Set troca a senha. Com senha já definida, current precisa estar certa. newPw vazia remove a senha.
func (l *Lock) Set(current, newPw string) error {
	if l.Enabled() && !l.Check(current) {
		return errors.New("senha atual incorreta")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if newPw == "" {
		l.rec = nil
		if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if len([]rune(newPw)) < minLen {
		return errors.New("use pelo menos 4 caracteres")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	r := &record{Salt: hex.EncodeToString(salt), Iter: iterations, Hash: hex.EncodeToString(kdf.PBKDF2([]byte(newPw), salt, iterations, 32))}
	data, _ := json.Marshal(r)
	if err := os.WriteFile(l.path, data, 0o600); err != nil {
		return err
	}
	l.rec = r
	l.failures = 0
	return nil
}
