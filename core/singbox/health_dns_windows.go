//go:build windows

package singbox

func healthDNSConfig() map[string]any {
	return map[string]any{
		"servers": []map[string]any{{"type": "local", "tag": "dns-local"}},
		"final":   "dns-local",
	}
}
