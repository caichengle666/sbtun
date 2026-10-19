//go:build darwin

package capture

import (
	"fmt"
	"os/exec"
	"strings"
)

const systemKeychain = "/Library/Keychains/System.keychain"

func userCertificateSupported() bool { return false }

func installCertificate(path, level string) error {
	if level == "user" {
		return fmt.Errorf("macOS 只支持安装到系统钥匙串")
	}
	return runSecurity("add-trusted-cert", "-d", "-r", "trustRoot", "-k", systemKeychain, path)
}

func uninstallCertificate(path, level string) error {
	if level == "user" {
		return fmt.Errorf("macOS 只支持从系统钥匙串卸载")
	}
	_ = runSecurity("remove-trusted-cert", "-d", path)
	return runSecurity("delete-certificate", "-c", "sbtun Local Capture CA", systemKeychain)
}

func runSecurity(args ...string) error {
	if out, err := exec.Command("security", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("security %s 失败: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
