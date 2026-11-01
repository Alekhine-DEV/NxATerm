package applock

import (
	"path/filepath"
	"testing"
)

func TestLockFlow(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lock.json")
	l, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if l.Enabled() || !l.Check("qualquer") {
		t.Fatal("sem senha tudo passa")
	}
	if err := l.Set("", "ab"); err == nil {
		t.Fatal("senha curta aceita")
	}
	if err := l.Set("", "segredo"); err != nil {
		t.Fatal(err)
	}
	l2, err := Open(p)
	if err != nil || !l2.Enabled() {
		t.Fatalf("reabrir: %v", err)
	}
	if l2.Check("errada") || !l2.Check("segredo") {
		t.Fatal("conferência errada")
	}
	if err := l2.Set("errada", "nova123"); err == nil {
		t.Fatal("trocou com senha atual errada")
	}
	if err := l2.Set("segredo", "nova123"); err != nil || !l2.Check("nova123") {
		t.Fatalf("troca: %v", err)
	}
	if err := l2.Set("nova123", ""); err != nil || l2.Enabled() {
		t.Fatalf("remover: %v", err)
	}
}

func TestDeriveKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lock.json")
	l, _ := Open(p)
	if _, err := l.DeriveKey("x"); err == nil {
		t.Fatal("sem senha não deveria derivar chave")
	}
	if err := l.Set("", "segredo"); err != nil {
		t.Fatal(err)
	}
	k1, err := l.DeriveKey("segredo")
	if err != nil || len(k1) != 32 {
		t.Fatalf("chave: %v %d", err, len(k1))
	}
	if _, err := l.DeriveKey("errada"); err == nil {
		t.Fatal("senha errada derivou chave")
	}
	l2, _ := Open(p)
	k2, _ := l2.DeriveKey("segredo")
	if string(k1) != string(k2) {
		t.Fatal("a chave muda entre aberturas")
	}
	// trocar a senha troca a chave
	if err := l2.Set("segredo", "outra-senha"); err != nil {
		t.Fatal(err)
	}
	k3, _ := l2.DeriveKey("outra-senha")
	if string(k3) == string(k1) {
		t.Fatal("chave igual após trocar a senha")
	}
}

func TestLegacyRecordGetsKeySalt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lock.json")
	l, _ := Open(p)
	l.Set("", "segredo")
	// simula arquivo antigo: tira o sal da chave
	l.mu.Lock()
	r := *l.rec
	r.KSalt = ""
	l.save(&r)
	l.mu.Unlock()
	l2, _ := Open(p)
	k1, err := l2.DeriveKey("segredo")
	if err != nil {
		t.Fatal(err)
	}
	l3, _ := Open(p)
	k2, _ := l3.DeriveKey("segredo")
	if string(k1) != string(k2) {
		t.Fatal("o sal criado na migração não foi salvo")
	}
}
