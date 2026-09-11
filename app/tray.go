package app

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/caichengle666/sbtun/core"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// trayIconBase64 is a Windows ICO file encoded as base64.
const trayIconBase64 = "AAABAAEAQEAAAAEAIABkAgAAFgAAAIlQTkcNChoKAAAADUlIRFIAAABAAAAAQAgGAAAAqmlx3gAAAitJREFUeJztk9tHpGEcx/s7IiKii4iIiOgiIiIiuliWZVli6SIiIiK6iIiIiC5iiaU222HVbplmmmqa6TCdptN0mg47NZ3zzRMva+dp5u2d932f9/k9ffjc/z5ffmlp77wjlMF0F0TfYDlDGTMwqujbDfMj0w2zFd2UlOEsD+xSdGscP7NnYbeim18YyfFCtELCR3O9cJq2xY/lzcGpWh4/nj8Pp2tZ/K+CBcii6fEThT7Ipmnxk0WLkNWU438X+yG7KQ3wp8QP2TUcP1UaABUNDTBdtgQqvjneVb4MauqOn6lYAVV1DeCuXAVVdQ3gqQqCqknjZ6uDoG7CAbw1a6BuwgHmP6yDuq/GL3zcgCpyB/B92oQqcgdY/LwFVeQO4P8SgipyBwjUhqCK3AGWv25DFbkDrNTtQBXj4lfrd6GacSMEG/agitwXWGvchypyB9hoCkMVuQNsNoehitwBtloOoIrcAUKth1BF7gDbbUdQRe4AjJ32Y1D31XjGXscJqJtwgP3OCKibcABGuCsCqiaNZxx0n4KqugY47DkDVXUNwDjqPQc1dcdrnPRdgIpvjmdE+v+CioYGYJx+i0J2DcczzgaikN2UBmCcf7+ErKYcr3ExeAXZNC1eIzocgyyaHq9xOXINp2tZvMbV+A2cquXxGrHJWzhN2+L/5XrqFqIVEv4/N6472K3o5jjuPPewS9GtSbmfe4DZim4yzIPvEUYVfbvlPAae6Ec6mWdkkz46WDxBsQAAAABJRU5ErkJggg=="

var (
	trayOnce     sync.Once
	trayActionMu sync.Mutex
)

func (a *App) StartTray() {
	trayOnce.Do(func() {
		goruntime.LockOSThread()
		defer goruntime.UnlockOSThread()
		a.trayEnd = systray.Quit
		systray.Run(func() {
			icon, err := base64.StdEncoding.DecodeString(trayIconBase64)
			if err == nil {
				iconPath := filepath.Join(os.TempDir(), "sbtun-tray.ico")
				if os.WriteFile(iconPath, icon, 0o600) == nil {
					_ = systray.SetIconFromFilePath(iconPath)
				}
			}
			systray.SetTitle("sbtun")
			systray.SetTooltip("sbtun")

			showItem := systray.AddMenuItem("显示主窗口", "显示 sbtun 主窗口")
			systray.AddSeparator()
			startItem := systray.AddMenuItem("开启 TUN", "开启 TUN 代理")
			stopItem := systray.AddMenuItem("关闭 TUN", "关闭 TUN 代理")
			stopItem.Disable()
			systray.AddSeparator()
			nodeMenu := systray.AddMenuItem("切换节点", "选择当前代理节点")
			runningNodeItem := nodeMenu.AddSubMenuItem("当前运行节点：未启动", "当前实际运行的代理节点")
			runningNodeItem.Disable()
			nodeMenu.AddSeparator()
			nodeItems := make(map[string]*systray.MenuItem)
			nodeNames := make(map[string]string)
			if cfg, err := a.manager.Load(); err == nil {
				for _, node := range cfg.Nodes {
					nodeID := node.ID
					item := nodeMenu.AddSubMenuItemCheckbox(node.Name, node.Server, node.ID == cfg.CurrentNodeID)
					nodeItems[nodeID] = item
					nodeNames[nodeID] = node.Name
					go func() {
						for range item.ClickedCh {
							a.runTrayAction(func() { _ = a.SelectNode(nodeID) })
						}
					}()
				}
			}
			systray.AddSeparator()
			quitItem := systray.AddMenuItem("退出", "退出 sbtun 并关闭代理")

			// 菜单事件处理
			go func() {
				for {
					select {
					case <-showItem.ClickedCh:
						a.ShowWindow()
					case <-startItem.ClickedCh:
						go a.runTrayAction(func() {
							if err := a.Start(); err != nil {
								a.showTrayError(err)
							}
						})
					case <-stopItem.ClickedCh:
						go a.runTrayAction(func() {
							if err := a.Stop(); err != nil {
								a.showTrayError(err)
							}
						})
					case <-quitItem.ClickedCh:
						a.QuitApp()
						return
					}
				}
			}()

			// 状态同步
			go func() {
				for {
					state, _ := a.runtime.State.Get()
					up, down := a.refreshTraffic()
					currentNodeID := ""
					if cfg, err := a.manager.Load(); err == nil {
						currentNodeID = cfg.CurrentNodeID
						for nodeID, item := range nodeItems {
							if nodeID == currentNodeID {
								item.Check()
							} else {
								item.Uncheck()
							}
						}
					}
					switch state {
					case core.StateRunning:
						startItem.Disable()
						stopItem.Enable()
						runningNodeItem.SetTitle(fmt.Sprintf("当前运行节点：%s", trayNodeName(nodeNames, currentNodeID)))
						systray.SetTooltip(fmt.Sprintf("sbtun | 运行中 | 上行 %s/s | 下行 %s/s", formatTraffic(up), formatTraffic(down)))
					case core.StateError:
						startItem.Enable()
						stopItem.Disable()
						runningNodeItem.SetTitle("当前运行节点：异常停止")
						systray.SetTooltip("sbtun | 代理异常")
					default:
						startItem.Enable()
						stopItem.Disable()
						runningNodeItem.SetTitle("当前运行节点：未启动")
						systray.SetTooltip("sbtun | 已停止")
					}
					select {
					case <-time.After(time.Second):
					}
				}
			}()
		}, func() {})
	})
}

func trayNodeName(nodes map[string]string, id string) string {
	if name := nodes[id]; name != "" {
		return name
	}
	return "未选择"
}

func formatTraffic(value uint64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	if value < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(value)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(value)/(1024*1024))
}

func (a *App) runTrayAction(action func()) {
	trayActionMu.Lock()
	defer trayActionMu.Unlock()
	action()
}

func (a *App) showTrayError(err error) {
	if err == nil || a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

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
