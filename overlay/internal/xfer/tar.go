package xfer

// Transferência de pastas com muitos arquivos por um único fluxo tar.
//
// Copiar arquivo por arquivo pelo SFTP custa várias idas e voltas ao servidor
// por arquivo (abrir, ler, fechar, listar a pasta). Numa pasta com dezenas de
// milhares de arquivos pequenos (uma release de Magento, node_modules) isso é
// muito mais lento que os bytes em si e castiga o disco do servidor. Aqui o
// servidor roda `tar` e o conteúdo vem (ou vai) por um canal só.

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MinTarFiles é a quantidade de arquivos a partir da qual o tar compensa.
const MinTarFiles = 50

// Exec roda um comando no servidor (normalmente numa sessão SSH). Deve devolver
// *ExitError quando o comando termina com código diferente de zero.
type Exec func(ctx context.Context, cmd string, stdin io.Reader, stdout, stderr io.Writer) error

// ExitError é o código de saída de um comando remoto.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("o comando terminou com código %d", e.Code) }

// shQuote põe s entre aspas simples para o shell.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// TarProbe conta os arquivos e bytes de uma pasta remota com um único comando
// no servidor, sem percorrê-la pelo SFTP. ok=false se o servidor não suporta
// (sem tar, ou find sem -printf): quem chama usa então o método arquivo a arquivo.
func TarProbe(ctx context.Context, ex Exec, dir string) (files int, bytes int64, ok bool, err error) {
	script := "command -v tar >/dev/null 2>&1 || exit 127; " +
		"find " + shQuote(dir) + " -type f -printf '%s\\n' 2>/dev/null | awk '{s+=$1;n++} END{print n+0, s+0}'"
	var out strings.Builder
	err = ex(ctx, "sh -c "+shQuote(script), nil, &out, io.Discard)
	if err != nil {
		var ee *ExitError
		if errors.As(err, &ee) {
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}
	if _, serr := fmt.Sscanf(strings.TrimSpace(out.String()), "%d %d", &files, &bytes); serr != nil || files <= 0 {
		return 0, 0, false, nil
	}
	return files, bytes, true, nil
}

// HasTar diz se o servidor tem o comando tar.
func HasTar(ctx context.Context, ex Exec) (bool, error) {
	err := ex(ctx, "sh -c "+shQuote("command -v tar >/dev/null 2>&1"), nil, io.Discard, io.Discard)
	var ee *ExitError
	if errors.As(err, &ee) {
		return false, nil
	}
	return err == nil, err
}

// safeRel devolve o caminho relativo seguro de uma entrada do tar dentro da
// pasta base, ou ok=false se a entrada não pertence a ela ou tenta escapar.
func safeRel(name, base string) (rel string, ok bool) {
	name = path.Clean(strings.TrimPrefix(name, "./"))
	if name == base {
		return "", true
	}
	if !strings.HasPrefix(name, base+"/") {
		return "", false
	}
	rel = strings.TrimPrefix(name, base+"/")
	for _, p := range strings.Split(rel, "/") {
		if p == ".." || p == "" {
			return "", false
		}
	}
	return rel, true
}

// ctxReader interrompe a leitura quando o contexto é cancelado.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// countingWriter soma os bytes escritos.
type countingWriter struct {
	w io.Writer
	n *int64
}

func (c countingWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	*c.n += int64(n)
	return n, err
}

// countingReader soma os bytes lidos.
type countingReader struct {
	r io.Reader
	n *int64
}

func (c countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	*c.n += int64(n)
	return n, err
}

// firstLine devolve a primeira linha não vazia de s.
func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// lockedBuf é um buffer seguro (e limitado) para o stderr de um comando remoto.
type lockedBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.b.Len() < 8192 {
		l.b.Write(p)
	}
	return len(p), nil
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// remoteWarn transforma o código de saída do tar (depois de um fluxo completo)
// numa mensagem; nil se terminou bem.
func remoteWarn(exErr error, stderr *lockedBuf) (error, bool) {
	if exErr == nil {
		return nil, true
	}
	var ee *ExitError
	if errors.As(exErr, &ee) {
		msg := firstLine(stderr.String())
		if msg == "" {
			msg = exErr.Error()
		}
		return errors.New(msg), true
	}
	return exErr, false
}

// TarDownload baixa a pasta remota remoteDir para localDir (que ainda não deve
// existir). total é o tamanho esperado em bytes, só para o progresso.
func TarDownload(ctx context.Context, ex Exec, remoteDir, localDir string, total int64, progress func(done, total int64, cur string)) (Stats, error) {
	var st Stats
	remoteDir = strings.TrimRight(remoteDir, "/")
	parent, base := path.Dir(remoteDir), path.Base(remoteDir)
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return st, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	pr, pw := io.Pipe()
	var stderr lockedBuf
	exDone := make(chan error, 1)
	go func() {
		err := ex(ctx, "tar -C "+shQuote(parent)+" -cf - "+shQuote(base), nil, pw, &stderr)
		pw.CloseWithError(err) // nil vira io.EOF para o leitor
		exDone <- err
	}()

	var done int64
	report := func(cur string) {
		if progress != nil {
			progress(done, total, cur)
		}
	}
	var first error
	fail := func(err error) {
		st.Failed++
		if first == nil {
			first = err
		}
	}

	tr := tar.NewReader(ctxReader{ctx, pr})
	var readErr error
loop:
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err != io.EOF {
				readErr = err
			}
			break
		}
		rel, ok := safeRel(hdr.Name, base)
		if !ok {
			st.Skipped++
			continue
		}
		dest := filepath.Join(localDir, filepath.FromSlash(rel))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				fail(fmt.Errorf("%s: %w", rel, err))
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				fail(fmt.Errorf("%s: %w", rel, err))
				continue
			}
			f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(hdr.Mode&0o777)|0o600)
			if err != nil {
				fail(fmt.Errorf("%s: %w", rel, err))
				continue
			}
			report(rel)
			_, cerr := io.Copy(countingWriter{f, &done}, tr)
			if err := f.Close(); cerr == nil {
				cerr = err
			}
			if cerr != nil {
				if ctx.Err() != nil || errors.Is(cerr, io.ErrUnexpectedEOF) {
					readErr = cerr
					break loop
				}
				fail(fmt.Errorf("%s: %w", rel, cerr))
				continue
			}
			os.Chtimes(dest, time.Now(), hdr.ModTime)
			st.Files++
			st.Bytes += hdr.Size
		default:
			st.Skipped++ // links simbólicos e especiais não são seguidos
		}
	}
	if readErr != nil {
		cancel()
		pr.Close()
		exErr := <-exDone
		if err := ctx.Err(); err != nil {
			return st, err
		}
		if msg := firstLine(stderr.String()); msg != "" {
			return st, errors.New(msg)
		}
		if exErr != nil {
			return st, exErr
		}
		return st, readErr
	}
	io.Copy(io.Discard, ctxReader{ctx, pr}) // o tar completa o último bloco com zeros; sem ler isso o comando não termina
	warn, fatal := remoteWarn(<-exDone, &stderr)
	if err := ctx.Err(); err != nil {
		return st, err
	}
	if !fatal {
		return st, warn
	}
	if warn != nil {
		fail(warn) // tar terminou o fluxo mas avisou (arquivo sem permissão, que mudou…)
	}
	if st.Failed > 0 {
		return st, fmt.Errorf("%d item(ns) com erro; o primeiro: %w", st.Failed, first)
	}
	report("")
	return st, nil
}

