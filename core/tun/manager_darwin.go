//go:build darwin

package tun

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// sing-box creates utunN with 172.18.0.1/30 and routes the default ranges to
// the peer 172.18.0.2. macOS keeps the more specific LAN route for a router
// DNS server, so DNS would bypass TUN; every network service is pointed at the
// peer address while TUN runs, where the hijack-dns rule answers it.
const (
	tunIPv4      = "172.18.0.1"
	tunDNSServer = "172.18.0.2"
	dnsBackup    = "/Library/Application Support/sbtun/dns-backup.json"
)

// macOS names TUN devices utunN itself, so the adapter is found by address.
func interfaceExists(string) bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		if !strings.HasPrefix(iface.Name, "utun") {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.String() == tunIPv4 {
				return true
			}
		}
	}
	return false
}

func configurePlatform(string) error {
	services, err := networkServices()
	if err != nil {
		return err
	}
	// Keep an existing backup: it holds the user's DNS from before a run that
	// did not get to restore it.
	if _, err := os.Stat(dnsBackup); errors.Is(err, os.ErrNotExist) {
		original := make(map[string][]string, len(services))
		for _, service := range services {
			servers, err := dnsServers(service)
			if err != nil {
				return err
			}
			original[service] = servers
		}
		data, _ := json.Marshal(original)
		if err := os.MkdirAll(filepath.Dir(dnsBackup), 0o755); err != nil {
			return fmt.Errorf("保存系统 DNS 失败: %w", err)
		}
		if err := os.WriteFile(dnsBackup, data, 0o600); err != nil {
			return fmt.Errorf("保存系统 DNS 失败: %w", err)
		}
	}
	for _, service := range services {
		if err := setDNSServers(service, []string{tunDNSServer}); err != nil {
			return err
		}
	}
	flushDNSCache()
	return nil
}

// sing-box removes its utun device and routes when it exits; only the DNS
// settings sbtun changed need restoring.
func cleanupPlatform(string) error { return restoreDNS() }

// A reload keeps DNS pointed at TUN; the new sing-box answers on the same address.
func cleanupRoutesPlatform(string) error { return nil }

func recoverPlatform() error { return restoreDNS() }

func restoreDNS() error {
	data, err := os.ReadFile(dnsBackup)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取系统 DNS 备份失败: %w", err)
	}
	var original map[string][]string
	if err := json.Unmarshal(data, &original); err != nil {
		_ = os.Remove(dnsBackup)
		return fmt.Errorf("系统 DNS 备份已损坏: %w", err)
	}
	var failed []string
	for service, servers := range original {
		if err := setDNSServers(service, servers); err != nil {
			failed = append(failed, service)
		}
	}
	flushDNSCache()
	if len(failed) > 0 {
		return fmt.Errorf("恢复系统 DNS 失败: %s", strings.Join(failed, ", "))
	}
	return os.Remove(dnsBackup)
}

func networkServices() ([]string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, fmt.Errorf("读取网络服务失败: %w", err)
	}
	return parseNetworkServices(string(out)), nil
}

// The first line is a notice and disabled services start with '*'.
func parseNetworkServices(output string) []string {
	var services []string
	for i, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if i == 0 || line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, line)
	}
	return services
}

func dnsServers(service string) ([]string, error) {
	out, err := exec.Command("networksetup", "-getdnsservers", service).Output()
	if err != nil {
		return nil, fmt.Errorf("读取 %s 的 DNS 失败: %w", service, err)
	}
	return parseDNSServers(string(out)), nil
}

// "There aren't any DNS Servers set on Wi-Fi." means DHCP-provided DNS.
func parseDNSServers(output string) []string {
	var servers []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if net.ParseIP(line) != nil {
			servers = append(servers, line)
		}
	}
	return servers
}

func setDNSServers(service string, servers []string) error {
	args := []string{"-setdnsservers", service}
	if len(servers) == 0 {
		args = append(args, "Empty")
	} else {
		args = append(args, servers...)
	}
	if out, err := exec.Command("networksetup", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("设置 %s 的 DNS 失败: %w: %s", service, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func flushDNSCache() {
	_ = exec.Command("dscacheutil", "-flushcache").Run()
	_ = exec.Command("killall", "-HUP", "mDNSResponder").Run()
}
