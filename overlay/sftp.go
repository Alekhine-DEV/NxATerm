package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"sort"
	"strings"
	"time"

	"nxaterm/internal/xfer"

	"github.com/pkg/sftp"
)

// isCancel diz se o erro de um diálogo de arquivo significa "o usuário
// cancelou" (a documentação do Wails não define se o cancelamento volta como
// erro ou como caminho vazio, então tratamos os dois casos).
func isCancel(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "cancel")
}

// FileEntry é um item da listagem de uma pasta (remota ou local).
type FileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	IsLink  bool   `json:"link"`
	Mode    string `json:"mode"`
	Perm    int    `json:"perm"`    // bits de permissão (0o755 = 493)
	ModTime int64  `json:"modTime"` // unix, segundos
}

// DirListing é o resultado de SFTPList e LocalList.
type DirListing struct {
	Path    string      `json:"path"` // caminho absoluto já resolvido
	Entries []FileEntry `json:"entries"`
}

func toFileEntry(e xfer.Entry) FileEntry {
	fe := FileEntry{Name: e.Name, Size: e.Size, IsDir: e.IsDir, IsLink: e.IsLink, ModTime: e.ModTime.Unix()}
	if e.ModTime.IsZero() {
		fe.ModTime = 0
	}
	if e.Mode != 0 { // o FTP não informa permissões
		fe.Mode, fe.Perm = e.Mode.String(), int(e.Mode.Perm())
	}
	return fe
}

func sortEntries(list []FileEntry) {
	// pastas primeiro, depois por nome
	sort.SliceStable(list, func(i, j int) bool {
		x, y := list[i], list[j]
		if x.IsDir != y.IsDir {
			return x.IsDir
		}
		return strings.ToLower(x.Name) < strings.ToLower(y.Name)
	})
}

func (a *TermService) sftpOf(id string) (*sftp.Client, *termSession, error) {
	t, err := a.term(id)
	if err != nil {
		return nil, nil, err
	}
	c, err := t.sftpClient()
	return c, t, err
}

// fsOf devolve o sistema de arquivos remoto da aba (SFTP ou FTP).
func (a *TermService) fsOf(id string) (xfer.FS, *termSession, error) {
	t, err := a.term(id)
	if err != nil {
		return nil, nil, err
	}
	f, err := t.files()
	return f, t, err
}

type realPather interface {
	Real(p string) (string, error)
}

// SFTPList lista um diretório remoto (SFTP ou FTP). Com path vazio, abre a pasta inicial.
func (a *TermService) SFTPList(id, path string) (DirListing, error) {
	f, _, err := a.fsOf(id)
	if err != nil {
		return DirListing{}, err
	}
	abs := path
	if r, ok := f.(realPather); ok {
		if path == "" {
			path = "."
		}
		if abs, err = r.Real(path); err != nil {
			return DirListing{}, err
		}
	}
	list, err := f.List(abs)
	if err != nil {
		return DirListing{}, err
	}
	res := DirListing{Path: abs, Entries: make([]FileEntry, 0, len(list))}
	for _, e := range list {
		res.Entries = append(res.Entries, toFileEntry(e))
	}
	sortEntries(res.Entries)
	return res, nil
}

// SFTPMkdir cria uma pasta remota (e as que faltarem no caminho).
func (a *TermService) SFTPMkdir(id, path string) error {
	f, _, err := a.fsOf(id)
	if err != nil {
		return err
	}
	return xfer.MkdirAll(f, path)
}

// SFTPCreateFile cria um arquivo vazio; falha se já existir.
func (a *TermService) SFTPCreateFile(id, path string) error {
	f, _, err := a.fsOf(id)
	if err != nil {
		return err
	}
	if _, err := f.Stat(path); err == nil {
		return errors.New("já existe um item com esse nome")
	}
	w, err := f.Create(path, 0)
	if err != nil {
		return err
	}
	return w.Close()
}

