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
	show := menu.Add(tr("Mostrar o NxATerm"))
	show.OnClick(func(*application.Context) { a.showWindow() })
	menu.AddSeparator()
	quit := menu.Add(tr("Sair"))
	quit.OnClick(func(*application.Context) { a.quit() })
	tray.SetMenu(menu)
	a.trayMu.Lock()
	a.trayMenu, a.trayShow, a.trayQuit = menu, show, quit
	a.trayMu.Unlock()
	tray.OnClick(func() { a.toggleWindow() })
}

// retranslateTray troca os textos do menu da bandeja quando o idioma muda.
func (a *TermService) retranslateTray() {
	a.trayMu.Lock()
	defer a.trayMu.Unlock()
	if a.trayMenu == nil {
		return
	}
	a.trayShow.SetLabel(tr("Mostrar o NxATerm"))
	a.trayQuit.SetLabel(tr("Sair"))
	a.trayMenu.Update()
}

// quit encerra de verdade (o "fechar para a bandeja" não segura a saída).
func (a *TermService) quit() {
	a.quitting.Store(true)
	a.app.Quit()
}
