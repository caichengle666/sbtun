//go:build windows

package capture

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func installCertificate(path string) error {
	return runCertificateCommand("certutil", "-addstore", "-f", "Root", path)
}

func uninstallCertificate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("分析 CA 证书格式无效")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	sum := sha1.Sum(cert.Raw)
	return runCertificateCommand("certutil", "-delstore", "Root", strings.ToUpper(hex.EncodeToString(sum[:])))
}

func runCertificateCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("系统证书操作失败: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
