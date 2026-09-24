//go:build linux

package capture

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const systemCertificatePath = "/usr/local/share/ca-certificates/sbtun-capture-ca.crt"

func userCertificateSupported() bool { return false }

func installCertificate(path, level string) error {
	if level == "user" {
		return fmt.Errorf("Linux 暂不支持自动安装用户级证书，请安装系统级证书或手动导入")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(systemCertificatePath, data, 0o644); err != nil {
		return fmt.Errorf("写入系统证书失败，请使用管理员权限运行: %w", err)
	}
	return runUpdateCertificates()
}

func uninstallCertificate(path, level string) error {
	if level == "user" {
		return fmt.Errorf("Linux 暂不支持自动卸载用户级证书")
	}
	if err := os.Remove(systemCertificatePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return runUpdateCertificates()
}

func runUpdateCertificates() error {
	cmd := exec.Command("update-ca-certificates")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("更新系统证书失败: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
