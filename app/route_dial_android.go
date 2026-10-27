//go:build android

package app

import (
	"context"
	"fmt"
	"net"

	"github.com/caichengle666/sbtun/core/singbox"
	"golang.org/x/net/proxy"
)

// 安卓上 App 自身不走 VPN，测试连接要经 sing-box 的本地 SOCKS5 入站。
func (a *App) routeTestDial(ctx context.Context, address string) (net.Conn, error) {
	var auth *proxy.Auth
	if a.runtime != nil && a.runtime.LocalAuth.ProxyUsername != "" {
		auth = &proxy.Auth{User: a.runtime.LocalAuth.ProxyUsername, Password: a.runtime.LocalAuth.ProxyPassword}
	}
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", singbox.MobileProxyPort), auth, proxy.Direct)
	if err != nil {
		return nil, err
	}
	return dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", address)
}
