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

func interfaceExists(string) bool { return true }

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
