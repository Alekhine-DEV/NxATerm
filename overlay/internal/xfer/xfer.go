// Package xfer é o motor de transferência de arquivos: copia arquivos e pastas
// (recursivamente) entre dois sistemas de arquivos quaisquer (disco local,
// SFTP, FTP), com progresso, cancelamento, retomada e tratamento de conflitos.
// Usa só a biblioteca padrão, então é testável sem rede.
package xfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"time"
)

// Entry descreve um item de uma pasta.
type Entry struct {
	Name    string
	Size    int64
	IsDir   bool // vale também para link simbólico que aponta para uma pasta
	IsLink  bool
	Mode    fs.FileMode
	ModTime time.Time
}

// FS é o que o motor precisa de um sistema de arquivos.
type FS interface {
	// List lista a pasta; para links, IsLink=true e IsDir/Size vêm do destino.
	List(dir string) ([]Entry, error)
	// Stat segue links. O erro de "não existe" deve satisfazer errors.Is(err, fs.ErrNotExist).
	Stat(p string) (Entry, error)
	// Open abre para leitura a partir de offset.
	Open(p string, offset int64) (io.ReadCloser, error)
	// Create abre para escrita; offset 0 recria o arquivo, offset > 0 continua dele.
	Create(p string, offset int64) (io.WriteCloser, error)
	Mkdir(p string) error
	Remove(p string) error
	RemoveDir(p string) error
	Rename(from, to string) error
	Join(elem ...string) string
}

// Políticas e ações para quando o destino já existe.
const (
	Ask       = "ask"
	Overwrite = "overwrite"
	Skip      = "skip"
	Rename    = "rename"
	Resume    = "resume"
	Cancel    = "cancel"
)

// ErrCanceled é devolvido quando o usuário cancela pelo diálogo de conflito.
var ErrCanceled = errors.New("transferência cancelada")

// Conflict descreve um destino que já existe.
type Conflict struct {
	Name    string
	Path    string
	SrcSize int64
	DstSize int64
	SrcTime time.Time
	DstTime time.Time
}

// Options configura Copy.
type Options struct {
	Policy   string                                              // Ask (padrão), Overwrite, Skip, Rename ou Resume
	Ask      func(Conflict) (action string, all bool, err error) // chamado quando Policy == Ask
	Progress func(done, total int64, current string)             // pode ser nil
	BufSize  int
}

// Stats resume o que foi copiado.
type Stats struct {
	Files   int
	Skipped int
	Bytes   int64
	Failed  int
}

const maxDepth = 64

type copier struct {
	ctx    context.Context
	src    FS
	dst    FS
	o      Options
	policy string
	total  int64
	done   int64
	st     Stats
	first  error
}

// Copy copia srcPath (arquivo ou pasta) para dstPath. dstPath é o caminho final
// do item (já com o nome), não a pasta que o contém.
func Copy(ctx context.Context, src, dst FS, srcPath, dstPath string, o Options) (Stats, error) {
	if o.BufSize <= 0 {
		o.BufSize = 256 * 1024
	}
	c := &copier{ctx: ctx, src: src, dst: dst, o: o, policy: o.Policy}
	if c.policy == "" {
		c.policy = Ask
	}
	se, err := src.Stat(srcPath)
	if err != nil {
		return c.st, err
	}
	c.total, err = c.measure(srcPath, se, 0)
	if err != nil {
		return c.st, err
	}
	c.report("")
	if err := c.entry(srcPath, dstPath, se, 0); err != nil {
		return c.st, err
	}
	if c.st.Failed > 0 {
		return c.st, fmt.Errorf("%d item(ns) com erro; o primeiro: %w", c.st.Failed, c.first)
	}
	return c.st, nil
}

func (c *copier) report(cur string) {
	if c.o.Progress != nil {
		c.o.Progress(c.done, c.total, cur)
	}
}

