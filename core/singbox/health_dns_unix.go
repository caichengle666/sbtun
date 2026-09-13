//go:build !windows

package singbox

// Use an IP-based DoH endpoint so URL health checks do not depend on a
// system resolver being configured on Linux.
func healthDNSConfig() map[string]any {
	return map[string]any{
		"servers": []map[string]any{{
			"type":        "https",
			"tag":         "dns-health",
			"server":      "1.1.1.1",
			"server_port": 443,
			"path":        "/dns-query",
			"tls":         map[string]any{"enabled": true, "server_name": "cloudflare-dns.com"},
			"detour":      "direct",
		}},
		"final": "dns-health",
	}
}
