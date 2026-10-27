//go:build !android

package app

import (
	"context"
	"net"
)

// 桌面版本机流量都经过 TUN，直接连就会走 sing-box 的路由。
func (a *App) routeTestDial(ctx context.Context, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", address)
}
