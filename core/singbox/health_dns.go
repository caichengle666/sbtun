package singbox

import "runtime"

// URL 测速用系统 DNS。安卓上 sing-box 作为独立进程运行，没有 /etc/resolv.conf，
// "local" 会去问 [::1]:53 而失败，所以改用和运行配置相同的国内公共 DNS。
func healthDNSConfig() map[string]any {
	if runtime.GOOS == "android" {
		return map[string]any{
			"servers": []map[string]any{
				{"type": "udp", "tag": "dns-local", "server": "223.5.5.5", "server_port": 53},
				{"type": "udp", "tag": "dns-local-standby", "server": "119.29.29.29", "server_port": 53},
			},
			"final": "dns-local",
		}
	}
	return map[string]any{
		"servers": []map[string]any{{"type": "local", "tag": "dns-local"}},
		"final":   "dns-local",
	}
}
