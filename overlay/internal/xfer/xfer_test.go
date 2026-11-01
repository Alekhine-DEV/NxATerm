package xfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCopyTree(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "a.txt"), "aaa")
	write(t, filepath.Join(src, "sub", "b.txt"), "bbbb")
	write(t, filepath.Join(src, "sub", "deep", "c.txt"), "c")
	os.MkdirAll(filepath.Join(src, "vazia"), 0o755)

	var lastDone, lastTotal int64
	st, err := Copy(context.Background(), Local{}, Local{}, src, filepath.Join(dst, "out"), Options{
		Policy:   Overwrite,
		Progress: func(d, tt int64, _ string) { lastDone, lastTotal = d, tt },
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.Files != 3 || st.Bytes != 8 {
		t.Fatalf("stats: %+v", st)
	}
	if lastTotal != 8 || lastDone != 8 {
		t.Fatalf("progresso %d/%d", lastDone, lastTotal)
	}
	if read(t, filepath.Join(dst, "out", "sub", "deep", "c.txt")) != "c" {
		t.Fatal("conteúdo")
	}
	if fi, err := os.Stat(filepath.Join(dst, "out", "vazia")); err != nil || !fi.IsDir() {
		t.Fatal("pasta vazia deveria existir")
	}
}

func TestConflictPolicies(t *testing.T) {
	for _, tc := range []struct {
		pol, want string
		extra     string // conteúdo de "f (1).txt", se esperado
	}{
		{Overwrite, "novo", ""},
		{Skip, "velho", ""},
		{Rename, "velho", "novo"},
	} {
		src, dst := t.TempDir(), t.TempDir()
		write(t, filepath.Join(src, "f.txt"), "novo")
		write(t, filepath.Join(dst, "f.txt"), "velho")
		_, err := Copy(context.Background(), Local{}, Local{}, filepath.Join(src, "f.txt"), filepath.Join(dst, "f.txt"), Options{Policy: tc.pol})
		if err != nil {
			t.Fatal(tc.pol, err)
		}
		if got := read(t, filepath.Join(dst, "f.txt")); got != tc.want {
			t.Errorf("%s: f.txt = %q, quer %q", tc.pol, got, tc.want)
		}
		if tc.extra != "" {
			if got := read(t, filepath.Join(dst, "f (1).txt")); got != tc.extra {
				t.Errorf("%s: f (1).txt = %q", tc.pol, got)
			}
		}
	}
}

func TestResume(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "f.bin"), "0123456789")
	write(t, filepath.Join(dst, "f.bin"), "0123")
	st, err := Copy(context.Background(), Local{}, Local{}, filepath.Join(src, "f.bin"), filepath.Join(dst, "f.bin"), Options{Policy: Resume})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dst, "f.bin")) != "0123456789" {
		t.Fatalf("retomada: %q", read(t, filepath.Join(dst, "f.bin")))
	}
	if st.Bytes != 6 {
		t.Fatalf("deveria copiar só 6 bytes, copiou %d", st.Bytes)
	}
	// já completo: pula
	st, _ = Copy(context.Background(), Local{}, Local{}, filepath.Join(src, "f.bin"), filepath.Join(dst, "f.bin"), Options{Policy: Resume})
	if st.Skipped != 1 || st.Files != 0 {
		t.Fatalf("esperava pular: %+v", st)
	}
}

func TestAskAll(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	for _, n := range []string{"1", "2", "3"} {
		write(t, filepath.Join(src, n), "novo"+n)
		write(t, filepath.Join(dst, n), "velho")
	}
	calls := 0
	_, err := Copy(context.Background(), Local{}, Local{}, src, dst, Options{
		Policy: Ask,
		Ask: func(c Conflict) (string, bool, error) {
			calls++
			return Overwrite, true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("perguntou %d vezes, esperava 1 (aplicar a todos)", calls)
	}
	if read(t, filepath.Join(dst, "3")) != "novo3" {
		t.Fatal("não sobrescreveu")
	}
}

func TestAskCancel(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "f"), "x")
	write(t, filepath.Join(dst, "f"), "y")
	_, err := Copy(context.Background(), Local{}, Local{}, filepath.Join(src, "f"), filepath.Join(dst, "f"), Options{
		Ask: func(Conflict) (string, bool, error) { return Cancel, false, nil },
	})
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("erro = %v", err)
	}
	if read(t, filepath.Join(dst, "f")) != "y" {
		t.Fatal("não deveria tocar no destino")
	}
}

