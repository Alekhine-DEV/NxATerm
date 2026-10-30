package main

import (
	"strings"
	"testing"
	"time"

	"nxaterm/internal/store"
)

func TestSessionsHTMLEscapesAndGroups(t *testing.T) {
	list := []store.Session{
		{Name: `<b>x</b>`, Type: "ssh", Host: "h", User: "u", Port: 22, Folder: "A/B", Notes: `a & b`},
		{Name: "serial", Type: "serial", SerialPort: "COM3", BaudRate: 9600},
	}
	out := sessionsHTML(list, []string{"Vazia"}, time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC))
	for _, want := range []string{"&lt;b&gt;x&lt;/b&gt;", "A › B", "(sem pasta)", "Vazia", "pasta vazia", "COM3 · 9600 baud", "a &amp; b", "u@h:22"} {
		if !strings.Contains(out, want) {
			t.Errorf("faltou %q", want)
		}
	}
	if strings.Contains(out, "<b>x</b>") {
		t.Error("HTML não escapado")
	}
}

func newSvc(t *testing.T) *TermService {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/sessions.json")
	if err != nil {
		t.Fatal(err)
	}
	return &TermService{store: st}
}

func TestImportBytesDetectsFormat(t *testing.T) {
	a := newSvc(t)
	r, err := a.importBytes([]byte(`{"app":"NxATerm","sessions":[{"host":"h","user":"u"}],"folders":["F"]}`))
	if err != nil || r.Added != 1 {
		t.Fatalf("json: %+v %v", r, err)
	}
	r, err = a.importBytes([]byte("[Bookmarks]\r\nSubRep=\r\nImgNum=42\r\nx=#109#0%srv%22%root%\r\n[Bookmarks_1]\r\nSubRep=P\\Q\r\ny=#98#1%sw%23%\r\n"))
	if err != nil || r.Added != 2 || r.Folders != 1 {
		t.Fatalf("moba: %+v %v", r, err)
	}
	r, err = a.importBytes([]byte("Host web\n  HostName 10.0.0.9\n  User deploy\n"))
	if err != nil || r.Added != 1 {
		t.Fatalf("ssh config: %+v %v", r, err)
	}
	if _, err := a.importBytes([]byte("isso não é nada conhecido")); err == nil {
		t.Error("formato desconhecido deveria falhar")
	}
	// importar de novo não duplica
	r, _ = a.importBytes([]byte(`{"sessions":[{"host":"h","user":"u"}]}`))
	if r.Added != 0 || r.Skipped != 1 {
		t.Fatalf("duplicata: %+v", r)
	}
}
