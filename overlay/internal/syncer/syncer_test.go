package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func bundle() Bundle {
	return Bundle{
		Sessions: json.RawMessage(`[{"id":"a","name":"x"}]`), Folders: json.RawMessage(`["P"]`),
		Macros: json.RawMessage(`[]`), Settings: json.RawMessage(`{"theme":"Nord"}`),
	}
}

func TestSealOpen(t *testing.T) {
	plain := []byte(`{"a":1}`)
	if got, err := Seal(plain, ""); err != nil || string(got) != string(plain) {
		t.Fatalf("sem senha: %s %v", got, err)
	}
	sealed, err := Seal(plain, "segredo")
	if err != nil || strings.Contains(string(sealed), `"a":1`) {
		t.Fatalf("seal: %s %v", sealed, err)
	}
	got, err := Open(sealed, "segredo")
	if err != nil || string(got) != string(plain) {
		t.Fatalf("open: %s %v", got, err)
	}
	if _, err := Open(sealed, "errada"); err == nil || !strings.Contains(err.Error(), "incorreta") {
		t.Fatalf("senha errada: %v", err)
	}
	if _, err := Open(sealed, ""); err == nil {
		t.Fatal("sem senha numa cópia cifrada devia falhar")
	}
	if got, err := Open(plain, "qualquer"); err != nil || string(got) != string(plain) {
		t.Fatalf("texto claro: %s %v", got, err)
	}
}

func TestGistFlow(t *testing.T) {
	store := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			io.WriteString(w, `{"message":"Bad credentials"}`)
			return
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/gists", r.Method == "PATCH" && r.URL.Path == "/gists/g1":
			var p struct {
				Files map[string]struct {
					Content string `json:"content"`
				} `json:"files"`
				Public *bool `json:"public"`
			}
			json.NewDecoder(r.Body).Decode(&p)
			if r.Method == "POST" && (p.Public == nil || *p.Public) {
				t.Error("gist novo deve ser secreto")
			}
			store = p.Files[FileName].Content
			io.WriteString(w, `{"id":"g1"}`)
		case r.Method == "GET" && r.URL.Path == "/gists/g1":
			j, _ := json.Marshal(map[string]any{"files": map[string]any{FileName: map[string]any{"content": store}}})
			w.Write(j)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), GistBase: srv.URL}
	ctx := context.Background()
	cfg := Config{Provider: Gist, Token: "tok", Passcode: "pw"}
	if _, err := c.Pull(ctx, cfg); !errors.Is(err, ErrNotFound) {
		t.Fatalf("sem gist: %v", err)
	}
	id, err := c.Push(ctx, cfg, bundle())
	if err != nil || id != "g1" {
		t.Fatalf("push: %q %v", id, err)
	}
	if strings.Contains(store, "Nord") {
		t.Fatal("a cópia foi enviada sem criptografia")
	}
	cfg.GistID = id
	got, err := c.Pull(ctx, cfg)
	if err != nil || !strings.Contains(string(got.Settings), "Nord") || got.Version != 1 {
		t.Fatalf("pull: %+v %v", got, err)
	}
	if _, err := c.Push(ctx, cfg, bundle()); err != nil {
		t.Fatalf("atualizar: %v", err)
	}
	cfg.Token = "ruim"
	if _, err := c.Pull(ctx, cfg); err == nil || !strings.Contains(err.Error(), "acesso negado") {
		t.Fatalf("token ruim: %v", err)
	}
}

func TestWebDAVAndCustom(t *testing.T) {
	var data []byte
	var path, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		switch r.Method {
		case "PUT":
			data, _ = io.ReadAll(r.Body)
			w.WriteHeader(201)
		case "GET":
			if data == nil {
				w.WriteHeader(404)
				return
			}
			w.Write(data)
		}
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), GistBase: srv.URL}
	ctx := context.Background()

	dav := Config{Provider: WebDAV, URL: srv.URL + "/dav/", User: "u", Password: "p"}
	if _, err := c.Pull(ctx, dav); !errors.Is(err, ErrNotFound) {
		t.Fatalf("webdav vazio: %v", err)
	}
	if _, err := c.Push(ctx, dav, bundle()); err != nil {
		t.Fatal(err)
	}
	if path != "/dav/"+FileName || !strings.HasPrefix(auth, "Basic ") {
		t.Fatalf("webdav: %s %s", path, auth)
	}
	if b, err := c.Pull(ctx, dav); err != nil || len(b.Sessions) == 0 {
		t.Fatalf("webdav pull: %v", err)
	}

	cust := Config{Provider: Custom, URL: srv.URL + "/sync", Token: "abc"}
	if _, err := c.Push(ctx, cust, bundle()); err != nil {
		t.Fatal(err)
	}
	if path != "/sync" || auth != "Bearer abc" {
		t.Fatalf("custom: %s %s", path, auth)
	}
	if _, err := c.Pull(ctx, Config{Provider: Custom, URL: "ftp://x"}); err == nil {
		t.Fatal("endereço ftp devia ser recusado")
	}
	data = []byte(`{"outra":"coisa"}`)
	if _, err := c.Pull(ctx, cust); err == nil || !strings.Contains(err.Error(), "não é uma cópia") {
		t.Fatalf("arquivo estranho: %v", err)
	}
}
