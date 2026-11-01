package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"nxaterm/internal/secfile"
)

func TestStoreEncryptedLifecycle(t *testing.T) {
	defer secfile.ClearKey()
	dir := t.TempDir()
	p := filepath.Join(dir, "sessions.json")
	k := bytes.Repeat([]byte{9}, 32)

	secfile.SetKey(k)
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Session{Name: "servidor-secreto", Host: "10.1.2.3", User: "root"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateFolder("Produção"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if !secfile.IsEncrypted(raw) || bytes.Contains(raw, []byte("10.1.2.3")) {
		t.Fatal("sessions.json não está criptografado")
	}
	rawF, _ := os.ReadFile(filepath.Join(dir, "folders.json"))
	if !secfile.IsEncrypted(rawF) {
		t.Fatal("folders.json não está criptografado")
	}

	// reabre sem a chave: bloqueado, vazio e sem gravar nada
	secfile.ClearKey()
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Locked() || len(s2.List()) != 0 {
		t.Fatal("deveria abrir bloqueado e vazio")
	}
	if _, err := s2.Save(Session{Name: "x", Host: "h", User: "u"}); !errors.Is(err, secfile.ErrLocked) {
		t.Fatalf("gravação bloqueada esperada: %v", err)
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(raw, after) {
		t.Fatal("o arquivo mudou enquanto bloqueado")
	}

	// desbloqueia
	secfile.SetKey(k)
	if err := s2.Reload(); err != nil {
		t.Fatal(err)
	}
	if s2.Locked() || len(s2.List()) != 1 || s2.List()[0].Host != "10.1.2.3" {
		t.Fatalf("dados não voltaram: %+v", s2.List())
	}
	if f := s2.Folders(); len(f) != 1 || f[0] != "Produção" {
		t.Fatalf("pastas: %v", f)
	}

	// remove a senha: volta a texto
	secfile.ClearKey()
	if err := s2.Rewrite(); err != nil {
		t.Fatal(err)
	}
	plain, _ := os.ReadFile(p)
	if secfile.IsEncrypted(plain) || !bytes.Contains(plain, []byte("10.1.2.3")) {
		t.Fatal("deveria ter voltado a texto")
	}
}

func TestMigratePlainToEncrypted(t *testing.T) {
	defer secfile.ClearKey()
	p := filepath.Join(t.TempDir(), "macros.json")
	m, _ := OpenMacros(p)
	if _, err := m.Save(Macro{Name: "deploy", Command: "ls -la"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if secfile.IsEncrypted(raw) {
		t.Fatal("começa em texto")
	}
	secfile.SetKey(bytes.Repeat([]byte{4}, 32))
	if err := m.Rewrite(); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(p)
	if !secfile.IsEncrypted(raw) || bytes.Contains(raw, []byte("ls -la")) {
		t.Fatal("macros não foram criptografadas")
	}
	secfile.ClearKey()
	m2, _ := OpenMacros(p)
	if !m2.Locked() {
		t.Fatal("deveria estar bloqueado")
	}
	if _, err := m2.Save(Macro{Name: "x", Command: "y"}); !errors.Is(err, secfile.ErrLocked) {
		t.Fatalf("esperava ErrLocked: %v", err)
	}
}
