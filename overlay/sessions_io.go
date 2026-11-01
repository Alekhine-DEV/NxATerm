package main

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"nxaterm/internal/store"
)

// ImportResult é o resumo de uma importação.
type ImportResult struct {
	Added   int    `json:"added"`   // sessões novas
	Skipped int    `json:"skipped"` // já existiam ou são de um tipo sem suporte
	Folders int    `json:"folders"` // pastas encontradas no arquivo
	Path    string `json:"path"`    // arquivo lido ("" se o usuário cancelou)
}

// ---------------------------------------------------------------- pastas

// ListFolders devolve as pastas criadas pelo usuário (as das sessões vêm nas próprias sessões).
func (a *TermService) ListFolders() []string { return a.store.Folders() }

// CreateFolder cria uma pasta, mesmo vazia. Use "/" para subpastas.
func (a *TermService) CreateFolder(path string) error { return a.store.CreateFolder(path) }

// RenameFolder renomeia/move uma pasta com tudo o que há dentro.
func (a *TermService) RenameFolder(from, to string) error { return a.store.RenameFolder(from, to) }

// DeleteFolder apaga uma pasta. withSessions=false mantém as sessões (sobem um nível).
// Devolve quantas sessões foram apagadas.
func (a *TermService) DeleteFolder(path string, withSessions bool) (int, error) {
	return a.store.DeleteFolder(path, withSessions)
}

// ---------------------------------------------------------------- exportar / importar

// ExportSessions pergunta onde salvar e grava todas as sessões e pastas em JSON.
// Devolve o caminho salvo (vazio se o usuário cancelou). Senhas nunca existem no arquivo.
func (a *TermService) ExportSessions() (string, error) {
	data, err := a.store.Export()
	if err != nil {
		return "", err
	}
	return a.saveAs("nxaterm-sessoes.json", data)
}

