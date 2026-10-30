package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := application.New(application.Options{
		Name:        "NxATerm",
		Description: "Gerenciador de sessões SSH com terminal e SFTP",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	svc, err := NewTermService(app)
	if err != nil {
		log.Fatal(err)
	}
	app.RegisterService(application.NewService(svc))

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "NxATerm",
		Width:            1280,
		Height:           800,
		MinWidth:         900,
		MinHeight:        500,
		BackgroundColour: application.NewRGB(24, 25, 30),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
