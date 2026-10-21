//go:build android

package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core/singbox"
)

// StartupMobile 初始化安卓版：dataDir 是应用私有目录，binary 是 APK 里带的 sing-box（只读、可执行）。
// 规则文件放在 dataDir/rules。
func (a *App) StartupMobile(ctx context.Context, dataDir, binary string) {
	// 安卓默认临时目录 /data/local/tmp 应用写不了。
	tmp := filepath.Join(dataDir, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	_ = os.Setenv("TMPDIR", tmp)
	a.ctx = ctx
	a.workDir = filepath.Join(dataDir, "runtime-data")
	a.binary = binary
	a.manager = config.NewManager(filepath.Join(a.workDir, "config.json"))
	a.initRuntime(dataDir)
	go a.monitorNodes()
	go a.autoBackupLoop()
	go a.subscriptionUpdateLoop()
}

// MobileProxyURL 是 VPN 转发流量用的本地 SOCKS5 地址（sing-box 的 mixed 入站，带本地认证）。
func (a *App) MobileProxyURL() string {
	u := url.URL{Scheme: "socks5", Host: fmt.Sprintf("127.0.0.1:%d", singbox.MobileProxyPort)}
	if a.runtime != nil && a.runtime.LocalAuth.ProxyUsername != "" {
		u.User = url.UserPassword(a.runtime.LocalAuth.ProxyUsername, a.runtime.LocalAuth.ProxyPassword)
	}
	return u.String()
}
