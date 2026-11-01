package aichat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatTextAndTools(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, `{"error":{"message":"não autorizado"}}`, 401)
			return
		}
		json.NewDecoder(r.Body).Decode(&got)
		if _, ok := got["tools"]; ok {
			w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"run_command","arguments":"{\"command\":\"ls\"}"}}]}}]}`))
			return
		}
		w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"olá"}}]}`))
	}))
	defer srv.Close()
	c := NewClient()
	cfg := Config{BaseURL: srv.URL + "/v1/", APIKey: "k", Model: "m"}
	r, err := c.Chat(context.Background(), cfg, []Message{{Role: "user", Content: "oi"}}, nil)
	if err != nil || r.Content != "olá" {
		t.Fatalf("texto: %v %+v", err, r)
	}
	tools := []Tool{{Name: "run_command", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}}
	r, err = c.Chat(context.Background(), cfg, []Message{{Role: "user", Content: "liste"}}, tools)
	if err != nil || len(r.ToolCalls) != 1 || r.ToolCalls[0].Name != "run_command" || !strings.Contains(r.ToolCalls[0].Arguments, "ls") {
		t.Fatalf("tools: %v %+v", err, r)
	}
	// o resultado da ferramenta volta como role=tool; a mensagem do assistente sem texto não leva content
	msgs := []Message{{Role: "user", Content: "x"}, {Role: "assistant", ToolCalls: r.ToolCalls}, {Role: "tool", ToolCallID: "c1", Content: "a b"}}
	raw, _ := json.Marshal(toWire(msgs))
	if !strings.Contains(string(raw), `"tool_call_id":"c1"`) || !strings.Contains(string(raw), `"role":"assistant","content":null`) && strings.Contains(string(raw), `"role":"assistant","content":""`) {
		t.Fatalf("wire: %s", raw)
	}
	cfg.APIKey = "errada"
	if _, err = c.Chat(context.Background(), cfg, []Message{{Role: "user", Content: "oi"}}, nil); err == nil || !strings.Contains(err.Error(), "recusou a chave") {
		t.Fatalf("401: %v", err)
	}
	if _, err = c.Chat(context.Background(), Config{BaseURL: "ftp://x", Model: "m"}, []Message{{Role: "user", Content: "oi"}}, nil); err == nil {
		t.Fatal("endereço inválido deveria falhar")
	}
	if _, err = c.Chat(context.Background(), Config{BaseURL: srv.URL}, []Message{{Role: "user", Content: "oi"}}, nil); err == nil {
		t.Fatal("sem modelo deveria falhar")
	}
}

func TestModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"b"},{"id":"a"}]}`))
	}))
	defer srv.Close()
	ids, err := NewClient().Models(context.Background(), Config{BaseURL: srv.URL})
	if err != nil || len(ids) != 2 || ids[0] != "a" {
		t.Fatalf("%v %v", err, ids)
	}
}
