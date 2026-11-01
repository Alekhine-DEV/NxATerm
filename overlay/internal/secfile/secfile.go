// Package secfile lê e grava os arquivos de dados do app. Com uma chave definida (derivada da
// senha de abertura), tudo o que é gravado fica criptografado (AES-256-GCM); sem chave, o arquivo
// é gravado em texto. A leitura aceita os dois formatos, o que permite migrar arquivos antigos.
package secfile

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// ErrLocked indica que o arquivo está criptografado e a senha de abertura ainda não foi digitada.
var ErrLocked = errors.New("dados protegidos: digite a senha de abertura")

var magic = []byte("NXENC1\n")

var (
	mu  sync.RWMutex
	key []byte
)

// SetKey define a chave de 32 bytes usada nas próximas leituras e gravações.
func SetKey(k []byte) {
	mu.Lock()
	defer mu.Unlock()
	if len(k) == 0 {
		key = nil
		return
	}
	key = append([]byte(nil), k...)
}

// ClearKey remove a chave: as próximas gravações voltam a ser em texto.
func ClearKey() { SetKey(nil) }

// HasKey diz se há chave definida.
func HasKey() bool {
	mu.RLock()
	defer mu.RUnlock()
	return key != nil
}

func currentKey() []byte {
	mu.RLock()
	defer mu.RUnlock()
	return key
}

// IsEncrypted diz se o conteúdo está no formato criptografado.
func IsEncrypted(data []byte) bool { return bytes.HasPrefix(data, magic) }

// Encrypt cifra plain com a chave k.
func Encrypt(k, plain []byte) ([]byte, error) {
	gcm, err := newGCM(k)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte{}, magic...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plain, magic), nil
}

// Decrypt abre o conteúdo gerado por Encrypt.
func Decrypt(k, data []byte) ([]byte, error) {
	if !IsEncrypted(data) {
		return nil, errors.New("arquivo não está criptografado")
	}
	gcm, err := newGCM(k)
	if err != nil {
		return nil, err
	}
	body := data[len(magic):]
	if len(body) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("arquivo criptografado corrompido")
	}
	plain, err := gcm.Open(nil, body[:gcm.NonceSize()], body[gcm.NonceSize():], magic)
	if err != nil {
		return nil, errors.New("não foi possível abrir o arquivo (senha errada ou arquivo corrompido)")
	}
	return plain, nil
}

func newGCM(k []byte) (cipher.AEAD, error) {
	if len(k) != 32 {
		return nil, errors.New("chave inválida")
	}
	b, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// Decode devolve o texto de um conteúdo lido do disco, abrindo-o se estiver criptografado.
func Decode(data []byte) ([]byte, error) {
	if !IsEncrypted(data) {
		return data, nil
	}
	k := currentKey()
	if k == nil {
		return nil, ErrLocked
	}
	return Decrypt(k, data)
}

// Read lê o arquivo (os.ErrNotExist continua sendo devolvido como está).
func Read(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decode(data)
}

// Write grava de forma atômica, criptografando se houver chave.
func Write(path string, data []byte) error {
	if k := currentKey(); k != nil {
		enc, err := Encrypt(k, data)
		if err != nil {
			return err
		}
		data = enc
	}
	return WriteAtomic(path, data, 0o600)
}

// WriteAtomic grava em arquivo temporário e renomeia, para nunca deixar um arquivo pela metade.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
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
	if err := os.Chmod(name, perm); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
