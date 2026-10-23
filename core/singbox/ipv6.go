package singbox

import (
	"fmt"
	"net"
	"runtime"

	"github.com/caichengle666/sbtun/core/tun"
)

const tunIPv6Address = "fdfe:dcba:9876::1/126"

// HasUsableIPv6 reports whether an active non-loopback interface has a usable
// IPv6 address. The sbtun TUN address is excluded so reloads cannot self-detect.
func HasUsableIPv6() bool {
	tunPrefix := &net.IPNet{
		IP:   net.ParseIP("fdfe:dcba:9876::"),
		Mask: net.CIDRMask(48, 128),
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip != nil && ip.To4() == nil && ip.IsGlobalUnicast() && !ip.IsLinkLocalUnicast() && !tunPrefix.Contains(ip) {
				return true
			}
		}
	}
	return false
}

func tunAddresses(ipv6Enabled bool) []string {
	addresses := []string{tunIPv4Address(localIPv4s())}
	if ipv6Enabled && HasUsableIPv6() {
		addresses = append(addresses, tunIPv6Address)
	}
	return addresses
}

// tunIPv4Address 默认用 172.18.0.1/30。Windows / Linux 上这个地址可能已被别的网卡占着
// （v2rayN 等同样基于 sing-box 的客户端默认也用它，或上次异常退出残留的网卡），
// sing-box 会报 “set ipv4 address: The object already exists” 直接退出；这时顺延到
// 172.19.0.1、172.20.0.1……第一个空闲的 /30。macOS 的路由修正依赖固定地址，不换。
func tunIPv4Address(used []net.IP) string {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return "172.18.0.1/30"
	}
	for second := 18; second <= 31; second++ {
		_, block, _ := net.ParseCIDR(fmt.Sprintf("172.%d.0.0/30", second))
		free := true
		for _, ip := range used {
			if block.Contains(ip) {
				free = false
				break
			}
		}
		if free {
			return fmt.Sprintf("172.%d.0.1/30", second)
		}
	}
	return "172.18.0.1/30"
}

// localIPv4s 列出本机网卡（包括未连接的）上的 IPv4 地址，跳过 sbtun 自己的 TUN 网卡，
// 否则运行中重载配置会因为看到自己的地址而换地址、导致无谓重启。
func localIPv4s() []net.IP {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var ips []net.IP
	for _, iface := range interfaces {
		if iface.Name == tun.InterfaceName {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			if value, ok := address.(*net.IPNet); ok && value.IP.To4() != nil {
				ips = append(ips, value.IP)
			}
		}
	}
	return ips
}
