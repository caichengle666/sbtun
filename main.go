package main

import (
	"embed"

	"github.com/caichengle666/sbtun/app"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed frontend/dist
var assets embed.FS

func main() {
	releaseInstance, err := app.AcquireSingleInstance()
	if err != nil {
		app.ShowSingleInstanceMessage()
		return
	}
	defer releaseInstance()

	application := app.New()

	err = wails.Run(&options.App{
		Title:             "sbtun",
		Width:             980,
		Height:            680,
		MinWidth:          760,
		MinHeight:         520,
		BackgroundColour:  &options.RGBA{R: 13, G: 14, B: 21, A: 255},
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  application.Startup,
		OnShutdown: application.Shutdown,
		Bind:       []interface{}{application},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})
	if err != nil {
		panic(err)
	}
}
