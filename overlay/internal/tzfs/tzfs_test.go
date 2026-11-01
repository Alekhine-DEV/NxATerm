package tzfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeName(t *testing.T) {
	for in, want := range map[string]string{
		"a.txt": "a.txt", "../../etc/passwd": "passwd", `..\x\y.txt`: "y.txt", "..": "arquivo", "": "arquivo",
		"a:b?.txt": "a_b_.txt", "nul": "_nul", "COM1.txt": "_COM1.txt", "com10": "com10", "x. ": "x",
	} {
		if got := SafeName(in); got != want {
			t.Errorf("%q -> %q, queria %q", in, got, want)
		}
	}
}

func TestUploadTree(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "pasta", "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "pasta", "a.txt"), []byte("hello world"), 0o644)
	os.WriteFile(filepath.Join(root, "pasta", "sub", "b.bin"), []byte{1, 2, 3}, 0o644)
	os.Symlink(root, filepath.Join(root, "pasta", "laco"))
	fs := New()
	d, err := fs.AddDir("o", filepath.Join(root, "pasta"))
	if err != nil || d.Name != "pasta" || d.Kind != "directory" {
		t.Fatal(d, err)
	}
	l, err := fs.List(d.Token)
	if err != nil || len(l) != 2 {
		t.Fatalf("lista: %+v %v", l, err)
	}
	var file Node
	for _, n := range l {
		if n.Kind == "file" {
			file = n
		}
	}
	if file.Name != "a.txt" || file.Size != 11 {
		t.Fatal(file)
	}
	b, err := fs.Read(file.Token, 6, 100)
	if err != nil || string(b) != "world" {
		t.Fatalf("%q %v", b, err)
	}
	if b, _ := fs.Read(file.Token, 11, 10); len(b) != 0 {
		t.Fatal("deveria estar no fim")
	}
	if _, err := fs.List(file.Token); err == nil {
		t.Fatal("listar arquivo devia falhar")
	}
	if _, err := fs.Read(d.Token, 0, 1); err == nil {
		t.Fatal("ler pasta devia falhar")
	}
	if _, err := fs.Read("t999", 0, 1); err == nil {
		t.Fatal("ficha inválida devia falhar")
	}
	fs.Release("o")
	if _, err := fs.Read(file.Token, 0, 1); err == nil {
		t.Fatal("ficha liberada devia falhar")
	}
}

func TestDownload(t *testing.T) {
	root := t.TempDir()
	fs := New()
	d, _ := fs.AddDir("o", root)
	sub, err := fs.Mkdir(d.Token, "../fora")
	if err != nil || sub.Name != "fora" {
		t.Fatal(sub, err)
	}
	if _, err := fs.Mkdir(d.Token, "fora"); err != nil { // já existe: ok
		t.Fatal(err)
	}
	h, err := fs.OpenWrite(sub.Token, "../../x.txt", true)
	if err != nil {
		t.Fatal(err)
	}
	fs.Write(h, []byte("abc"))
	fs.Write(h, []byte("def"))
	if err := fs.CloseWrite(h); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "fora", "x.txt"))
	if err != nil || string(got) != "abcdef" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "x.txt")); err == nil {
		t.Fatal("escapou da pasta")
	}
	// nome igual a pasta
	if _, err := fs.OpenWrite(d.Token, "fora", true); err == nil {
		t.Fatal("devia recusar gravar sobre pasta")
	}
	// atalho no destino
	os.Symlink(filepath.Join(root, "fora", "x.txt"), filepath.Join(root, "atalho"))
	if _, err := fs.OpenWrite(d.Token, "atalho", true); err == nil {
		t.Fatal("devia recusar atalho")
	}
	// Release fecha o que ficou aberto
	h2, _ := fs.OpenWrite(d.Token, "y", true)
	fs.Release("o")
	if err := fs.Write(h2, []byte("z")); err == nil {
		t.Fatal("escrita após Release devia falhar")
	}
}
