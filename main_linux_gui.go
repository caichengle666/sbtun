//go:build linux && !cli

package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/caichengle666/sbtun/app"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed frontend/dist
var assets embed.FS

//go:embed resources/icon.png
var trayIcon []byte

func main() {
	release, err := app.AcquireSingleInstance()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer release()

	application := app.New()
	application.SetTrayIcon(trayIcon)
	appOptions := &options.App{
		Title:             "sbtun",
		Width:             980,
		Height:            680,
		MinWidth:          760,
		MinHeight:         520,
		BackgroundColour:  &options.RGBA{R: 13, G: 14, B: 21, A: 255},
		HideWindowOnClose: true,
		AssetServer:       &assetserver.Options{Assets: assets},
		OnStartup:         application.Startup,
		OnShutdown:        application.Shutdown,
		Bind:              []interface{}{application},
	}
	configurePlatformOptions(appOptions)
	if err := wails.Run(appOptions); err != nil {
		panic(err)
	}
}
