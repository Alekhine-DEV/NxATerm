package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"nxaterm/internal/xfer"
)

// Painel de arquivos locais (o "outro lado" das transferências).

// LocalHome devolve a pasta pessoal do usuário.
func (a *TermService) LocalHome() string {
	h, _ := os.UserHomeDir()
	return h
}

// LocalDrives lista as raízes: letras de unidade no Windows, "/" nos outros.
func (a *TermService) LocalDrives() []string {
	if runtime.GOOS != "windows" {
		return []string{"/"}
	}
	var out []string
	for c := 'A'; c <= 'Z'; c++ {
		root := string(c) + `:\`
		if _, err := os.Stat(root); err == nil {
			out = append(out, root)
		}
	}
	return out
}

// LocalList lista uma pasta do computador. Com path vazio, abre a pasta pessoal.
func (a *TermService) LocalList(path string) (DirListing, error) {
	if path == "" {
		path = a.LocalHome()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return DirListing{}, err
	}
	list, err := xfer.Local{}.List(abs)
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

// LocalMkdir cria uma pasta local.
func (a *TermService) LocalMkdir(path string) error { return os.MkdirAll(path, 0o755) }

// LocalCreateFile cria um arquivo local vazio; falha se já existir.
func (a *TermService) LocalCreateFile(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// LocalRename renomeia um item local.
func (a *TermService) LocalRename(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return errors.New("já existe um item com esse nome")
	}
	return os.Rename(from, to)
}

// LocalRemove apaga um item local (pastas com todo o conteúdo). O frontend confirma antes.
func (a *TermService) LocalRemove(path string) error {
	if filepath.Dir(path) == path {
		return errors.New("não é possível apagar a raiz")
	}
	return os.RemoveAll(path)
}

// LocalOpen abre o arquivo com o programa padrão do sistema.
func (a *TermService) LocalOpen(path string) error { return openPath(path) }

// LocalShow mostra o item no gerenciador de arquivos do sistema.
func (a *TermService) LocalShow(path string) error {
	switch runtime.GOOS {
	case "windows":
		return runDetached("explorer.exe", "/select,"+path)
	case "darwin":
		return runDetached("open", "-R", path)
	default:
		return openPath(filepath.Dir(path))
	}
}

func openPath(p string) error {
	if strings.TrimSpace(p) == "" {
		return errors.New("caminho vazio")
	}
	switch runtime.GOOS {
	case "windows":
		return runDetached("rundll32", "url.dll,FileProtocolHandler", p)
	case "darwin":
		return runDetached("open", p)
	default:
		return runDetached("xdg-open", p)
	}
}

func runDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
