package singbox

import (
	"encoding/json"

	"github.com/caichengle666/sbtun/config"
)

// BuildHealthConfig creates a temporary mixed-inbound config for a real URL
// probe. The inbound requires username/password so other local programs cannot
// use it as an open proxy.
func BuildHealthConfig(node config.Node, listenPort int, username, password string) ([]byte, error) {
	proxy, err := buildOutbound(node)
	if err != nil {
		return nil, err
	}
	outbounds, endpoints := splitEndpoints([]map[string]any{proxy})
	result := map[string]any{
		"log": map[string]any{"level": "error", "timestamp": true},
		"dns": healthDNSConfig(),
		"inbounds": []map[string]any{{
			"type": "mixed", "tag": "health-in",
			"listen": "127.0.0.1", "listen_port": listenPort,
			"users": []map[string]any{{"username": username, "password": password}},
		}},
		"outbounds": append(outbounds, map[string]any{"type": "direct", "tag": "direct"}, map[string]any{"type": "block", "tag": "block"}),
		"route":     map[string]any{"final": "proxy", "default_domain_resolver": "dns-local"},
	}
	if len(endpoints) > 0 {
		result["endpoints"] = endpoints
	}
	return json.MarshalIndent(result, "", "  ")
}
