package store

import (
	"path/filepath"
	"testing"
)

func TestMacrosCRUD(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x", "macros.json")
	m, err := OpenMacros(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Save(Macro{Name: "", Command: "ls"}); err == nil {
		t.Fatal("nome vazio deveria falhar")
	}
	if _, err := m.Save(Macro{Name: "x", Command: "  "}); err == nil {
		t.Fatal("comando vazio deveria falhar")
	}
	a, err := m.Save(Macro{Name: "Zeta", Command: "uptime", Enter: true})
	if err != nil || a.ID == "" {
		t.Fatalf("save: %v %+v", err, a)
	}
	b, _ := m.Save(Macro{Name: "alfa", Command: "df -h"})
	if l := m.List(); len(l) != 2 || l[0].ID != b.ID {
		t.Fatalf("ordenação por nome: %+v", l)
	}
	a.Command = "uptime -p"
	if _, err := m.Save(a); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Save(Macro{ID: "nope", Name: "n", Command: "c"}); err == nil {
		t.Fatal("id inexistente deveria falhar")
	}
	m2, err := OpenMacros(p)
	if err != nil {
		t.Fatal(err)
	}
	if l := m2.List(); len(l) != 2 || l[1].Command != "uptime -p" || !l[1].Enter {
		t.Fatalf("não persistiu: %+v", l)
	}
	if err := m2.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := m2.Delete(a.ID); err == nil {
		t.Fatal("apagar duas vezes deveria falhar")
	}
	if len(m2.List()) != 1 {
		t.Fatal("delete não funcionou")
	}
}
