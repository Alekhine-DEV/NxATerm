package main

import (
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"nxaterm/internal/store"
)

const maxBackground = 5 << 20 // imagem de fundo: até 5 MB

// TextFile é um arquivo de texto lido do disco.
type TextFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// ExportText mostra "Salvar como" e grava o texto (temas, listas). Devolve "" se cancelado.
func (a *TermService) ExportText(name, content string) (string, error) {
	return a.saveAs(name, []byte(content))
}

// ImportText pergunta qual arquivo abrir e devolve o texto (até 1 MB). Name vazio = cancelado.
func (a *TermService) ImportText(title string) (TextFile, error) {
	path, err := a.app.Dialog.OpenFile().SetTitle(title).PromptForSingleSelection()
	if err != nil && !isCancel(err) {
		return TextFile{}, err
	}
	if err != nil || path == "" {
		return TextFile{}, nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return TextFile{}, err
	}
	if st.Size() > 1<<20 {
		return TextFile{}, errors.New("arquivo grande demais")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return TextFile{}, err
	}
	return TextFile{Name: filepath.Base(path), Content: string(data)}, nil
}

func (a *TermService) backgroundPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "background.img"), nil
}

// PickBackground escolhe a imagem de fundo do terminal e a guarda na pasta de dados.
// Devolve a imagem como data URL ("" se o usuário cancelar).
func (a *TermService) PickBackground() (string, error) {
	path, err := a.app.Dialog.OpenFile().SetTitle(tr("Escolha a imagem de fundo")).PromptForSingleSelection()
	if err != nil && !isCancel(err) {
		return "", err
	}
	if err != nil || path == "" {
		return "", nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if st.Size() > maxBackground {
		return "", errors.New("a imagem passa de 5 MB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	url, err := imageDataURL(data)
	if err != nil {
		return "", err
	}
	dst, err := a.backgroundPath()
	if err != nil {
		return "", err
	}
	if err := store.WriteFileAtomic(dst, data); err != nil {
		return "", err
	}
	return url, nil
}

// GetBackground devolve a imagem de fundo guardada como data URL ("" se não houver).
func (a *TermService) GetBackground() string {
	p, err := a.backgroundPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	url, err := imageDataURL(data)
	if err != nil {
		return ""
	}
	return url
}

// ClearBackground apaga a imagem de fundo.
func (a *TermService) ClearBackground() error {
	p, err := a.backgroundPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func imageDataURL(data []byte) (string, error) {
	mime := http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
	default:
		// SVG não é detectado pelo conteúdo; aceita só o que tem cara de SVG
		if len(data) > 5 && (string(data[:5]) == "<?xml" || string(data[:4]) == "<svg") {
			mime = "image/svg+xml"
		} else {
			return "", errors.New("formato de imagem não suportado (use PNG, JPG, GIF, WebP, BMP ou SVG)")
		}
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
