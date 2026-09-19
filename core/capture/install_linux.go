//go:build linux

package capture

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const systemCertificatePath = "/usr/local/share/ca-certificates/sbtun-capture-ca.crt"

func installCertificate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(systemCertificatePath, data, 0o644); err != nil {
		return fmt.Errorf("写入系统证书失败，请使用管理员权限运行: %w", err)
	}
	return runUpdateCertificates()
}

func uninstallCertificate(path string) error {
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
