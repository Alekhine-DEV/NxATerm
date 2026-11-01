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
