package main

import (
	_ "embed"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed build/appicon.png
var trayIcon []byte

// setupTray cria o ícone na bandeja do sistema: clique alterna a janela; o menu tem Mostrar e Sair.
func (a *TermService) setupTray() {
	tray := a.app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("NxATerm")
	menu := a.app.Menu.New()
	menu.Add("Mostrar o NxATerm").OnClick(func(*application.Context) { a.showWindow() })
	menu.AddSeparator()
	menu.Add("Sair").OnClick(func(*application.Context) { a.quit() })
	tray.SetMenu(menu)
	tray.OnClick(func() { a.toggleWindow() })
}

// quit encerra de verdade (o "fechar para a bandeja" não segura a saída).
func (a *TermService) quit() {
	a.quitting.Store(true)
	a.app.Quit()
}
