// Package mcpsrv é um servidor MCP (Model Context Protocol) mínimo sobre HTTP, só para o computador local.
// Fala JSON-RPC 2.0 no transporte "Streamable HTTP" (uma resposta JSON por POST) e exige um token.
package mcpsrv

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Tool descreve uma ferramenta exposta.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Backend executa as chamadas. Devolve o texto do resultado; erro de negócio vai em isError.
type Backend interface {
	Tools() []Tool
	Call(ctx context.Context, name string, args json.RawMessage) (text string, isError bool)
}

// NewToken gera um token aleatório (64 caracteres hexadecimais).
func NewToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Server é o servidor; Start/Stop são seguros para chamar mais de uma vez.
type Server struct {
	mu   sync.Mutex
	srv  *http.Server
	addr string
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

const maxRequest = 1 << 20

// Handler monta o handler HTTP (usado também nos testes).
func Handler(token string, port int, b Backend) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if !hostOK(r.Host, port) {
			http.Error(w, "host inválido", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" { // navegadores: bloqueia páginas de fora
			http.Error(w, "origem não permitida", http.StatusForbidden)
			return
		}
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || subtle.ConstantTimeCompare([]byte(auth), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "token inválido", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "use POST", http.StatusMethodNotAllowed)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, maxRequest))
		if err != nil {
			http.Error(w, "corpo ilegível", http.StatusBadRequest)
			return
		}
		var req rpcReq
		if err := json.Unmarshal(data, &req); err != nil || req.Method == "" {
			writeJSON(w, rpcResp{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcErr{-32700, "JSON inválido"}})
			return
		}
		if len(req.ID) == 0 { // notificação: sem resposta
			w.WriteHeader(http.StatusAccepted)
			return
		}
		resp := rpcResp{JSONRPC: "2.0", ID: req.ID}
		switch req.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			json.Unmarshal(req.Params, &p)
			ver := p.ProtocolVersion
			if ver == "" {
				ver = "2025-03-26"
			}
			resp.Result = map[string]any{
				"protocolVersion": ver,
				"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
				"serverInfo":      map[string]any{"name": "nxaterm", "version": "1"},
			}
		case "ping":
			resp.Result = map[string]any{}
		case "tools/list":
			resp.Result = map[string]any{"tools": b.Tools()}
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
				resp.Error = &rpcErr{-32602, "parâmetros inválidos"}
				break
			}
			if len(p.Arguments) == 0 {
				p.Arguments = json.RawMessage("{}")
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
			text, isErr := b.Call(ctx, p.Name, p.Arguments)
			cancel()
			resp.Result = map[string]any{
				"content": []map[string]any{{"type": "text", "text": text}},
				"isError": isErr,
			}
		default:
			resp.Error = &rpcErr{-32601, "método não suportado: " + req.Method}
		}
		writeJSON(w, resp)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// hostOK aceita só 127.0.0.1/localhost/[::1] na porta do servidor (protege contra DNS rebinding).
func hostOK(host string, port int) bool {
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	if p != fmt.Sprint(port) {
		return false
	}
	switch strings.ToLower(h) {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// Start escuta em 127.0.0.1:port. Uma segunda chamada troca o servidor anterior.
func (s *Server) Start(port int, token string, b Backend) error {
	if port < 1024 || port > 65535 {
		return errors.New("porta inválida (use de 1024 a 65535)")
	}
	if len(token) < 16 {
		return errors.New("token curto demais")
	}
	s.Stop()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("não foi possível abrir a porta %d: %w", port, err)
	}
	srv := &http.Server{Handler: Handler(token, port, b), ReadHeaderTimeout: 10 * time.Second}
	s.mu.Lock()
	s.srv, s.addr = srv, ln.Addr().String()
	s.mu.Unlock()
	go srv.Serve(ln)
	return nil
}

// Stop encerra o servidor, se estiver ativo.
func (s *Server) Stop() {
	s.mu.Lock()
	srv := s.srv
	s.srv, s.addr = nil, ""
	s.mu.Unlock()
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if srv.Shutdown(ctx) != nil {
			srv.Close()
		}
	}
}

// Running diz se está escutando e onde.
func (s *Server) Running() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.srv != nil, s.addr
}
