package secfile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func key32(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func TestPlainThenEncrypted(t *testing.T) {
	defer ClearKey()
	p := filepath.Join(t.TempDir(), "a.json")
	if err := Write(p, []byte(`{"x":1}`)); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if IsEncrypted(raw) || string(raw) != `{"x":1}` {
		t.Fatalf("sem chave deveria gravar texto: %q", raw)
	}
	SetKey(key32(1))
	if err := Write(p, []byte("segredo-da-sessao")); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(p)
	if !IsEncrypted(raw) || bytes.Contains(raw, []byte("segredo")) {
		t.Fatal("arquivo não ficou criptografado")
	}
	got, err := Read(p)
	if err != nil || string(got) != "segredo-da-sessao" {
		t.Fatalf("leitura: %q %v", got, err)
	}
	ClearKey()
	if _, err := Read(p); !errors.Is(err, ErrLocked) {
		t.Fatalf("sem chave deveria dar ErrLocked: %v", err)
	}
	SetKey(key32(2))
	if _, err := Read(p); err == nil || errors.Is(err, ErrLocked) {
		t.Fatalf("chave errada deveria falhar: %v", err)
	}
}

func TestTamperAndNonce(t *testing.T) {
	k := key32(7)
	a, _ := Encrypt(k, []byte("igual"))
	b, _ := Encrypt(k, []byte("igual"))
	if bytes.Equal(a, b) {
		t.Fatal("nonce repetido")
	}
	a[len(a)-1] ^= 1
	if _, err := Decrypt(k, a); err == nil {
		t.Fatal("adulteração não detectada")
	}
	if _, err := Decrypt(k, []byte("NXENC1\ncurto")); err == nil {
		t.Fatal("arquivo curto aceito")
	}
	if _, err := Encrypt([]byte("curta"), nil); err == nil {
		t.Fatal("chave curta aceita")
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := Read(filepath.Join(t.TempDir(), "nao-existe")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("esperava ErrNotExist: %v", err)
	}
}
