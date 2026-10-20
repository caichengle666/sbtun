//go:build android

package app

// 安卓没有托盘和桌面窗口，界面由 APK 的 WebView 提供。

func (a *App) StartTray()      {}
func (a *App) HideToTray()     {}
func (a *App) ShowWindow()     {}
func (a *App) MinimizeToTray() {}
func (a *App) CloseToTray()    {}

// QuitApp 在安卓上只停止代理，进程由系统管理。
func (a *App) QuitApp() { _ = a.Stop() }
