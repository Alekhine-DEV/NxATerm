package xfer

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// localExec roda o comando no próprio computador, como se fosse o servidor.
func localExec(ctx context.Context, cmd string, stdin io.Reader, stdout, stderr io.Writer) error {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Stdin, c.Stdout, c.Stderr = stdin, stdout, stderr
	err := c.Run()
	if ee, ok := err.(*exec.ExitError); ok && ctx.Err() == nil {
		return &ExitError{Code: ee.ExitCode()}
	}
	return err
}

func needTar(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("usa sh, tar e find -printf do Linux")
	}
	for _, b := range []string{"sh", "tar", "find", "awk"} {
		if _, err := exec.LookPath(b); err != nil {
			t.Skip(b + " ausente")
		}
	}
}

func makeTree(t *testing.T, root string, n int) int64 {
	var total int64
	for i := 0; i < n; i++ {
		body := fmt.Sprintf("arquivo %d\n", i)
		write(t, filepath.Join(root, fmt.Sprintf("d%d", i%7), fmt.Sprintf("s%d", i%3), fmt.Sprintf("f%d.txt", i)), body)
		total += int64(len(body))
	}
	os.MkdirAll(filepath.Join(root, "vazia"), 0o755)
	write(t, filepath.Join(root, "com espaço", "it's.txt"), "x")
	total++
	return total
}

func TestTarProbe(t *testing.T) {
	needTar(t)
	src := filepath.Join(t.TempDir(), "rel 1")
	want := makeTree(t, src, 120)
	files, b, ok, err := TarProbe(context.Background(), localExec, src)
	if err != nil || !ok || files != 121 || b != want {
		t.Fatalf("files=%d bytes=%d (esperado %d) ok=%v err=%v", files, b, want, ok, err)
	}
	if _, _, ok, _ := TarProbe(context.Background(), localExec, filepath.Join(src, "nao-existe")); ok {
		t.Fatal("pasta inexistente não deveria dar ok")
	}
}

func TestTarDownloadUpload(t *testing.T) {
	needTar(t)
	root := t.TempDir()
	src := filepath.Join(root, "release 22")
	want := makeTree(t, src, 200)
	os.Symlink("d0", filepath.Join(src, "link")) // não deve ser seguido

	dst := filepath.Join(root, "baixado", "release 22")
	var lastDone, lastTotal int64
	st, err := TarDownload(context.Background(), localExec, src, dst, want, func(d, tt int64, _ string) { lastDone, lastTotal = d, tt })
	if err != nil {
		t.Fatal(err)
	}
	if st.Files != 201 || st.Bytes != want || st.Skipped != 1 {
		t.Fatalf("stats: %+v (bytes esperados %d)", st, want)
	}
	if lastDone != want || lastTotal != want {
		t.Fatalf("progresso %d/%d", lastDone, lastTotal)
	}
	if read(t, filepath.Join(dst, "com espaço", "it's.txt")) != "x" || read(t, filepath.Join(dst, "d3", "s0", "f3.txt")) != "arquivo 3\n" {
		t.Fatal("conteúdo")
	}
	if fi, err := os.Stat(filepath.Join(dst, "vazia")); err != nil || !fi.IsDir() {
		t.Fatal("pasta vazia deveria existir")
	}
	if _, err := os.Lstat(filepath.Join(dst, "link")); err == nil {
		t.Fatal("link simbólico não deveria ser copiado")
	}

	// envio: volta para outro lugar "remoto"
	remote := filepath.Join(root, "remoto")
	os.MkdirAll(remote, 0o755)
	lf, lb, _ := LocalScan(dst)
	if lf != 201 || lb != want {
		t.Fatalf("LocalScan %d %d", lf, lb)
	}
	lastDone = 0
	st, err = TarUpload(context.Background(), localExec, dst, remote, "release 22", lb, func(d, _ int64, _ string) { lastDone = d })
	if err != nil {
		t.Fatal(err)
	}
	if st.Files != 201 || lastDone != want {
		t.Fatalf("upload: %+v done=%d", st, lastDone)
	}
	if read(t, filepath.Join(remote, "release 22", "d6", "s0", "f6.txt")) != "arquivo 6\n" {
		t.Fatal("conteúdo do envio")
	}
}

func TestTarDownloadCancelAndErrors(t *testing.T) {
	needTar(t)
	root := t.TempDir()
	src := filepath.Join(root, "r")
	makeTree(t, src, 100)

	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	_, err := TarDownload(ctx, localExec, src, filepath.Join(root, "o1"), 0, func(_, _ int64, cur string) {
		if n++; n == 10 {
			cancel()
		}
	})
	if err != context.Canceled {
		t.Fatalf("esperava cancelamento, veio %v", err)
	}

	// pasta que não existe: tar falha e a mensagem do servidor vem no erro
	_, err = TarDownload(context.Background(), localExec, filepath.Join(root, "nada"), filepath.Join(root, "o2"), 0, nil)
	if err == nil {
		t.Fatal("deveria falhar")
	}

	// arquivo ilegível no meio: o resto é baixado e o erro é reportado
	if os.Geteuid() != 0 {
		os.Chmod(filepath.Join(src, "d1", "s1", "f1.txt"), 0)
		st, err := TarDownload(context.Background(), localExec, src, filepath.Join(root, "o3"), 0, nil)
		if err == nil || st.Failed != 1 || st.Files < 90 {
			t.Fatalf("parcial: %+v %v", st, err)
		}
	}
}

func TestSafeRel(t *testing.T) {
	for _, c := range []struct {
		in, rel string
		ok      bool
	}{
		{"rel", "", true}, {"rel/", "", true}, {"rel/a/b", "a/b", true}, {"./rel/a", "a", true},
		{"rel/../x", "", false}, {"outro/a", "", false}, {"rel/a/../../x", "", false}, {"/etc/passwd", "", false},
	} {
		rel, ok := safeRel(c.in, "rel")
		if rel != c.rel || ok != c.ok {
			t.Errorf("%q → %q,%v", c.in, rel, ok)
		}
	}
}
