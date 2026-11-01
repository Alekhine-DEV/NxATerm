package main

import (
	"embed"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"nxaterm/internal/launch"
	"nxaterm/internal/secfile"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

// startupSettings são as configurações que precisam valer antes da janela existir.
type startupSettings struct {
	MultiInstance     bool   `json:"multiInstance"`
	WindowTransparent bool   `json:"windowTransparent"`
	Tray              bool   `json:"tray"`         // ícone na bandeja do sistema
	CloseToTray       bool   `json:"closeToTray"`  // fechar a janela só esconde
	GlobalHotkey      string `json:"globalHotkey"` // mostra/esconde a janela
}

func readStartupSettings() startupSettings {
	var v startupSettings
	if dir, err := dataDir(); err == nil {
		// startup.json (em texto) existe quando as configurações são criptografadas
		if data, err := os.ReadFile(filepath.Join(dir, "startup.json")); err == nil && json.Unmarshal(data, &v) == nil {
			return v
		}
		if data, err := secfile.Read(filepath.Join(dir, "settings.json")); err == nil {
			json.Unmarshal(data, &v)
		}
	}
	return v
}

func main() {
	pendingLaunch.items = launch.Collect(os.Args[1:])
	cfg := readStartupSettings()

	var svc *TermService
	opts := application.Options{
		Name:        "NxATerm",
		Description: "Gerenciador de sessões SSH com terminal e SFTP",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	}
	if !cfg.MultiInstance {
		opts.SingleInstance = &application.SingleInstanceOptions{
			UniqueID: "dev.alekhine.nxaterm",
			OnSecondInstanceLaunch: func(d application.SecondInstanceData) {
				if svc != nil {
					svc.onSecondInstance(d.Args)
				}
			},
		}
	}
	app := application.New(opts)

	var err error
	svc, err = NewTermService(app)
	if err != nil {
		log.Fatal(err)
	}
	app.RegisterService(application.NewService(svc))

	winOpts := application.WebviewWindowOptions{
		Title:            "NxATerm",
		Width:            1280,
		Height:           800,
		MinWidth:         900,
		MinHeight:        500,
		BackgroundColour: application.NewRGB(24, 25, 30),
		URL:              "/",
		EnableFileDrop:   true,
	}
	if cfg.WindowTransparent {
		// janela transparente: a interface define a opacidade (só vale ao reabrir o app)
		winOpts.BackgroundType = application.BackgroundTypeTransparent
		winOpts.BackgroundColour = application.NewRGBA(0, 0, 0, 0)
	}
	win := app.Window.NewWithOptions(winOpts)

	// arquivos arrastados do sistema para a janela: o frontend decide o que fazer
	// conforme o elemento (data-file-drop-target) em que foram soltos
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		target := ""
		if d := event.Context().DropTargetDetails(); d != nil {
			target = d.ElementID
		}
		svc.emitJSON("files:dropped", map[string]any{"files": event.Context().DroppedFiles(), "target": target})
	})

	svc.showWindow = func() {
		win.Show()
		win.Restore()
		win.Focus()
	}
	svc.toggle = func() {
		if win.IsVisible() && !win.IsMinimised() {
			win.Hide()
			return
		}
		svc.showWindow()
	}

	if cfg.Tray {
		svc.setupTray()
		if cfg.CloseToTray {
			win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
				if !svc.quitting.Load() {
					e.Cancel()
					win.Hide()
				}
			})
		}
	}
	if cfg.GlobalHotkey != "" {
		if err := svc.SetGlobalHotkey(cfg.GlobalHotkey); err != nil {
			log.Printf("atalho global: %v", err)
		}
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
