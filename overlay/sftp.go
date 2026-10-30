package main

import (
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/pkg/sftp"
)

// isCancel diz se o erro de um diálogo de arquivo significa "o usuário
// cancelou" (a documentação do Wails não define se o cancelamento volta como
// erro ou como caminho vazio, então tratamos os dois casos).
func isCancel(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "cancel")
}

// FileEntry é um item da listagem SFTP.
type FileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	Mode    string `json:"mode"`
	ModTime int64  `json:"modTime"` // unix, segundos
}

// DirListing é o resultado de SFTPList.
type DirListing struct {
	Path    string      `json:"path"` // caminho absoluto já resolvido
	Entries []FileEntry `json:"entries"`
}

// SFTPList lista um diretório remoto. Com path vazio, abre a pasta inicial.
func (a *TermService) SFTPList(id, path string) (DirListing, error) {
	t, err := a.term(id)
	if err != nil {
		return DirListing{}, err
	}
	c, err := t.sftpClient()
	if err != nil {
		return DirListing{}, err
	}
	if path == "" {
		path = "."
	}
	abs, err := c.RealPath(path)
	if err != nil {
		return DirListing{}, err
	}
	infos, err := c.ReadDir(abs)
	if err != nil {
		return DirListing{}, err
	}
	res := DirListing{Path: abs, Entries: make([]FileEntry, 0, len(infos))}
	for _, fi := range infos {
		res.Entries = append(res.Entries, FileEntry{
			Name:    fi.Name(),
			Size:    fi.Size(),
			IsDir:   fi.IsDir(),
			Mode:    fi.Mode().String(),
			ModTime: fi.ModTime().Unix(),
		})
	}
	// pastas primeiro, depois por nome
	sort.SliceStable(res.Entries, func(i, j int) bool {
		x, y := res.Entries[i], res.Entries[j]
		if x.IsDir != y.IsDir {
			return x.IsDir
		}
		return strings.ToLower(x.Name) < strings.ToLower(y.Name)
	})
	return res, nil
}

// SFTPDownload pergunta onde salvar e baixa o arquivo remoto.
// Devolve o caminho local salvo (vazio se o usuário cancelou).
func (a *TermService) SFTPDownload(id, remotePath string) (string, error) {
	t, err := a.term(id)
	if err != nil {
		return "", err
	}
	c, err := t.sftpClient()
	if err != nil {
		return "", err
	}
	local, err := a.app.Dialog.SaveFile().
		SetFilename(pathpkg.Base(remotePath)).
		PromptForSingleSelection()
	if err != nil && !isCancel(err) {
		return "", err
	}
	if err != nil || local == "" {
		return "", nil // o usuário cancelou
	}
	src, err := c.Open(remotePath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.Create(local)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(local)
		return "", fmt.Errorf("download interrompido: %w", err)
	}
	return local, dst.Close()
}

// SFTPUpload pergunta quais arquivos enviar e os coloca em remoteDir.
// Devolve quantos arquivos foram enviados.
func (a *TermService) SFTPUpload(id, remoteDir string) (int, error) {
	t, err := a.term(id)
	if err != nil {
		return 0, err
	}
	c, err := t.sftpClient()
	if err != nil {
		return 0, err
	}
	files, err := a.app.Dialog.OpenFile().
		SetTitle("Escolha os arquivos para enviar").
		PromptForMultipleSelection()
	if err != nil && !isCancel(err) {
		return 0, err
	}
	if err != nil || len(files) == 0 {
		return 0, nil // o usuário cancelou
	}
	sent := 0
	for _, local := range files {
		if err := uploadOne(c, local, pathpkg.Join(remoteDir, baseName(local))); err != nil {
			return sent, fmt.Errorf("falha ao enviar %s: %w", local, err)
		}
		sent++
	}
	return sent, nil
}

// uploadOne copia um arquivo local para o caminho remoto, sobrescrevendo se existir.
func uploadOne(c *sftp.Client, local, remote string) error {
	src, err := os.Open(local)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := c.Create(remote)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

// SFTPMkdir cria uma pasta remota.
func (a *TermService) SFTPMkdir(id, path string) error {
	t, err := a.term(id)
	if err != nil {
		return err
	}
	c, err := t.sftpClient()
	if err != nil {
		return err
	}
	return c.Mkdir(path)
}

// SFTPRemove apaga um arquivo ou uma pasta VAZIA no servidor.
// O frontend sempre pede confirmação antes de chamar.
func (a *TermService) SFTPRemove(id, path string, isDir bool) error {
	t, err := a.term(id)
	if err != nil {
		return err
	}
	c, err := t.sftpClient()
	if err != nil {
		return err
	}
	if isDir {
		return c.RemoveDirectory(path)
	}
	return c.Remove(path)
}

// SFTPRename renomeia/move um item remoto.
func (a *TermService) SFTPRename(id, from, to string) error {
	t, err := a.term(id)
	if err != nil {
		return err
	}
	c, err := t.sftpClient()
	if err != nil {
		return err
	}
	return c.Rename(from, to)
}

// baseName pega o nome do arquivo local, aceitando "\" do Windows.
func baseName(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	return pathpkg.Base(p)
}