func TestContextCancel(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "f"), strings.Repeat("x", 1<<20))
	ctx, cancel := context.WithCancel(context.Background())
	_, err := Copy(ctx, Local{}, Local{}, filepath.Join(src, "f"), filepath.Join(dst, "f"), Options{
		Policy:  Overwrite,
		BufSize: 4096,
		Progress: func(d, _ int64, _ string) {
			if d > 10000 {
				cancel()
			}
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v", err)
	}
	fi, _ := os.Stat(filepath.Join(dst, "f"))
	if fi == nil || fi.Size() >= 1<<20 {
		t.Fatal("deveria ter parado no meio")
	}
}

func TestFailuresKeepGoing(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("depende de permissões do Unix")
	}
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "a"), "a")
	write(t, filepath.Join(src, "b"), "b")
	os.Chmod(filepath.Join(src, "a"), 0)
	st, err := Copy(context.Background(), Local{}, Local{}, src, dst, Options{Policy: Overwrite})
	if err == nil || st.Failed != 1 || st.Files != 1 {
		t.Fatalf("st=%+v err=%v", st, err)
	}
	if read(t, filepath.Join(dst, "b")) != "b" {
		t.Fatal("deveria seguir copiando os outros")
	}
}

func TestSymlinkDirSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("links no Windows exigem privilégio")
	}
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "d", "x"), "x")
	os.Symlink(src, filepath.Join(src, "d", "laco")) // aponta para o ancestral
	st, err := Copy(context.Background(), Local{}, Local{}, src, filepath.Join(dst, "o"), Options{Policy: Overwrite})
	if err != nil {
		t.Fatal(err)
	}
	if st.Files != 1 || st.Skipped != 1 {
		t.Fatalf("%+v", st)
	}
}

func TestUniqueName(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a.tar.gz"), "")
	write(t, filepath.Join(d, "a.tar (1).gz"), "")
	write(t, filepath.Join(d, ".bashrc"), "")
	got, _ := UniqueName(Local{}, filepath.Join(d, "a.tar.gz"))
	if filepath.Base(got) != "a.tar (2).gz" {
		t.Fatal(got)
	}
	got, _ = UniqueName(Local{}, filepath.Join(d, ".bashrc"))
	if filepath.Base(got) != ".bashrc (1)" {
		t.Fatal(got)
	}
}

func TestRemoveAllAndSize(t *testing.T) {
	d := t.TempDir()
	root := filepath.Join(d, "r")
	write(t, filepath.Join(root, "a"), "12")
	write(t, filepath.Join(root, "s", "b"), "345")
	ds, err := Size(context.Background(), Local{}, root)
	if err != nil || ds.Files != 2 || ds.Dirs != 1 || ds.Bytes != 5 {
		t.Fatalf("%+v %v", ds, err)
	}
	e, _ := Local{}.Stat(root)
	if err := RemoveAll(context.Background(), Local{}, root, e); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("deveria ter sumido")
	}
}

func TestRemoveAllDoesNotFollowLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	d := t.TempDir()
	keep := filepath.Join(d, "keep")
	write(t, filepath.Join(keep, "importante"), "!")
	root := filepath.Join(d, "r")
	os.MkdirAll(root, 0o755)
	os.Symlink(keep, filepath.Join(root, "ln"))
	e, _ := Local{}.Stat(root)
	if err := RemoveAll(context.Background(), Local{}, root, e); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(keep, "importante")) != "!" {
		t.Fatal("apagou o conteúdo do destino do link!")
	}
}
