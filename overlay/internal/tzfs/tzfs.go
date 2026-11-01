// Package tzfs dá ao Trzsz acesso a arquivos do disco por "fichas": o frontend nunca vê caminhos
// escolhidos pelo servidor, só fichas que o próprio programa distribuiu depois de o usuário escolher
// a pasta ou os arquivos.
package tzfs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Node é um arquivo ou pasta visível ao frontend.
type Node struct {
	Token string `json:"token"`
	Name  string `json:"name"`
	Kind  string `json:"kind"` // "file" ou "directory"
	Size  int64  `json:"size"`
}

type ent struct {
	path  string
	owner string
	dir   bool
}

type wfile struct {
	f     *os.File
	owner string
}

// FS guarda as fichas de uma transferência.
type FS struct {
	mu    sync.Mutex
	seq   int
	ents  map[string]ent
	files map[string]wfile
}

func New() *FS { return &FS{ents: map[string]ent{}, files: map[string]wfile{}} }

func (f *FS) add(owner, path string, dir bool) string {
	f.seq++
	tok := "t" + strconv.Itoa(f.seq)
	f.ents[tok] = ent{path: path, owner: owner, dir: dir}
	return tok
}

// AddFile registra um arquivo comum escolhido pelo usuário.
func (f *FS) AddFile(owner, path string) (Node, error) {
	st, err := os.Stat(path)
	if err != nil {
		return Node{}, err
	}
	if !st.Mode().IsRegular() {
		return Node{}, fmt.Errorf("%s não é um arquivo comum", filepath.Base(path))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return Node{Token: f.add(owner, path, false), Name: filepath.Base(path), Kind: "file", Size: st.Size()}, nil
}

// AddDir registra uma pasta escolhida pelo usuário.
func (f *FS) AddDir(owner, path string) (Node, error) {
	st, err := os.Stat(path)
	if err != nil {
		return Node{}, err
	}
	if !st.IsDir() {
		return Node{}, fmt.Errorf("%s não é uma pasta", filepath.Base(path))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = path
	}
	return Node{Token: f.add(owner, path, true), Name: name, Kind: "directory"}, nil
}

func (f *FS) get(token string, wantDir bool) (ent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.ents[token]
	if !ok {
		return ent{}, errors.New("ficha de arquivo inválida")
	}
	if e.dir != wantDir {
		if wantDir {
			return ent{}, errors.New("não é uma pasta")
		}
		return ent{}, errors.New("não é um arquivo")
	}
	return e, nil
}

// List devolve o conteúdo de uma pasta. Atalhos para pastas ficam de fora (evita laços).
func (f *FS) List(token string) ([]Node, error) {
	e, err := f.get(token, true)
	if err != nil {
		return nil, err
	}
	items, err := os.ReadDir(e.path)
	if err != nil {
		return nil, err
	}
	var out []Node
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, it := range items {
		p := filepath.Join(e.path, it.Name())
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if it.Type()&os.ModeSymlink != 0 && st.IsDir() {
			continue
		}
		switch {
		case st.IsDir():
			out = append(out, Node{Token: f.add(e.owner, p, true), Name: it.Name(), Kind: "directory"})
		case st.Mode().IsRegular():
			out = append(out, Node{Token: f.add(e.owner, p, false), Name: it.Name(), Kind: "file", Size: st.Size()})
		}
	}
	return out, nil
}

// Read lê até n bytes a partir de off. Vazio = fim do arquivo.
func (f *FS) Read(token string, off int64, n int) ([]byte, error) {
	e, err := f.get(token, false)
	if err != nil {
		return nil, err
	}
	if n <= 0 || n > 1<<20 {
		n = 1 << 20
	}
	fh, err := os.Open(e.path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	buf := make([]byte, n)
	got, err := fh.ReadAt(buf, off)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf[:got], nil
}

var reserved = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}

// SafeName deixa só um nome simples: o servidor controla esse texto e não pode escolher pastas
// nem sobrescrever nada fora da pasta escolhida.
func SafeName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = name[strings.LastIndex(name, "/")+1:]
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 0x7f || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimSpace(strings.TrimRight(name, ". "))
	if name == "" || name == "." || name == ".." {
		return "arquivo"
	}
	stem := strings.ToUpper(name)
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	if reserved[stem] || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9') {
		name = "_" + name
	}
	return name
}

func (f *FS) child(dirToken, name string) (ent, string, error) {
	e, err := f.get(dirToken, true)
	if err != nil {
		return ent{}, "", err
	}
	p := filepath.Join(e.path, SafeName(name))
	if st, err := os.Lstat(p); err == nil && st.Mode()&os.ModeSymlink != 0 {
		return ent{}, "", errors.New("o destino é um atalho; recusado")
	}
	return e, p, nil
}

// OpenWrite abre (cria) um arquivo dentro da pasta; truncate zera o que já existir.
func (f *FS) OpenWrite(dirToken, name string, truncate bool) (string, error) {
	e, p, err := f.child(dirToken, name)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return "", errors.New("já existe uma pasta com esse nome")
	}
	flag := os.O_WRONLY | os.O_CREATE
	if truncate {
		flag |= os.O_TRUNC
	}
	fh, err := os.OpenFile(p, flag, 0o644)
	if err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	h := "w" + strconv.Itoa(f.seq)
	f.files[h] = wfile{f: fh, owner: e.owner}
	return h, nil
}

func (f *FS) Write(handle string, data []byte) error {
	f.mu.Lock()
	w, ok := f.files[handle]
	f.mu.Unlock()
	if !ok {
		return errors.New("arquivo não está aberto")
	}
	_, err := w.f.Write(data)
	return err
}

func (f *FS) CloseWrite(handle string) error {
	f.mu.Lock()
	w, ok := f.files[handle]
	delete(f.files, handle)
	f.mu.Unlock()
	if !ok {
		return nil
	}
	return w.f.Close()
}

// Mkdir cria (se faltar) uma subpasta e devolve a ficha dela.
func (f *FS) Mkdir(dirToken, name string) (Node, error) {
	e, p, err := f.child(dirToken, name)
	if err != nil {
		return Node{}, err
	}
	if st, err := os.Stat(p); err == nil {
		if !st.IsDir() {
			return Node{}, errors.New("já existe um arquivo com esse nome")
		}
	} else if err := os.Mkdir(p, 0o755); err != nil {
		return Node{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return Node{Token: f.add(e.owner, p, true), Name: filepath.Base(p), Kind: "directory"}, nil
}

// Release descarta as fichas e fecha os arquivos de um dono (uma aba).
func (f *FS) Release(owner string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for t, e := range f.ents {
		if e.owner == owner {
			delete(f.ents, t)
		}
	}
	for h, w := range f.files {
		if w.owner == owner {
			w.f.Close()
			delete(f.files, h)
		}
	}
}
