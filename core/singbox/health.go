package singbox

import (
	"encoding/json"

	"github.com/caichengle666/sbtun/config"
)

// BuildHealthConfig creates a temporary mixed-inbound config for a real URL probe.
func BuildHealthConfig(node config.Node, listenPort int) ([]byte, error) {
	proxy, err := buildOutbound(node)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"log": map[string]any{"level": "error", "timestamp": true},
		"inbounds": []map[string]any{{
			"type": "mixed", "tag": "health-in",
			"listen": "127.0.0.1", "listen_port": listenPort,
		}},
		"outbounds": []map[string]any{proxy, {"type": "direct", "tag": "direct"}, {"type": "block", "tag": "block"}},
		"route":     map[string]any{"final": "proxy"},
	}
	return json.MarshalIndent(result, "", "  ")
}
