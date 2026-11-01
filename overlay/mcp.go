package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nxaterm/internal/mcpsrv"
)

// McpState descreve o servidor MCP local.
type McpState struct {
	Running bool   `json:"running"`
	Addr    string `json:"addr"`
	URL     string `json:"url"`
}

type mcpAnswer struct {
	text  string
	isErr bool
}

type mcpRequest struct {
	ID   string          `json:"id"`
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args"`
}

func (a *TermService) mcpTokenPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mcp-token.txt"), nil
}

// McpToken devolve o token do servidor MCP (cria na primeira vez).
func (a *TermService) McpToken() (string, error) {
	p, err := a.mcpTokenPath()
	if err != nil {
		return "", err
	}
	if data, err := os.ReadFile(p); err == nil && len(strings.TrimSpace(string(data))) >= 32 {
		return strings.TrimSpace(string(data)), nil
	}
	tok := mcpsrv.NewToken()
	return tok, os.WriteFile(p, []byte(tok), 0o600)
}

// McpResetToken gera um token novo (os clientes antigos deixam de funcionar) e reinicia o servidor se estiver ativo.
func (a *TermService) McpResetToken() (string, error) {
	p, err := a.mcpTokenPath()
	if err != nil {
		return "", err
	}
	tok := mcpsrv.NewToken()
	if err := os.WriteFile(p, []byte(tok), 0o600); err != nil {
		return "", err
	}
	if on, addr := a.mcp.Running(); on {
		if port := portOf(addr); port > 0 {
			if err := a.McpStart(port); err != nil {
				return tok, err
			}
		}
	}
	return tok, nil
}

func portOf(addr string) int {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return 0
	}
	n := 0
	for _, c := range addr[i+1:] {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// McpStart liga o servidor MCP em 127.0.0.1:porta (só este computador).
func (a *TermService) McpStart(port int) error {
	tok, err := a.McpToken()
	if err != nil {
		return err
	}
	return a.mcp.Start(port, tok, mcpBackend{a})
}

// McpStop desliga o servidor MCP.
func (a *TermService) McpStop() { a.mcp.Stop() }

// McpStatus diz se o servidor está ligado.
func (a *TermService) McpStatus() McpState {
	on, addr := a.mcp.Running()
	st := McpState{Running: on, Addr: addr}
	if on {
		st.URL = "http://" + addr + "/mcp"
	}
	return st
}

// McpRespond é chamado pela interface com o resultado de uma ferramenta que ela executou.
func (a *TermService) McpRespond(id, text string, isError bool) {
	a.mcpMu.Lock()
	ch := a.mcpWait[id]
	a.mcpMu.Unlock()
	if ch != nil {
		select {
		case ch <- mcpAnswer{text, isError}:
		default:
		}
	}
}

// ---------------------------------------------------------------- ferramentas

type mcpBackend struct{ a *TermService }

var mcpTools = []mcpsrv.Tool{
	{Name: "list_sessions", Description: "Lista as sessões salvas no NxATerm (nome, tipo, host, usuário, pasta, tags). Não mostra senhas.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
	{Name: "list_tabs", Description: "Lista as abas abertas no NxATerm, com o id de cada uma.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
	{Name: "read_terminal", Description: "Lê as últimas linhas visíveis de uma aba de terminal.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tab":{"type":"string","description":"id da aba (de list_tabs); vazio = aba ativa"},"lines":{"type":"integer","description":"quantas linhas (padrão 50, máximo 500)"}}}`)},
	{Name: "send_input", Description: "Digita um texto numa aba de terminal. O usuário precisa aprovar na tela do NxATerm cada vez.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tab":{"type":"string","description":"id da aba; vazio = aba ativa"},"text":{"type":"string","description":"texto a digitar"},"enter":{"type":"boolean","description":"apertar Enter no final (padrão true)"}},"required":["text"]}`)},
	{Name: "open_session", Description: "Abre uma sessão salva pelo nome. O usuário precisa aprovar na tela do NxATerm.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"nome da sessão (de list_sessions)"}},"required":["name"]}`)},
}

func (b mcpBackend) Tools() []mcpsrv.Tool { return mcpTools }

func (b mcpBackend) Call(ctx context.Context, name string, args json.RawMessage) (string, bool) {
	known := false
	for _, t := range mcpTools {
		if t.Name == name {
			known = true
		}
	}
	if !known {
		return "ferramenta desconhecida: " + name, true
	}
	if name == "list_sessions" {
		type row struct {
			Name   string   `json:"name"`
			Type   string   `json:"type"`
			Host   string   `json:"host,omitempty"`
			User   string   `json:"user,omitempty"`
			Port   int      `json:"port,omitempty"`
			Folder string   `json:"folder,omitempty"`
			Tags   []string `json:"tags,omitempty"`
		}
		var rows []row
		for _, s := range b.a.store.List() {
			rows = append(rows, row{Name: s.Name, Type: string(s.Type), Host: s.Host, User: s.User, Port: s.Port, Folder: s.Folder, Tags: s.Tags})
		}
		js, _ := json.MarshalIndent(rows, "", " ")
		return string(js), false
	}
	if b.a.app == nil {
		return "o aplicativo não está pronto", true
	}
	id, err := mcpNewID()
	if err != nil {
		return err.Error(), true
	}
	ch := make(chan mcpAnswer, 1)
	b.a.mcpMu.Lock()
	if b.a.mcpWait == nil {
		b.a.mcpWait = map[string]chan mcpAnswer{}
	}
	b.a.mcpWait[id] = ch
	b.a.mcpMu.Unlock()
	defer func() {
		b.a.mcpMu.Lock()
		delete(b.a.mcpWait, id)
		b.a.mcpMu.Unlock()
	}()
	js, _ := json.Marshal(mcpRequest{ID: id, Tool: name, Args: args})
	b.a.app.Event.Emit("mcp:request", string(js))
	select {
	case r := <-ch:
		return r.text, r.isErr
	case <-ctx.Done():
		return "tempo esgotado esperando o NxATerm", true
	case <-time.After(4 * time.Minute):
		return "tempo esgotado esperando o NxATerm", true
	}
}

func mcpNewID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("sem fonte de números aleatórios")
	}
	return hex.EncodeToString(b), nil
}
