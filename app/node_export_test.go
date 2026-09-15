package app

import (
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestNodeShareLinksRoundTrip(t *testing.T) {
	nodes := []config.Node{
		{ID: "vmess", Name: "VMess 测试", Protocol: "vmess", Server: "example.com", Port: 443, Settings: map[string]string{"uuid": "uuid", "alter_id": "0", "security": "auto", "tls": "true", "server_name": "tls.example.com", "transport_type": "ws", "transport_host": "cdn.example.com", "transport_path": "/ws"}},
		{ID: "vless", Name: "VLESS 测试", Protocol: "vless", Server: "example.com", Port: 443, Settings: map[string]string{"uuid": "uuid", "tls": "true", "server_name": "tls.example.com", "transport_type": "grpc", "transport_service_name": "proxy", "reality_public_key": "public", "reality_short_id": "short"}},
		{ID: "trojan", Name: "Trojan 测试", Protocol: "trojan", Server: "example.com", Port: 443, Settings: map[string]string{"password": "secret", "tls": "true", "server_name": "tls.example.com", "transport_type": "ws", "transport_path": "/proxy"}},
		{ID: "ss", Name: "SS 测试", Protocol: "shadowsocks", Server: "2001:db8::1", Port: 8388, Settings: map[string]string{"method": "aes-256-gcm", "password": "secret"}},
		{ID: "socks", Name: "SOCKS 测试", Protocol: "socks", Server: "example.com", Port: 1080, Settings: map[string]string{"username": "user", "password": "pass"}},
		{ID: "http", Name: "HTTP 测试", Protocol: "http", Server: "example.com", Port: 8443, Settings: map[string]string{"username": "user", "password": "pass", "tls": "true", "server_name": "tls.example.com", "path": "/proxy"}},
		{ID: "hy2", Name: "Hysteria2 测试", Protocol: "hysteria2", Server: "example.com", Port: 443, Settings: map[string]string{"password": "secret", "sni": "tls.example.com", "insecure": "true", "obfs_type": "salamander", "obfs_password": "obfs"}},
	}
	for _, original := range nodes {
		t.Run(original.Protocol, func(t *testing.T) {
			link, err := nodeShareLink(original)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := parseSubscriptionLine(link)
			if err != nil {
				t.Fatalf("parse exported link %q: %v", link, err)
			}
			if parsed.Protocol != original.Protocol || parsed.Server != original.Server || parsed.Port != original.Port || parsed.Name != original.Name {
				t.Fatalf("round trip mismatch: got %+v want %+v", parsed, original)
			}
			for key, value := range original.Settings {
				if parsed.Settings[key] != value {
					t.Fatalf("setting %s=%q want %q; link=%s", key, parsed.Settings[key], value, link)
				}
			}
		})
	}
}
