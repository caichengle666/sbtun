package app

import (
	"fmt"
	"os"
	"time"
)

const configWatchInterval = time.Second

// watchConfigFile 让运行中的实例（GUI 或 sbtun run）应用别的进程对 config.json 和规则集清单 sets.json 的修改，
// 比如另开终端执行 sbtun custom add / groups add / route / rules add。生成的 sing-box 配置没变时不重启，
// 所以自己保存触发的检测是无害的。
func (a *App) watchConfigFile() {
	if a.ctx == nil || a.manager == nil {
		return
	}
	last := a.watchStamp()
	ticker := time.NewTicker(configWatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
		}
		stamp := a.watchStamp()
		if stamp == last {
			continue
		}
		last = stamp
		if err := a.applyExternalConfig(); err != nil {
			fmt.Fprintln(os.Stderr, "应用外部配置修改失败:", err)
		}
	}
}

func (a *App) applyExternalConfig() error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if a.closing || a.runtime == nil || a.runtime.State == nil || !a.runtime.State.IsRunning() {
		return nil
	}
	cfg, err := a.manager.Load()
	if err != nil {
		return err
	}
	return a.applyRuntimeConfigLocked(cfg, true, true)
}

func (a *App) watchStamp() string {
	stamp := configStamp(a.manager.Path)
	if a.rulesManager != nil {
		stamp += "|" + configStamp(a.rulesManager.StatePath())
	}
	return stamp
}

// configStamp 用修改时间和大小判断文件是否被改过；读不到时返回空串。
func configStamp(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size())
}
