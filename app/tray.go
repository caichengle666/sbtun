package app

import "github.com/wailsapp/wails/v2/pkg/runtime"

func (a *App) HideToTray() {
	if a.ctx != nil {
		runtime.WindowHide(a.ctx)
	}
}

func (a *App) ShowWindow() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
		runtime.WindowUnminimise(a.ctx)
	}
}

func (a *App) MinimizeToTray() {
	a.HideToTray()
}

func (a *App) CloseToTray() {
	a.HideToTray()
}

func (a *App) QuitApp() {
	// Let Wails invoke OnShutdown so the proxy and TUN state are cleaned up.
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}
