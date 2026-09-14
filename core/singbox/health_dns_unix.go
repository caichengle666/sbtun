//go:build !windows

package singbox

// Use the system resolver for URL health checks on Linux, matching Windows
// and avoiding hard-coded public DNS endpoints that may be unreachable.
func healthDNSConfig() map[string]any {
	return map[string]any{
		"servers": []map[string]any{{"type": "local", "tag": "dns-local"}},
		"final":   "dns-local",
	}
}
