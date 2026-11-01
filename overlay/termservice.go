package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"nxaterm/internal/applock"
	"nxaterm/internal/mcpsrv"
	"nxaterm/internal/netx"
	"nxaterm/internal/store"
	"nxaterm/internal/tomb"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TermService guarda o estado do aplicativo. É registrado como serviço do
// Wails v3, então todos os métodos exportados viram funções no frontend
// (bindings gerados por `wails3 generate bindings`).
type TermService struct {
	app        *application.App
	showWindow func() // traz a janela principal para a frente

	store      *store.Store
	knownHosts string
	khMu       sync.Mutex

	termMu sync.Mutex
	terms  map[string]*termSession

	macros       *store.MacroStore
	tombs        *tomb.Log // o que foi apagado (para a sincronização propagar exclusões)
	settingsPath string

	tunMu   sync.Mutex
	tunnels map[string]*activeTunnel

	promptMu sync.Mutex
	prompts  map[string]chan promptAnswer

	xf     *xferState
	lock   *applock.Lock
	dataMu sync.Mutex // troca de senha / desbloqueio dos dados

	syncMu sync.Mutex

	mcp     mcpsrv.Server
	mcpMu   sync.Mutex
	mcpWait map[string]chan mcpAnswer

	trayMu   sync.Mutex
	trayMenu *application.Menu
	trayShow *application.MenuItem
	trayQuit *application.MenuItem

	quitting atomic.Bool // o usuário pediu para sair de verdade (não é só fechar a janela)
	toggle   func()      // mostra ou esconde a janela principal
}

// NewTermService prepara a pasta de dados e carrega as sessões salvas.
func NewTermService(app *application.App) (*TermService, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, err
	}
	st, err := store.Open(filepath.Join(dir, "sessions.json"))
	if err != nil {
		return nil, err
	}
	ms, err := store.OpenMacros(filepath.Join(dir, "macros.json"))
	if err != nil {
		return nil, err
	}
	lk, err := applock.Open(filepath.Join(dir, "lock.json"))
	if err != nil {
		return nil, err
	}
	tl, err := tomb.Open(filepath.Join(dir, "deleted.json"))
	if err != nil {
		return nil, err
	}
	st.SetTombs(tl)
	ms.SetTombs(tl)
	return &TermService{
		lock:         lk,
		app:          app,
		store:        st,
		knownHosts:   filepath.Join(dir, "known_hosts"),
		terms:        map[string]*termSession{},
		macros:       ms,
		tombs:        tl,
		settingsPath: filepath.Join(dir, "settings.json"),
		tunnels:      map[string]*activeTunnel{},
		prompts:      map[string]chan promptAnswer{},
		xf:           newXferState(),
	}, nil
}

// ServiceStartup é chamado pelo Wails ao iniciar o app.
func (a *TermService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown é chamado pelo Wails ao encerrar: fecha todas as conexões.
func (a *TermService) ServiceShutdown() error {
	a.mcp.Stop()
	a.closeAll()
	return nil
}

// ListSessions devolve todas as sessões salvas (sem limite de quantidade).
func (a *TermService) ListSessions() []store.Session { return a.store.List() }

// SaveSession cria ou atualiza uma sessão.
func (a *TermService) SaveSession(s store.Session) (store.Session, error) {
	if !a.CheckEncoding(s.Encoding) {
		return store.Session{}, fmt.Errorf("codificação desconhecida: %q", s.Encoding)
	}
	if px := strings.TrimSpace(s.Proxy); px != "" && px != "none" {
		if _, err := netx.ParseProxy(px); err != nil {
			return store.Session{}, err
		}
	}
	return a.store.Save(s)
}

// DeleteSession remove uma sessão salva.
func (a *TermService) DeleteSession(id string) error { return a.store.Delete(id) }

// ImportSSHConfig importa os hosts de ~/.ssh/config. Devolve quantos eram novos.
func (a *TermService) ImportSSHConfig() (int, error) {
	p, err := store.DefaultSSHConfigPath()
	if err != nil {
		return 0, err
	}
	f, err := os.Open(p)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	home, _ := os.UserHomeDir()
	return a.store.Import(store.ParseSSHConfig(f, home))
}

// PickKeyFile abre o seletor de arquivos para escolher a chave privada.
// Devolve string vazia se o usuário cancelar.
func (a *TermService) PickKeyFile() (string, error) {
	dlg := a.app.Dialog.OpenFile().SetTitle(tr("Escolha a chave privada"))
	if home, err := os.UserHomeDir(); err == nil {
		dlg = dlg.SetDirectory(filepath.Join(home, ".ssh"))
	}
	p, err := dlg.PromptForSingleSelection()
	if err != nil && !isCancel(err) {
		return "", err
	}
	if err != nil {
		return "", nil
	}
	return p, nil
}
