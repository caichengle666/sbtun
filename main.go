package main

import (
	"embed"

	"github.com/caichengle666/sbtun/app"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
)

//go:embed frontend/dist
var assets embed.FS

func main() {
	application := app.New()

	err := wails.Run(&options.App{
		Title:             "sbtun",
		Width:             980,
		Height:            680,
		MinWidth:          760,
		MinHeight:         520,
		BackgroundColour:  &options.RGBA{R: 13, G: 14, B: 21, A: 1},
		AssetServer:       options.AssetServer{Assets: assets},
		OnStartup:         application.Startup,
		Bind:              []interface{}{application},
		Windows: options.Windows{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:   false,
		},
	})
	if err != nil {
		panic(err)
	}
}
