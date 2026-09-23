//go:build windows

package tun

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func cleanupPlatform(name string) error {
	_ = cleanupRoutesPlatform(name)
	_ = runWindows("netsh", "interface", "ipv4", "set", "address", "name="+name, "source=dhcp")
	_ = runWindows("netsh", "interface", "set", "interface", name, "admin=disabled")
	_ = runWindows("netsh", "interface", "set", "interface", name, "admin=enabled")
	return nil
}

func cleanupRoutesPlatform(name string) error {
	index, found := interfaceIndex(name)
	if found {
		// sing-box creates 172.18.0.2 as the TUN next hop; deleting by
		// interface index also removes routes without relying on that address.
		_ = runWindows("route", "DELETE", "0.0.0.0", "MASK", "0.0.0.0", "IF", index)
		_ = runWindows("route", "DELETE", "172.18.0.0", "MASK", "255.255.255.252", "IF", index)
		_ = runWindows("netsh", "interface", "ipv6", "delete", "route", "::/0", "interface="+index)
		_ = runWindows("netsh", "interface", "ipv6", "delete", "route", "fdfe:dcba:9876::/126", "interface="+index)
	}
	_ = runWindows("netsh", "interface", "ipv4", "set", "dnsservers", "name="+name, "source=dhcp")
	_ = runWindows("netsh", "interface", "ipv6", "delete", "address", "interface="+name, "address=fdfe:dcba:9876::1")
	return nil
}

func runWindows(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Run()
}

func interfaceExists(name string) bool {
	_, found := interfaceIndex(name)
	return found
}

func interfaceIndex(name string) (string, bool) {
	cmd := exec.Command("netsh", "interface", "ipv4", "show", "interfaces")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
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
			return fields[0], true
		}
	}
	return "", false
}
