// Package aichat fala com qualquer serviço compatível com a API de chat da OpenAI
// (OpenAI, OpenRouter, Ollama, LM Studio, vLLM...). Escrito do zero; só usa a biblioteca padrão.
package aichat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Config é o destino da conversa.
type Config struct {
	BaseURL string `json:"baseUrl"` // ex.: https://api.openai.com/v1
	APIKey  string `json:"apiKey"`  // pode ser vazio (Ollama local)
	Model   string `json:"model"`
}

// ToolCall é um pedido do modelo para executar uma ferramenta.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON em texto
}

// Message é uma mensagem da conversa.
type Message struct {
	Role       string     `json:"role"` // system | user | assistant | tool
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"toolCalls,omitempty"`
	ToolCallID string     `json:"toolCallId,omitempty"`
}

// Tool descreve uma ferramenta que o modelo pode pedir.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// Reply é a resposta do modelo.
type Reply struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"toolCalls"`
	Finish    string     `json:"finish"`
}

type Client struct{ HTTP *http.Client }

func NewClient() *Client { return &Client{HTTP: &http.Client{Timeout: 3 * time.Minute}} }

const maxBody = 8 << 20

func (c Config) base() (string, error) {
	b := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	u, err := url.Parse(b)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("endereço da API inválido (use http:// ou https://)")
	}
	return b, nil
}

func (c *Client) do(ctx context.Context, cfg Config, method, path string, body any, out any) error {
	base, err := cfg.base()
	if err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if k := strings.TrimSpace(cfg.APIKey); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}
	req.Header.Set("Accept", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("não foi possível falar com a API: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return errors.New(apiError(resp.StatusCode, data))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New("resposta inesperada da API")
	}
	return nil
}

func apiError(status int, data []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	msg := ""
	if json.Unmarshal(data, &e) == nil {
		msg = e.Error.Message
		if msg == "" {
			msg = e.Message
		}
	}
	if msg == "" {
		msg = strings.TrimSpace(string(data))
		if len(msg) > 300 {
			msg = msg[:300]
		}
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "a API recusou a chave (" + fmt.Sprint(status) + "): " + msg
	}
	return fmt.Sprintf("erro da API (%d): %s", status, msg)
}

type wireMsg struct {
	Role       string     `json:"role"`
	Content    *string    `json:"content"`
	ToolCalls  []wireCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type wireCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

func toWire(msgs []Message) []wireMsg {
	out := make([]wireMsg, 0, len(msgs))
	for _, m := range msgs {
		w := wireMsg{Role: m.Role, ToolCallID: m.ToolCallID}
		content := m.Content
		if !(m.Role == "assistant" && len(m.ToolCalls) > 0 && content == "") {
			w.Content = &content
		}
		for _, tc := range m.ToolCalls {
			var wc wireCall
			wc.ID, wc.Type = tc.ID, "function"
			wc.Function.Name, wc.Function.Arguments = tc.Name, tc.Arguments
			w.ToolCalls = append(w.ToolCalls, wc)
		}
		out = append(out, w)
	}
	return out
}

// Chat envia a conversa e devolve a resposta do modelo (sem streaming).
func (c *Client) Chat(ctx context.Context, cfg Config, msgs []Message, tools []Tool) (Reply, error) {
	if strings.TrimSpace(cfg.Model) == "" {
		return Reply{}, errors.New("escolha o modelo")
	}
	if len(msgs) == 0 {
		return Reply{}, errors.New("conversa vazia")
	}
	body := map[string]any{"model": cfg.Model, "messages": toWire(msgs), "stream": false}
	if len(tools) > 0 {
		wt := make([]wireTool, 0, len(tools))
		for _, t := range tools {
			var x wireTool
			x.Type = "function"
			x.Function.Name, x.Function.Description, x.Function.Parameters = t.Name, t.Description, t.Parameters
			wt = append(wt, x)
		}
		body["tools"] = wt
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content   *string    `json:"content"`
				ToolCalls []wireCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := c.do(ctx, cfg, http.MethodPost, "/chat/completions", body, &resp); err != nil {
		return Reply{}, err
	}
	if len(resp.Choices) == 0 {
		return Reply{}, errors.New("a API não devolveu resposta")
	}
	ch := resp.Choices[0]
	r := Reply{Finish: ch.FinishReason}
	if ch.Message.Content != nil {
		r.Content = *ch.Message.Content
	}
	for i, tc := range ch.Message.ToolCalls {
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		r.ToolCalls = append(r.ToolCalls, ToolCall{ID: id, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	return r, nil
}

// Models lista os modelos que o serviço oferece (GET /models).
func (c *Client) Models(ctx context.Context, cfg Config) ([]string, error) {
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct { // formato de alguns servidores
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := c.do(ctx, cfg, http.MethodGet, "/models", nil, &resp); err != nil {
		return nil, err
	}
	var ids []string
	for _, d := range resp.Data {
		if d.ID != "" {
			ids = append(ids, d.ID)
		}
	}
	for _, m := range resp.Models {
		if m.Name != "" {
			ids = append(ids, m.Name)
		}
	}
	sort.Strings(ids)
	return ids, nil
}
