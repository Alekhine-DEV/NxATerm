package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"nxaterm/internal/store"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TermService guarda o estado do aplicativo. É registrado como serviço do
// Wails v3, então todos os métodos exportados viram funções no frontend
// (bindings gerados por `wails3 generate bindings`).
type TermService struct {
	app *application.App

	store      *store.Store
	knownHosts string
	khMu       sync.Mutex

	termMu sync.Mutex
	terms  map[string]*termSession

	macros       *store.MacroStore
	settingsPath string

	tunMu   sync.Mutex
	tunnels map[string]*tunnel
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
	return &TermService{
		app:          app,
		store:        st,
		knownHosts:   filepath.Join(dir, "known_hosts"),
		terms:        map[string]*termSession{},
		macros:       ms,
		settingsPath: filepath.Join(dir, "settings.json"),
		tunnels:      map[string]*tunnel{},
	}, nil
}

// ServiceStartup é chamado pelo Wails ao iniciar o app.
func (a *TermService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown é chamado pelo Wails ao encerrar: fecha todas as conexões.
func (a *TermService) ServiceShutdown() error {
	a.closeAll()
	return nil
}

// ListSessions devolve todas as sessões salvas (sem limite de quantidade).
func (a *TermService) ListSessions() []store.Session { return a.store.List() }

// SaveSession cria ou atualiza uma sessão.
func (a *TermService) SaveSession(s store.Session) (store.Session, error) { return a.store.Save(s) }

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
	dlg := a.app.Dialog.OpenFile().SetTitle("Escolha a chave privada")
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
