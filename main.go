package main

import (
	"embed"

	appsvc "markvault/internal/app"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := appsvc.New()

	err := wails.Run(&options.App{
		Title:            "Markvault",
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 27, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
