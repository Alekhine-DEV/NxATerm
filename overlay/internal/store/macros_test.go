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

func TestMacroOrderAndPin(t *testing.T) {
	m, err := OpenMacros(filepath.Join(t.TempDir(), "m.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, n := range []string{"c", "a", "b"} {
		x, err := m.Save(Macro{Name: n, Command: "ls"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, x.ID)
	}
	names := func() string {
		s := ""
		for _, x := range m.List() {
			s += x.Name
		}
		return s
	}
	if names() != "abc" {
		t.Fatalf("ordem padrão: %s", names())
	}
	if err := m.Reorder([]string{ids[0], ids[2], ids[1]}); err != nil || names() != "cba" {
		t.Fatalf("reordenar: %s %v", names(), err)
	}
	x, _ := m.Save(Macro{ID: ids[1], Name: "a", Command: "ls", Pinned: true, Tags: []string{"web", "Web"}})
	if names() != "acb" || len(x.Tags) != 1 {
		t.Fatalf("fixar: %s %v", names(), x.Tags)
	}
}
