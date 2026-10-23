//go:build windows

package tun

import (
	"net"
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
	return recoverPlatform()
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

func configurePlatform(string) error { return nil }

// recoverPlatform 清掉上次没清干净的 TUN 地址（sing-box 被强杀、断电、蓝屏时会残留）。
// 只动 sbtun 自己的网卡：现在的 sbtun，以及旧版用过、且已经不在工作状态的 tun0
// （活动中的 tun0 可能属于别的客户端）。只在没有实例运行时调用。
func recoverPlatform() error {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range interfaces {
		if iface.Name != InterfaceName && !(strings.EqualFold(iface.Name, "tun0") && iface.Flags&net.FlagUp == 0) {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ipNet, ok := address.(*net.IPNet)
			if !ok {
				continue
			}
			if ip4 := ipNet.IP.To4(); ip4 != nil && ip4[0] == 172 && ip4[1] >= 18 && ip4[1] <= 31 {
				_ = runWindows("netsh", "interface", "ipv4", "delete", "address", "name="+iface.Name, "address="+ip4.String())
			} else if ip4 == nil && strings.HasPrefix(ipNet.IP.String(), "fdfe:dcba:9876:") {
				_ = runWindows("netsh", "interface", "ipv6", "delete", "address", "interface="+iface.Name, "address="+ipNet.IP.String())
			}
		}
	}
	return nil
}