// LocalScan conta os arquivos e bytes de uma pasta local (sem seguir links).
func LocalScan(dir string) (files int, bytes int64, err error) {
	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				files++
				bytes += fi.Size()
			}
		}
		return nil
	})
	return
}

// TarUpload envia a pasta local localDir para remoteParent/name por um fluxo
// tar. A pasta remota não deve existir ainda.
func TarUpload(ctx context.Context, ex Exec, localDir, remoteParent, name string, total int64, progress func(done, total int64, cur string)) (Stats, error) {
	var st Stats
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	pr, pw := io.Pipe()
	var stderr lockedBuf
	script := "command -v tar >/dev/null 2>&1 || exit 127; O=; " +
		"if tar --help 2>&1 | grep -q -e --no-same-owner; then O=--no-same-owner; fi; " +
		"exec tar $O -C " + shQuote(remoteParent) + " -xf -"
	exDone := make(chan error, 1)
	go func() {
		err := ex(ctx, "sh -c "+shQuote(script), pr, nil, &stderr)
		pr.CloseWithError(errors.New("o servidor parou de receber")) // destrava o escritor
		exDone <- err
	}()

	var done int64
	report := func(cur string) {
		if progress != nil {
			progress(done, total, cur)
		}
	}
	sendFile := func(tw *tar.Writer, p, rel, tname string) error {
		f, err := os.Open(p)
		if err != nil {
			st.Failed++
			return nil
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			st.Failed++
			return nil
		}
		h, _ := tar.FileInfoHeader(fi, "")
		h.Name, h.Uid, h.Gid, h.Uname, h.Gname = tname, 0, 0, "", ""
		h.Mode &= 0o777
		report(rel)
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		n, err := io.Copy(tw, countingReader{ctxReader{ctx, f}, &done})
		if err != nil {
			return err
		}
		st.Files++
		st.Bytes += n
		return nil
	}
	werr := func() error {
		tw := tar.NewWriter(pw)
		err := filepath.WalkDir(localDir, func(p string, d os.DirEntry, err error) error {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			if err != nil {
				st.Failed++
				return nil
			}
			rel, rerr := filepath.Rel(localDir, p)
			if rerr != nil {
				return rerr
			}
			tname := name
			if rel != "." {
				tname = name + "/" + filepath.ToSlash(rel)
			}
			switch {
			case d.IsDir():
				fi, ierr := d.Info()
				if ierr != nil {
					return nil
				}
				h, _ := tar.FileInfoHeader(fi, "")
				h.Name, h.Uid, h.Gid, h.Uname, h.Gname = tname+"/", 0, 0, "", ""
				h.Mode = 0o755
				return tw.WriteHeader(h)
			case d.Type().IsRegular():
				return sendFile(tw, p, rel, tname)
			default:
				st.Skipped++ // links simbólicos e especiais não são seguidos
				return nil
			}
		})
		if err != nil {
			return err
		}
		return tw.Close()
	}()
	if werr != nil {
		pw.CloseWithError(werr)
	} else {
		pw.Close()
	}
	exErr := <-exDone
	if err := ctx.Err(); err != nil {
		return st, err
	}
	if werr != nil {
		if msg := firstLine(stderr.String()); msg != "" {
			return st, errors.New(msg)
		}
		return st, werr
	}
	warn, fatal := remoteWarn(exErr, &stderr)
	if !fatal || warn != nil {
		return st, warn
	}
	if st.Failed > 0 {
		return st, fmt.Errorf("%d item(ns) não puderam ser lidos", st.Failed)
	}
	report("")
	return st, nil
}
