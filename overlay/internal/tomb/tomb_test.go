package tomb

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nxaterm/internal/secfile"
)

func TestAddHasPersist(t *testing.T) {
	p := filepath.Join(t.TempDir(), "deleted.json")
	l, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Add(Session, "a1", "a2"); err != nil {
		t.Fatal(err)
	}
	l.Add(Folder, "Prod/Web")
	if !l.Has(Session, "a1") || l.Has(Macro, "a1") || l.Has(Session, "zz") {
		t.Fatal("Has errado")
	}
	l2, _ := Open(p)
	if !l2.Has(Folder, "Prod/Web") || len(l2.List()) != 3 {
		t.Fatalf("não persistiu: %+v", l2.List())
	}
	l2.Revive(Folder, "Prod/Web")
	l3, _ := Open(p)
	if l3.Has(Folder, "Prod/Web") {
		t.Fatal("Revive não gravou")
	}
}

func TestMergeKeepsNewestAndFilters(t *testing.T) {
	l, _ := Open(filepath.Join(t.TempDir(), "d.json"))
	now := time.Now().Unix()
	l.Add(Session, "x")
	err := l.Merge([]Entry{
		{Session, "y", now - 10},
		{"estranho", "z", now},
		{Macro, "", now},
		{Macro, "velha", now - int64(400*24*time.Hour/time.Second)},
		{Macro, "futuro", now + 10*24*3600},
		{Session, "x", now + 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !l.Has(Session, "y") || l.Has("estranho", "z") || l.Has(Macro, "velha") || l.Has(Macro, "futuro") {
		t.Fatalf("filtro errado: %+v", l.List())
	}
	for _, e := range l.List() {
		if e.Key == "x" && e.At != now+5 {
			t.Fatal("data mais recente não prevaleceu")
		}
	}
}

func TestPrune(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.json")
	l, _ := Open(p)
	old := time.Now().Add(-400 * 24 * time.Hour)
	l.now = func() time.Time { return old }
	l.Add(Session, "antiga")
	l.now = time.Now
	l.Add(Session, "nova")
	if l.Has(Session, "antiga") || !l.Has(Session, "nova") {
		t.Fatalf("poda errada: %+v", l.List())
	}
}

func TestEncryptedAndLocked(t *testing.T) {
	defer secfile.ClearKey()
	p := filepath.Join(t.TempDir(), "d.json")
	secfile.SetKey(bytes.Repeat([]byte{3}, 32))
	l, _ := Open(p)
	l.Add(Session, "id-secreto")
	raw, _ := os.ReadFile(p)
	if !secfile.IsEncrypted(raw) {
		t.Fatal("deveria estar criptografado")
	}
	secfile.ClearKey()
	l2, _ := Open(p)
	if !l2.Locked() {
		t.Fatal("deveria abrir bloqueado")
	}
	if err := l2.Add(Session, "x"); !errors.Is(err, secfile.ErrLocked) {
		t.Fatalf("esperava ErrLocked: %v", err)
	}
	secfile.SetKey(bytes.Repeat([]byte{3}, 32))
	l2.Reload()
	if !l2.Has(Session, "id-secreto") {
		t.Fatal("não voltou após desbloquear")
	}
}

func TestNilLogIsSafe(t *testing.T) {
	var l *Log
	if l.Has(Session, "a") || l.Add(Session, "a") != nil || len(l.List()) != 0 || l.Merge(nil) != nil {
		t.Fatal("log nulo deveria ser inofensivo")
	}
	l.Revive(Session, "a")
}