// SFTPRemove apaga um arquivo ou uma pasta com todo o conteúdo (links são apagados
// sem seguir o destino). O frontend sempre pede confirmação antes de chamar.
func (a *TermService) SFTPRemove(id, path string) error {
	f, _, err := a.fsOf(id)
	if err != nil {
		return err
	}
	e, err := xfer.Lstat(f, path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	return xfer.RemoveAll(ctx, f, path, e)
}

// SFTPRename renomeia/move um item remoto.
func (a *TermService) SFTPRename(id, from, to string) error {
	f, _, err := a.fsOf(id)
	if err != nil {
		return err
	}
	if _, err := f.Stat(to); err == nil {
		return errors.New("já existe um item com esse nome")
	}
	return f.Rename(from, to)
}

// SFTPMove move itens para outra pasta do servidor.
func (a *TermService) SFTPMove(id string, from []string, toDir string) error {
	f, _, err := a.fsOf(id)
	if err != nil {
		return err
	}
	for _, p := range from {
		dst := f.Join(toDir, pathpkg.Base(p))
		if dst == p {
			continue
		}
		if _, err := f.Stat(dst); err == nil {
			return fmt.Errorf("já existe %q na pasta de destino", pathpkg.Base(p))
		}
		if err := f.Rename(p, dst); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

// SFTPCopy copia itens dentro do servidor (usa "cp -a" por SSH, então precisa de shell).
func (a *TermService) SFTPCopy(id string, from []string, toDir string) error {
	t, err := a.term(id)
	if err != nil {
		return err
	}
	client, err := t.sshClient()
	if err != nil {
		return err
	}
	if len(from) == 0 {
		return nil
	}
	args := make([]string, 0, len(from)+1)
	for _, p := range from {
		args = append(args, shellQuote(p))
	}
	args = append(args, shellQuote(strings.TrimSuffix(toDir, "/")+"/"))
	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	out, err := sess.CombinedOutput("cp -a -n -- " + strings.Join(args, " "))
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

// SFTPChmod muda as permissões (perm em decimal, ex.: 0o755 = 493). Com recursive,
// vale para tudo dentro da pasta.
func (a *TermService) SFTPChmod(id, path string, perm int, recursive bool) error {
	c, _, err := a.sftpOf(id)
	if err != nil {
		return err
	}
	if perm < 0 || perm > 0o7777 {
		return errors.New("permissão inválida")
	}
	mode := os.FileMode(perm & 0o777)
	if err := c.Chmod(path, mode); err != nil {
		return err
	}
	if !recursive {
		return nil
	}
	fi, err := c.Stat(path)
	if err != nil || !fi.IsDir() {
		return err
	}
	return chmodTree(c, path, mode, 0)
}

func chmodTree(c *sftp.Client, dir string, mode os.FileMode, depth int) error {
	if depth > 64 {
		return nil
	}
	list, err := sftpFS{c}.List(dir)
	if err != nil {
		return err
	}
	for _, e := range list {
		p := pathpkg.Join(dir, e.Name)
		if e.IsLink {
			continue
		}
		if err := c.Chmod(p, mode); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if e.IsDir {
			if err := chmodTree(c, p, mode, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// FileDetail são as propriedades de um item remoto.
type FileDetail struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	IsDir      bool   `json:"isDir"`
	IsLink     bool   `json:"link"`
	LinkTarget string `json:"linkTarget"`
	Mode       string `json:"mode"`
	Perm       int    `json:"perm"`
	ModTime    int64  `json:"modTime"`
	AccessTime int64  `json:"accessTime"`
	UID        uint32 `json:"uid"`
	GID        uint32 `json:"gid"`
}

// SFTPInfo devolve as propriedades de um item remoto.
func (a *TermService) SFTPInfo(id, path string) (FileDetail, error) {
	c, _, err := a.sftpOf(id)
	if err != nil {
		return FileDetail{}, err
	}
	fi, err := c.Lstat(path)
	if err != nil {
		return FileDetail{}, err
	}
	d := FileDetail{
		Path: path, Name: pathpkg.Base(path), Size: fi.Size(), IsDir: fi.IsDir(),
		Mode: fi.Mode().String(), Perm: int(fi.Mode().Perm()), ModTime: fi.ModTime().Unix(),
	}
	if st, ok := fi.Sys().(*sftp.FileStat); ok && st != nil {
		d.UID, d.GID = st.UID, st.GID
		d.AccessTime = int64(st.Atime)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		d.IsLink = true
		d.LinkTarget, _ = c.ReadLink(path)
		if st, err := c.Stat(path); err == nil {
			d.IsDir, d.Size = st.IsDir(), st.Size()
		}
	}
	return d, nil
}

// SFTPDirSize conta arquivos, pastas e bytes dentro de uma pasta remota.
func (a *TermService) SFTPDirSize(id, path string) (xfer.DirStat, error) {
	c, _, err := a.sftpOf(id)
	if err != nil {
		return xfer.DirStat{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return xfer.Size(ctx, sftpFS{c}, path)
}

// baseName pega o nome do arquivo local, aceitando "\" do Windows.
func baseName(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	return pathpkg.Base(p)
}
