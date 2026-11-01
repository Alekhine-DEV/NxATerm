// Package syncer guarda e busca uma cópia das sessões, macros e configurações em um destino remoto:
// GitHub Gist, WebDAV ou um servidor HTTP próprio. A cópia pode ser criptografada com uma senha.
package syncer

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nxaterm/internal/kdf"
)

const defaultGistBase = "https://api.github.com"

const (
	FileName   = "nxaterm-sync.json"
	maxBody    = 16 << 20
	kdfIters   = 150_000
	bundleKind = "nxaterm-sync"
)

// Provedores aceitos.
const (
	Gist   = "gist"
	WebDAV = "webdav"
	Custom = "custom"
)

// ErrNotFound: o destino ainda não tem cópia.
var ErrNotFound = errors.New("ainda não há cópia no destino")

// Config diz onde e como sincronizar.
type Config struct {
	Provider string `json:"provider"`
	Token    string `json:"token"`    // Gist (token do GitHub com permissão de gist) e servidor próprio (Bearer)
	GistID   string `json:"gistId"`   // vazio = cria um gist novo no primeiro envio
	URL      string `json:"url"`      // WebDAV (pasta ou arquivo) e servidor próprio
	User     string `json:"user"`     // WebDAV
	Password string `json:"password"` // WebDAV
	Passcode string `json:"passcode"` // senha da criptografia da cópia (vazio = sem criptografia)
}

// Bundle é o conteúdo sincronizado (JSON dentro do arquivo, claro ou criptografado).
type Bundle struct {
	Kind      string          `json:"kind"`
	Version   int             `json:"version"`
	UpdatedAt string          `json:"updatedAt"`
	Sessions  json.RawMessage `json:"sessions"`
	Folders   json.RawMessage `json:"folders"`
	Macros    json.RawMessage `json:"macros"`
	Settings  json.RawMessage `json:"settings"`
}

// Client faz as chamadas HTTP; GistBase existe para os testes.
type Client struct {
	HTTP     *http.Client
	GistBase string
}

// NewClient usa o cliente HTTP padrão com limite de tempo.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, GistBase: defaultGistBase}
}

// ---------------------------------------------------------------- criptografia

type envelope struct {
	Kind string `json:"kind"`
	Enc  string `json:"enc"`
	Iter int    `json:"iter"`
	Salt string `json:"salt"`
	Data string `json:"data"` // nonce + texto cifrado, em base64
}

// Seal cifra o conteúdo (AES-256-GCM, chave derivada com PBKDF2). Sem senha, devolve o conteúdo como veio.
func Seal(plain []byte, pass string) ([]byte, error) {
	if pass == "" {
		return plain, nil
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	gcm, err := newGCM(pass, salt, kdfIters)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nonce, nonce, plain, []byte(bundleKind))
	return json.Marshal(envelope{
		Kind: bundleKind, Enc: "aes-256-gcm", Iter: kdfIters,
		Salt: base64.StdEncoding.EncodeToString(salt), Data: base64.StdEncoding.EncodeToString(ct),
	})
}

// Open desfaz o Seal. Conteúdo sem criptografia passa direto (se pass vier preenchida, é aceito mesmo assim).
func Open(raw []byte, pass string) ([]byte, error) {
	var env envelope
	if json.Unmarshal(raw, &env) != nil || env.Enc == "" {
		return raw, nil
	}
	if pass == "" {
		return nil, errors.New("a cópia está criptografada: informe a senha da criptografia")
	}
	if env.Enc != "aes-256-gcm" || env.Iter < 1000 || env.Iter > 5_000_000 {
		return nil, errors.New("criptografia da cópia não reconhecida")
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return nil, errors.New("cópia corrompida")
	}
	ct, err := base64.StdEncoding.DecodeString(env.Data)
	if err != nil {
		return nil, errors.New("cópia corrompida")
	}
	gcm, err := newGCM(pass, salt, env.Iter)
	if err != nil {
		return nil, err
	}
	if len(ct) < gcm.NonceSize() {
		return nil, errors.New("cópia corrompida")
	}
	plain, err := gcm.Open(nil, ct[:gcm.NonceSize()], ct[gcm.NonceSize():], []byte(bundleKind))
	if err != nil {
		return nil, errors.New("senha da criptografia incorreta")
	}
	return plain, nil
}

func newGCM(pass string, salt []byte, iter int) (cipher.AEAD, error) {
	block, err := aes.NewCipher(kdf.PBKDF2([]byte(pass), salt, iter, 32))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// ---------------------------------------------------------------- envio e busca

// Push grava a cópia no destino. Devolve o ID do gist (novo ou o mesmo).
func (c *Client) Push(ctx context.Context, cfg Config, b Bundle) (gistID string, err error) {
	b.Kind, b.Version = bundleKind, 1
	b.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	plain, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return "", err
	}
	data, err := Seal(plain, cfg.Passcode)
	if err != nil {
		return "", err
	}
	switch cfg.Provider {
	case Gist:
		return c.gistPush(ctx, cfg, data)
	case WebDAV, Custom:
		return "", c.putURL(ctx, cfg, data)
	}
	return "", fmt.Errorf("destino desconhecido: %q", cfg.Provider)
}