// saveAs mostra o diálogo "Salvar como" e grava data. Devolve "" se cancelado.
func (a *TermService) saveAs(name string, data []byte) (string, error) {
	path, err := a.app.Dialog.SaveFile().SetFilename(name).PromptForSingleSelection()
	if err != nil && !isCancel(err) {
		return "", err
	}
	if err != nil || path == "" {
		return "", nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ImportSessionsFile pergunta qual arquivo abrir e descobre o formato sozinho:
// exportação do NxATerm (.json), MobaXterm (.mxtsessions / MobaXterm.ini) ou ~/.ssh/config.
func (a *TermService) ImportSessionsFile() (ImportResult, error) {
	path, err := a.app.Dialog.OpenFile().SetTitle("Escolha o arquivo de sessões").PromptForSingleSelection()
	if err != nil && !isCancel(err) {
		return ImportResult{}, err
	}
	if err != nil || path == "" {
		return ImportResult{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ImportResult{}, err
	}
	if len(data) > 32<<20 {
		return ImportResult{}, errors.New("arquivo grande demais para ser uma lista de sessões")
	}
	res, err := a.importBytes(data)
	res.Path = path
	return res, err
}

func (a *TermService) importBytes(data []byte) (ImportResult, error) {
	trim := bytes.TrimSpace(data)
	switch {
	case len(trim) > 0 && (trim[0] == '{' || trim[0] == '['):
		// "[Bookmarks]" também começa com "[": o JSON de verdade abre com {"/[{ ou [] e não tem "=" na 1ª linha
		if !bytes.HasPrefix(bytes.ToLower(trim), []byte("[bookmarks")) {
			added, skipped, err := a.store.ImportJSON(data)
			return ImportResult{Added: added, Skipped: skipped}, err
		}
		fallthrough
	case bytes.Contains(bytes.ToLower(data), []byte("[bookmarks")):
		m, err := store.ParseMobaXterm(bytes.NewReader(data))
		if err != nil {
			return ImportResult{}, err
		}
		added, skipped, err := a.store.ImportAll(m.Sessions, m.Folders)
		return ImportResult{Added: added, Skipped: skipped + m.Skipped, Folders: len(m.Folders)}, err
	default:
		home, _ := os.UserHomeDir()
		list := store.ParseSSHConfig(bytes.NewReader(data), home)
		if len(list) == 0 {
			return ImportResult{}, errors.New("não reconheci o formato do arquivo (esperava JSON do NxATerm, MobaXterm ou ~/.ssh/config)")
		}
		added, skipped, err := a.store.ImportAll(list, nil)
		return ImportResult{Added: added, Skipped: skipped}, err
	}
}

// ImportPuTTY lê as sessões salvas no PuTTY (registro do Windows).
func (a *TermService) ImportPuTTY() (ImportResult, error) {
	if runtime.GOOS != "windows" {
		return ImportResult{}, errors.New("a importação do PuTTY só funciona no Windows")
	}
	list, err := puttySessions()
	if err != nil {
		return ImportResult{}, errors.New("não encontrei sessões do PuTTY neste usuário")
	}
	if len(list) == 0 {
		return ImportResult{}, errors.New("o PuTTY não tem sessões SSH ou Telnet salvas")
	}
	added, skipped, err := a.store.ImportAll(list, []string{"PuTTY"})
	return ImportResult{Added: added, Skipped: skipped, Folders: 1}, err
}

// ExportHTML gera uma página HTML com a lista das sessões (sem senhas) para consulta ou impressão.
func (a *TermService) ExportHTML() (string, error) {
	return a.saveAs("nxaterm-sessoes.html", []byte(sessionsHTML(a.store.List(), a.store.Folders(), time.Now())))
}

func sessionSummaryGo(s store.Session) string {
	switch s.Type {
	case store.TypeTelnet:
		return fmt.Sprintf("%s:%d", s.Host, s.Port)
	case store.TypeFTP:
		return fmt.Sprintf("ftp://%s:%d", s.Host, s.Port)
	case store.TypeSerial:
		return fmt.Sprintf("%s · %d baud", s.SerialPort, s.BaudRate)
	case store.TypeShell:
		if s.Shell == "" {
			return "shell padrão"
		}
		return s.Shell
	case store.TypeWSL:
		if s.Distro == "" {
			return "distribuição padrão"
		}
		return s.Distro
	}
	return fmt.Sprintf("%s@%s:%d", s.User, s.Host, s.Port)
}

func sessionsHTML(list []store.Session, folders []string, at time.Time) string {
	byFolder := map[string][]store.Session{}
	names := map[string]bool{}
	for _, f := range folders {
		names[f] = true
	}
	for _, s := range list {
		byFolder[s.Folder] = append(byFolder[s.Folder], s)
		names[s.Folder] = true
	}
	keys := make([]string, 0, len(names))
	for k := range names {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="pt-BR"><head><meta charset="utf-8"><title>Sessões do NxATerm</title>
<style>body{font:15px/1.5 system-ui,sans-serif;max-width:900px;margin:30px auto;padding:0 16px;color:#222}
h1{font-size:22px}h2{font-size:16px;margin:22px 0 6px;border-bottom:1px solid #ddd}
table{border-collapse:collapse;width:100%}td{padding:4px 8px;border-bottom:1px solid #eee;vertical-align:top}
td.t{width:70px;color:#666}td.s{font-family:ui-monospace,Consolas,monospace}small{color:#777}</style></head><body>
<h1>Sessões do NxATerm</h1><small>Gerado em `)
	b.WriteString(html.EscapeString(at.Format("02/01/2006 15:04")))
	b.WriteString(` · senhas não são guardadas pelo NxATerm e não aparecem aqui</small>`)
	for _, k := range keys {
		title := k
		if title == "" {
			title = "(sem pasta)"
		}
		b.WriteString("<h2>" + html.EscapeString(strings.ReplaceAll(title, "/", " › ")) + "</h2><table>")
		if len(byFolder[k]) == 0 {
			b.WriteString(`<tr><td><small>pasta vazia</small></td></tr>`)
		}
		for _, s := range byFolder[k] {
			b.WriteString("<tr><td class=t>" + html.EscapeString(strings.ToUpper(s.Type)) + "</td><td>" + html.EscapeString(s.Name) +
				"</td><td class=s>" + html.EscapeString(sessionSummaryGo(s)) + "</td><td><small>" + html.EscapeString(s.Notes) + "</small></td></tr>")
		}
		b.WriteString("</table>")
	}
	b.WriteString("</body></html>")
	return b.String()
}
