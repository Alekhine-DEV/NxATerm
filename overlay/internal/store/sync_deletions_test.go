package store

import (
	"path/filepath"
	"testing"

	"nxaterm/internal/tomb"
)

type pc struct {
	st *Store
	ms *MacroStore
	tl *tomb.Log
}

func newPC(t *testing.T) pc {
	t.Helper()
	d := t.TempDir()
	tl, err := tomb.Open(filepath.Join(d, "deleted.json"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(filepath.Join(d, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	ms, err := OpenMacros(filepath.Join(d, "macros.json"))
	if err != nil {
		t.Fatal(err)
	}
	st.SetTombs(tl)
	ms.SetTombs(tl)
	return pc{st, ms, tl}
}

// sync simula: b recebe a cópia de a (modo mesclar), como o SyncPull faz.
func (b pc) pullFrom(t *testing.T, a pc) {
	t.Helper()
	if err := b.tl.Merge(a.tl.List()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.st.ApplyTombstones(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ms.ApplyTombstones(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.st.MergeSync(a.st.List(), a.st.Folders()); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ms.MergeSync(a.ms.List()); err != nil {
		t.Fatal(err)
	}
}

func TestDeletionPropagates(t *testing.T) {
	a, b := newPC(t), newPC(t)
	s1, _ := a.st.Save(Session{Name: "web", Host: "10.0.0.1", User: "root"})
	s2, _ := a.st.Save(Session{Name: "db", Host: "10.0.0.2", User: "root"})
	m1, _ := a.ms.Save(Macro{Name: "up", Command: "uptime"})
	a.st.CreateFolder("Prod")
	b.pullFrom(t, a)
	if len(b.st.List()) != 2 || len(b.ms.List()) != 1 || len(b.st.Folders()) != 1 {
		t.Fatalf("cópia inicial: %d %d %v", len(b.st.List()), len(b.ms.List()), b.st.Folders())
	}

	// A apaga uma sessão, a macro e a pasta
	if err := a.st.Delete(s1.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.ms.Delete(m1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.st.DeleteFolder("Prod", false); err != nil {
		t.Fatal(err)
	}
	b.pullFrom(t, a)
	l := b.st.List()
	if len(l) != 1 || l[0].ID != s2.ID {
		t.Fatalf("a sessão apagada voltou ou sumiu a errada: %+v", l)
	}
	if len(b.ms.List()) != 0 {
		t.Fatal("a macro apagada não saiu")
	}
	if len(b.st.Folders()) != 0 {
		t.Fatalf("a pasta apagada não saiu: %v", b.st.Folders())
	}
}

func TestDeletedHereDoesNotComeBack(t *testing.T) {
	a, b := newPC(t), newPC(t)
	s1, _ := a.st.Save(Session{Name: "web", Host: "10.0.0.1", User: "root"})
	b.pullFrom(t, a)
	// B apaga; A (que ainda tem a sessão) é mesclada em B de novo: não pode voltar
	if err := b.st.Delete(s1.ID); err != nil {
		t.Fatal(err)
	}
	b.pullFrom(t, a)
	if len(b.st.List()) != 0 {
		t.Fatal("sessão apagada voltou pela mesclagem")
	}
	// e quando A recebe a exclusão, ela também some de A
	a.pullFrom(t, b)
	if len(a.st.List()) != 0 {
		t.Fatal("A não recebeu a exclusão de B")
	}
}

func TestFolderWithSessionsIsKept(t *testing.T) {
	a, b := newPC(t), newPC(t)
	a.st.CreateFolder("Lab")
	b.pullFrom(t, a)
	b.st.Save(Session{Name: "x", Host: "h", User: "u", Folder: "Lab"})
	a.st.DeleteFolder("Lab", false)
	b.pullFrom(t, a)
	if len(b.st.Folders()) != 1 {
		t.Fatalf("pasta com sessões deveria ficar: %v", b.st.Folders())
	}
}

func TestRecreatedFolderIsRevived(t *testing.T) {
	a := newPC(t)
	a.st.CreateFolder("Tmp")
	a.st.DeleteFolder("Tmp", false)
	if !a.tl.Has(tomb.Folder, "Tmp") {
		t.Fatal("exclusão não registrada")
	}
	a.st.CreateFolder("Tmp")
	if a.tl.Has(tomb.Folder, "Tmp") {
		t.Fatal("pasta recriada continua marcada como apagada")
	}
}

func TestReplaceAllClearsTombstones(t *testing.T) {
	a := newPC(t)
	s, _ := a.st.Save(Session{Name: "web", Host: "h", User: "u"})
	a.st.Delete(s.ID)
	if !a.tl.Has(tomb.Session, s.ID) {
		t.Fatal("exclusão não registrada")
	}
	a.st.ReplaceAll([]Session{{ID: s.ID, Name: "web", Host: "h", User: "u"}}, nil)
	if a.tl.Has(tomb.Session, s.ID) {
		t.Fatal("sessão restaurada continua marcada como apagada")
	}
}