// Pull baixa a cópia do destino e a decifra.
func (c *Client) Pull(ctx context.Context, cfg Config) (Bundle, error) {
	var raw []byte
	var err error
	switch cfg.Provider {
	case Gist:
		raw, err = c.gistPull(ctx, cfg)
	case WebDAV, Custom:
		raw, err = c.getURL(ctx, cfg)
	default:
		return Bundle{}, fmt.Errorf("destino desconhecido: %q", cfg.Provider)
	}
	if err != nil {
		return Bundle{}, err
	}
	plain, err := Open(raw, cfg.Passcode)
	if err != nil {
		return Bundle{}, err
	}
	var b Bundle
	if err := json.Unmarshal(plain, &b); err != nil || b.Kind != bundleKind {
		return Bundle{}, errors.New("o arquivo no destino não é uma cópia do NxATerm")
	}
	return b, nil
}

func (c *Client) do(req *http.Request) ([]byte, int, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("falha de conexão: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(body) > maxBody {
		return nil, resp.StatusCode, errors.New("resposta grande demais")
	}
	return body, resp.StatusCode, nil
}

func httpErr(code int, body []byte) error {
	msg := strings.TrimSpace(string(body))
	var j struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &j) == nil && j.Message != "" {
		msg = j.Message
	}
	if len(msg) > 200 {
		msg = msg[:200]
	}
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("acesso negado (%d): confira o token/usuário e a senha. %s", code, msg)
	}
	return fmt.Errorf("o servidor respondeu %d %s", code, msg)
}

// ---------------------------------------------------------------- GitHub Gist

func (c *Client) gistReq(ctx context.Context, cfg Config, method, path string, body []byte) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.GistBase+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "NxATerm-sync")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) gistPush(ctx context.Context, cfg Config, data []byte) (string, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return "", errors.New("informe o token do GitHub (com permissão de gist)")
	}
	payload := map[string]any{"files": map[string]any{FileName: map[string]string{"content": string(data)}}}
	method, path := http.MethodPost, "/gists"
	if id := strings.TrimSpace(cfg.GistID); id != "" {
		method, path = http.MethodPatch, "/gists/"+url.PathEscape(id)
	} else {
		payload["description"] = "NxATerm: sessões e configurações"
		payload["public"] = false
	}
	js, _ := json.Marshal(payload)
	req, err := c.gistReq(ctx, cfg, method, path, js)
	if err != nil {
		return "", err
	}
	body, code, err := c.do(req)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK && code != http.StatusCreated {
		return "", httpErr(code, body)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.ID == "" {
		return "", errors.New("resposta inesperada do GitHub")
	}
	return out.ID, nil
}

func (c *Client) gistPull(ctx context.Context, cfg Config) ([]byte, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("informe o token do GitHub (com permissão de gist)")
	}
	if strings.TrimSpace(cfg.GistID) == "" {
		return nil, ErrNotFound
	}
	req, err := c.gistReq(ctx, cfg, http.MethodGet, "/gists/"+url.PathEscape(strings.TrimSpace(cfg.GistID)), nil)
	if err != nil {
		return nil, err
	}
	body, code, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if code != http.StatusOK {
		return nil, httpErr(code, body)
	}
	var g struct {
		Files map[string]struct {
			Content   string `json:"content"`
			Truncated bool   `json:"truncated"`
			RawURL    string `json:"raw_url"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &g); err != nil {
		return nil, err
	}
	f, ok := g.Files[FileName]
	if !ok {
		return nil, ErrNotFound
	}
	if !f.Truncated {
		return []byte(f.Content), nil
	}
	// arquivo grande: o conteúdo completo vem pelo endereço "raw" (só aceito se for do próprio GitHub)
	if c.GistBase == defaultGistBase {
		u, err := url.Parse(f.RawURL)
		if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), ".githubusercontent.com") {
			return nil, errors.New("endereço do arquivo inesperado")
		}
	}
	rreq, err := http.NewRequestWithContext(ctx, http.MethodGet, f.RawURL, nil)
	if err != nil {
		return nil, err
	}
	rreq.Header.Set("Authorization", "Bearer "+cfg.Token)
	raw, rcode, err := c.do(rreq)
	if err != nil {
		return nil, err
	}
	if rcode != http.StatusOK {
		return nil, httpErr(rcode, raw)
	}
	return raw, nil
}

// ---------------------------------------------------------------- WebDAV e servidor próprio

func targetURL(cfg Config) (string, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("endereço inválido (use http:// ou https://)")
	}
	if cfg.Provider == WebDAV && !strings.HasSuffix(strings.ToLower(u.Path), ".json") {
		u.Path = strings.TrimRight(u.Path, "/") + "/" + FileName
	}
	return u.String(), nil
}

func authorize(req *http.Request, cfg Config) {
	req.Header.Set("User-Agent", "NxATerm-sync")
	switch {
	case cfg.Provider == WebDAV && (cfg.User != "" || cfg.Password != ""):
		req.SetBasicAuth(cfg.User, cfg.Password)
	case cfg.Provider == Custom && cfg.Token != "":
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
}

func (c *Client) getURL(ctx context.Context, cfg Config) ([]byte, error) {
	u, err := targetURL(cfg)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	authorize(req, cfg)
	body, code, err := c.do(req)
	if err != nil {
		return nil, err
	}
	switch {
	case code == http.StatusNotFound:
		return nil, ErrNotFound
	case code != http.StatusOK:
		return nil, httpErr(code, body)
	}
	return body, nil
}

func (c *Client) putURL(ctx context.Context, cfg Config, data []byte) error {
	u, err := targetURL(cfg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	authorize(req, cfg)
	body, code, err := c.do(req)
	if err != nil {
		return err
	}
	if code < 200 || code > 299 {
		return httpErr(code, body)
	}
	return nil
}
