package mcpsrv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
)

type fake struct{}

func (fake) Tools() []Tool {
	return []Tool{{Name: "eco", Description: "d", InputSchema: json.RawMessage(`{"type":"object"}`)}}
}
func (fake) Call(ctx context.Context, name string, args json.RawMessage) (string, bool) {
	if name != "eco" {
		return "desconhecida", true
	}
	return "eco:" + string(args), false
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func post(t *testing.T, port int, token, body string, hdr map[string]string) (*http.Response, string) {
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/mcp", port), strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b bytes.Buffer
	b.ReadFrom(resp.Body)
	return resp, b.String()
}

func TestServer(t *testing.T) {
	port := freePort(t)
	tok := NewToken()
	var s Server
	if err := s.Start(port, tok, fake{}); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if ok, _ := s.Running(); !ok {
		t.Fatal("deveria estar rodando")
	}
	if r, _ := post(t, port, "", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, nil); r.StatusCode != 401 {
		t.Fatalf("sem token: %d", r.StatusCode)
	}
	if r, _ := post(t, port, "errado-errado-errado", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, nil); r.StatusCode != 401 {
		t.Fatalf("token errado: %d", r.StatusCode)
	}
	if r, _ := post(t, port, tok, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{"Origin": "https://evil.example"}); r.StatusCode != 403 {
		t.Fatalf("origem: %d", r.StatusCode)
	}
	if r, _ := post(t, port, tok, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{"Host": fmt.Sprintf("evil.example:%d", port)}); r.StatusCode != 403 {
		t.Fatalf("host: %d", r.StatusCode)
	}
	_, body := post(t, port, tok, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`, nil)
	if !strings.Contains(body, `"protocolVersion":"2025-06-18"`) || !strings.Contains(body, `"tools"`) {
		t.Fatalf("initialize: %s", body)
	}
	if r, _ := post(t, port, tok, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil); r.StatusCode != 202 {
		t.Fatalf("notificação: %d", r.StatusCode)
	}
	_, body = post(t, port, tok, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, nil)
	if !strings.Contains(body, `"name":"eco"`) {
		t.Fatalf("list: %s", body)
	}
	_, body = post(t, port, tok, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"eco","arguments":{"a":1}}}`, nil)
	if !strings.Contains(body, `eco:{\"a\":1}`) || !strings.Contains(body, `"isError":false`) {
		t.Fatalf("call: %s", body)
	}
	_, body = post(t, port, tok, `{"jsonrpc":"2.0","id":4,"method":"nada"}`, nil)
	if !strings.Contains(body, "-32601") {
		t.Fatalf("método: %s", body)
	}
	if r, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/mcp", port)); err != nil || r.StatusCode != 401 {
		t.Fatalf("GET sem token: %v", err)
	}
	s.Stop()
	if ok, _ := s.Running(); ok {
		t.Fatal("deveria ter parado")
	}
	if err := s.Start(80, tok, fake{}); err == nil {
		t.Fatal("porta baixa deveria falhar")
	}
}
