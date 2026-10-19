//go:build darwin

package app

// fyne systray needs the main thread on macOS, which Wails already owns, so
// the macOS build has no menu bar icon; the Dock icon and window are used.
func (a *App) StartTray() {}
