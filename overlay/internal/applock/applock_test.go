package applock

import (
	"encoding/hex"
	"path/filepath"
	"testing"
)

func TestPBKDF2Vector(t *testing.T) {
	// RFC 7914, seção 11
	got := hex.EncodeToString(pbkdf2([]byte("passwd"), []byte("salt"), 1, 64))
	want := "55ac046e56e3089fec1691c22544b605f94185216dde0465e68b9d57c20dacbc49ca9cccf179b645991664b39d77ef317c71b845b1e30bd509112041d3a19783"
	if got != want {
		t.Fatalf("pbkdf2 = %s", got)
	}
}

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
