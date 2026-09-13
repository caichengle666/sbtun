//go:build windows

package tun

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func cleanupPlatform(name string) error {
	var firstErr error
	if err := runWindows("route", "DELETE", "0.0.0.0", "MASK", "0.0.0.0", "172.18.0.1"); err != nil {
		firstErr = err
	}
	_ = runWindows("route", "DELETE", "172.18.0.0", "MASK", "255.255.255.252", "172.18.0.1")
	_ = runWindows("netsh", "interface", "ipv4", "set", "address", "name="+name, "source=dhcp")
	_ = runWindows("netsh", "interface", "set", "interface", name, "admin=disabled")
	_ = runWindows("netsh", "interface", "set", "interface", name, "admin=enabled")
	return firstErr
}

func runWindows(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Run()
}

func interfaceExists(name string) bool {
	cmd := exec.Command("netsh", "interface", "ipv4", "show", "interfaces")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if _, err := strconv.Atoi(fields[0]); err != nil {
			continue
		}
		candidate := strings.TrimSpace(strings.Join(fields[4:], " "))
		if strings.EqualFold(candidate, name) {
			return true
		}
	}
	return false
}
