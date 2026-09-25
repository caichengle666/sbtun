package singbox

import "net"

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
	addresses := []string{"172.18.0.1/30"}
	if ipv6Enabled && HasUsableIPv6() {
		addresses = append(addresses, tunIPv6Address)
	}
	return addresses
}
