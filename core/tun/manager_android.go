//go:build android

package tun

import "errors"

// 安卓上 TUN 由系统 VpnService 创建，sing-box 只开本地代理入站。
// APK 启动时设置 Platform：Start 建立 VPN 并把流量转到 sing-box，Stop 关闭 VPN。
// Start 要能重复调用（重载配置时 VPN 保持不断）。
type AndroidPlatform interface {
	Start() error
	Stop() error
}

var Platform AndroidPlatform

// ReadyProbe 由 APK 设置：sing-box 的本地入站可连接时返回 true。没有 TUN 网卡可等，
// 就绪判断改为等入站端口，免得 VPN 在内核监听前就接管流量。
var ReadyProbe func() bool

func interfaceExists(string) bool { return ReadyProbe == nil || ReadyProbe() }

func configurePlatform(string) error {
	if Platform == nil {
		return errors.New("安卓 VPN 未初始化")
	}
	return Platform.Start()
}

func cleanupPlatform(string) error {
	if Platform == nil {
		return nil
	}
	return Platform.Stop()
}

func cleanupRoutesPlatform(string) error { return nil }

func recoverPlatform() error { return nil }