// measure soma o tamanho de tudo que será copiado.
func (c *copier) measure(p string, e Entry, depth int) (int64, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	if !e.IsDir {
		return e.Size, nil
	}
	if depth > maxDepth {
		return 0, nil
	}
	list, err := c.src.List(p)
	if err != nil {
		return 0, err
	}
	var sum int64
	for _, ch := range list {
		if ch.IsLink && ch.IsDir {
			continue
		}
		n, err := c.measure(c.src.Join(p, ch.Name), ch, depth+1)
		if err != nil {
			return 0, err
		}
		sum += n
	}
	return sum, nil
}

func (c *copier) fail(err error) {
	c.st.Failed++
	if c.first == nil {
		c.first = err
	}
}

func (c *copier) entry(sp, dp string, e Entry, depth int) error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	if e.IsDir {
		return c.dir(sp, dp, depth)
	}
	err := c.file(sp, dp, e)
	if err != nil {
		if c.ctx.Err() != nil || errors.Is(err, ErrCanceled) {
			return err
		}
		c.fail(fmt.Errorf("%s: %w", sp, err))
		c.done += e.Size // mantém a barra andando
		c.report(sp)
	}
	return nil
}

func (c *copier) dir(sp, dp string, depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("pastas aninhadas demais em %s", sp)
	}
	de, err := c.dst.Stat(dp)
	switch {
	case err == nil && !de.IsDir:
		c.fail(fmt.Errorf("%s: já existe um arquivo com o nome da pasta", dp))
		return nil
	case err != nil && errors.Is(err, fs.ErrNotExist):
		if err := c.dst.Mkdir(dp); err != nil {
			c.fail(fmt.Errorf("%s: %w", dp, err))
			return nil
		}
	case err != nil:
		return err
	}
	list, err := c.src.List(sp)
	if err != nil {
		c.fail(fmt.Errorf("%s: %w", sp, err))
		return nil
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	for _, ch := range list {
		if ch.IsLink && ch.IsDir {
			c.st.Skipped++ // não segue links de pasta (evita laços)
			continue
		}
		if err := c.entry(c.src.Join(sp, ch.Name), c.dst.Join(dp, ch.Name), ch, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (c *copier) resolve(dp string, se Entry, de Entry) (string, error) {
	pol := c.policy
	if pol == Ask {
		if c.o.Ask == nil {
			pol = Skip
		} else {
			act, all, err := c.o.Ask(Conflict{
				Name: baseOf(dp), Path: dp, SrcSize: se.Size, DstSize: de.Size, SrcTime: se.ModTime, DstTime: de.ModTime,
			})
			if err != nil {
				return "", err
			}
			if act == Cancel {
				return "", ErrCanceled
			}
			pol = act
			if all {
				c.policy = act
			}
		}
	}
	return pol, nil
}

func (c *copier) file(sp, dp string, se Entry) error {
	var offset int64
	de, err := c.dst.Stat(dp)
	switch {
	case err == nil && de.IsDir:
		return errors.New("já existe uma pasta com o nome do arquivo")
	case err == nil:
		pol, err := c.resolve(dp, se, de)
		if err != nil {
			return err
		}
		switch pol {
		case Skip:
			c.st.Skipped++
			c.done += se.Size
			c.report(sp)
			return nil
		case Rename:
			dp, err = UniqueName(c.dst, dp)
			if err != nil {
				return err
			}
		case Resume:
			switch {
			case de.Size == se.Size:
				c.st.Skipped++
				c.done += se.Size
				c.report(sp)
				return nil
			case de.Size < se.Size:
				offset = de.Size
			}
		case Overwrite:
		default:
			return fmt.Errorf("ação desconhecida: %q", pol)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}

	r, err := c.src.Open(sp, offset)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := c.dst.Create(dp, offset)
	if err != nil {
		return err
	}
	c.done += offset
	n, cerr := c.pump(w, r, sp)
	if err := w.Close(); cerr == nil {
		cerr = err
	}
	if cerr != nil {
		return cerr
	}
	c.st.Files++
	c.st.Bytes += n
	return nil
}

// pump copia r para w contando o progresso e respeitando o cancelamento.
// Usa WriterTo/ReaderFrom quando existem (o SFTP faz leituras e escritas em paralelo).
func (c *copier) pump(w io.Writer, r io.Reader, cur string) (int64, error) {
	cw := &countWriter{c: c, w: w, cur: cur}
	if wt, ok := r.(io.WriterTo); ok {
		return wt.WriteTo(cw)
	}
	if rf, ok := w.(io.ReaderFrom); ok {
		return rf.ReadFrom(&countReader{c: c, r: r, cur: cur})
	}
	buf := make([]byte, c.o.BufSize)
	return io.CopyBuffer(cw, struct{ io.Reader }{r}, buf)
}

type countWriter struct {
	c   *copier
	w   io.Writer
	cur string
}

func (cw *countWriter) Write(p []byte) (int, error) {
	if err := cw.c.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := cw.w.Write(p)
	cw.c.done += int64(n)
	cw.c.report(cw.cur)
	return n, err
}

type countReader struct {
	c   *copier
	r   io.Reader
	cur string
}

func (cr *countReader) Read(p []byte) (int, error) {
	if err := cr.c.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := cr.r.Read(p)
	cr.c.done += int64(n)
	cr.c.report(cr.cur)
	return n, err
}

func splitDir(p string) (dir, name string) {
	i := strings.LastIndexAny(p, `/\`)
	return p[:i+1], p[i+1:]
}

func baseOf(p string) string { _, n := splitDir(p); return n }

// UniqueName devolve p com " (1)", " (2)"... antes da extensão até não existir.
func UniqueName(f FS, p string) (string, error) {
	dir, name := splitDir(p)
	stem, ext := name, ""
	if i := strings.LastIndex(name, "."); i > 0 {
		stem, ext = name[:i], name[i:]
	}
	for n := 1; n < 10000; n++ {
		cand := fmt.Sprintf("%s%s (%d)%s", dir, stem, n, ext)
		if _, err := f.Stat(cand); errors.Is(err, fs.ErrNotExist) {
			return cand, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", errors.New("não foi possível achar um nome livre")
}

// RemoveAll apaga um item; pastas são esvaziadas antes. Links são apagados sem
// seguir o destino.
func RemoveAll(ctx context.Context, f FS, p string, e Entry) error {
	return removeAll(ctx, f, p, e, 0)
}

func removeAll(ctx context.Context, f FS, p string, e Entry, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !e.IsDir || e.IsLink {
		return f.Remove(p)
	}
	if depth > maxDepth {
		return fmt.Errorf("pastas aninhadas demais em %s", p)
	}
	list, err := f.List(p)
	if err != nil {
		return err
	}
	for _, ch := range list {
		if err := removeAll(ctx, f, f.Join(p, ch.Name), ch, depth+1); err != nil {
			return err
		}
	}
	return f.RemoveDir(p)
}

// DirStat é o resumo de uma pasta.
type DirStat struct {
	Files int   `json:"files"`
	Dirs  int   `json:"dirs"`
	Bytes int64 `json:"bytes"`
}

// Size conta arquivos, pastas e bytes dentro de p (sem seguir links).
func Size(ctx context.Context, f FS, p string) (DirStat, error) {
	var ds DirStat
	err := walk(ctx, f, p, &ds, 0)
	return ds, err
}

func walk(ctx context.Context, f FS, p string, ds *DirStat, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > maxDepth {
		return nil
	}
	list, err := f.List(p)
	if err != nil {
		return err
	}
	for _, ch := range list {
		if ch.IsDir && !ch.IsLink {
			ds.Dirs++
			if err := walk(ctx, f, f.Join(p, ch.Name), ds, depth+1); err != nil {
				return err
			}
			continue
		}
		ds.Files++
		ds.Bytes += ch.Size
	}
	return nil
}
