package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"nxaterm/internal/launch"
	"nxaterm/internal/store"
)

// launchState guarda o que foi pedido ao abrir o programa até o frontend ficar pronto.
type launchState struct {
	mu    sync.Mutex
	items []launch.Item
}

var pendingLaunch launchState

// TakeLaunchItems devolve (uma única vez) os links e sessões pedidos na linha de comando.
func (a *TermService) TakeLaunchItems() []string {
	pendingLaunch.mu.Lock()
	defer pendingLaunch.mu.Unlock()
	out := pendingLaunch.items
	pendingLaunch.items = nil
	if out == nil {
		out = []string{}
	}
	return out
}

// onSecondInstance é chamado quando o usuário abre o NxATerm de novo (ou clica num
// link ssh://): a janela existente vem para a frente e abre o que foi pedido.
func (a *TermService) onSecondInstance(args []string) {
	if a.showWindow != nil {
		a.showWindow()
	}
	if len(args) > 0 {
		args = args[1:] // o primeiro é o nome do programa
	}
	items := launch.Collect(args)
	if len(items) == 0 {
		return
	}
	js, _ := json.Marshal(items)
	a.app.Event.Emit("launch:open", string(js))
}

// ResolveLaunch transforma um item (link ou "session:nome") numa sessão pronta para abrir.
func (a *TermService) ResolveLaunch(item string) (store.Session, error) {
	if name, ok := strings.CutPrefix(item, "session:"); ok {
		for _, s := range a.store.List() {
			if strings.EqualFold(s.Name, name) || s.ID == name {
				return s, nil
			}
		}
		return store.Session{}, fmt.Errorf("não existe a sessão %q", name)
	}
	t, err := launch.FromURL(item)
	if err != nil {
		return store.Session{}, err
	}
	typ := t.Type
	s := store.Session{Type: typ, Host: t.Host, User: t.User, Port: t.Port, AuthMethod: "password"}
	return store.Normalize(s)
}

// LinkHandlerRegistered diz se este programa já abre os links ssh:// e telnet://.
func (a *TermService) LinkHandlerRegistered() bool { return linkHandlerRegistered() }

// RegisterLinkHandler faz o sistema abrir links ssh:// e telnet:// com o NxATerm.
func (a *TermService) RegisterLinkHandler() error {
	exe, err := os.Executable()
	if err != nil {
		return errors.New("não consegui descobrir o caminho do programa")
	}
	return registerLinkHandler(exe)
}
