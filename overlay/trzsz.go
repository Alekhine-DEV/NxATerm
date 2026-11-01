package main

import (
	"encoding/base64"

	"nxaterm/internal/tzfs"
)

// Arquivos do Trzsz (trz/tsz). O protocolo roda no frontend (biblioteca trzsz); aqui ficam o
// diálogo de escolha e o acesso ao disco, por fichas (internal/tzfs).

var tzDisk = tzfs.New()

// TzPickFiles pergunta quais arquivos enviar. Lista vazia = o usuário cancelou.
func (a *TermService) TzPickFiles(owner string) ([]tzfs.Node, error) {
	tzDisk.Release(owner)
	paths, err := a.app.Dialog.OpenFile().SetTitle(tr("Escolha os arquivos para enviar (Trzsz)")).PromptForMultipleSelection()
	if err != nil && !isCancel(err) {
		return nil, err
	}
	if err != nil || len(paths) == 0 {
		return nil, nil
	}
	var out []tzfs.Node
	for _, p := range paths {
		n, err := tzDisk.AddFile(owner, p)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out, nil
}

// TzPickDir pergunta uma pasta: para enviar (save=false) ou para guardar o que chegar (save=true).
// Ficha vazia = o usuário cancelou.
func (a *TermService) TzPickDir(owner string, save bool) (tzfs.Node, error) {
	tzDisk.Release(owner)
	title := tr("Escolha a pasta para enviar (Trzsz)")
	if save {
		title = tr("Salvar arquivos recebidos em (Trzsz)")
	}
	p, err := a.app.Dialog.OpenFile().SetTitle(title).
		CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(save).PromptForSingleSelection()
	if err != nil && !isCancel(err) {
		return tzfs.Node{}, err
	}
	if err != nil || p == "" {
		return tzfs.Node{}, nil
	}
	return tzDisk.AddDir(owner, p)
}

// TzList lista o conteúdo de uma pasta já escolhida.
func (a *TermService) TzList(token string) ([]tzfs.Node, error) { return tzDisk.List(token) }

// TzRead lê um pedaço (base64) de um arquivo escolhido. Texto vazio = fim.
func (a *TermService) TzRead(token string, off int64, n int) (string, error) {
	b, err := tzDisk.Read(token, off, n)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// TzMkdir cria (se faltar) uma subpasta dentro da pasta de destino.
func (a *TermService) TzMkdir(dirToken, name string) (tzfs.Node, error) {
	return tzDisk.Mkdir(dirToken, name)
}

// TzOpenWrite abre um arquivo de destino dentro da pasta; truncate zera o que já existia.
func (a *TermService) TzOpenWrite(dirToken, name string, truncate bool) (string, error) {
	return tzDisk.OpenWrite(dirToken, name, truncate)
}

// TzWrite grava um pedaço (base64) no arquivo de destino.
func (a *TermService) TzWrite(handle, b64 string) error {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	return tzDisk.Write(handle, data)
}

// TzCloseWrite fecha o arquivo de destino.
func (a *TermService) TzCloseWrite(handle string) error { return tzDisk.CloseWrite(handle) }

// TzRelease descarta as fichas de uma aba ao fim da transferência.
func (a *TermService) TzRelease(owner string) { tzDisk.Release(owner) }
