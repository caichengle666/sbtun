//go:build !windows && !linux

package capture

import "fmt"

func userCertificateSupported() bool { return false }

func installCertificate(path, level string) error {
	return fmt.Errorf("当前系统暂不支持自动安装分析证书")
}
func uninstallCertificate(path, level string) error {
	return fmt.Errorf("当前系统暂不支持自动卸载分析证书")
}
