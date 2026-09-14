package singbox

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestGeneratedProtocolConfigsPassSingBoxCheck(t *testing.T) {
	binary := filepath.Join("..", "..", "dist", "downloads", "sing-box", "sing-box-1.14.0-windows-amd64", "sing-box.exe")
	if _, err := os.Stat(binary); err != nil {
		t.Skip("local sing-box 1.14.0 binary not available")
	}
	cases := []config.Node{
		{ID: "vless", Protocol: "vless", Server: "example.com", Port: 443, Settings: map[string]string{"uuid": "00000000-0000-0000-0000-000000000001", "tls": "true", "server_name": "example.com", "alpn": "h2,http/1.1", "transport_type": "ws", "transport_path": "/ws", "transport_host": "example.com"}},
		{ID: "vmess", Protocol: "vmess", Server: "example.com", Port: 443, Settings: map[string]string{"uuid": "00000000-0000-0000-0000-000000000001", "security": "auto", "tls": "true", "transport_type": "ws", "transport_path": "/ws", "transport_host": "example.com"}},
		{ID: "trojan", Protocol: "trojan", Server: "example.com", Port: 443, Settings: map[string]string{"password": "secret", "tls": "true", "transport_type": "grpc", "transport_service_name": "proxy"}},
		{ID: "shadowsocks", Protocol: "shadowsocks", Server: "example.com", Port: 8388, Settings: map[string]string{"method": "aes-256-gcm", "password": "secret", "network": "tcp"}},
		{ID: "socks", Protocol: "socks", Server: "example.com", Port: 1080, Settings: map[string]string{"version": "5", "username": "user", "password": "secret"}},
		{ID: "http", Protocol: "http", Server: "example.com", Port: 8080, Settings: map[string]string{"username": "user", "password": "secret", "tls": "true", "server_name": "example.com", "path": "/proxy"}},
		{ID: "hysteria2", Protocol: "hysteria2", Server: "example.com", Port: 443, Settings: map[string]string{"password": "secret", "sni": "example.com", "alpn": "h3", "server_ports": "2000-3000", "hop_interval": "30s", "obfs_type": "salamander", "obfs_password": "secret"}},
	}
	for _, node := range cases {
		t.Run(node.Protocol, func(t *testing.T) {
			data, err := BuildHealthConfig(node, 18080)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "check", "-c", path)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("sing-box check failed: %v\n%s\nconfig:\n%s", err, output, data)
			}
		})
	}
}

func TestHealthDNSUsesPlatformResolver(t *testing.T) {
	data, err := BuildHealthConfig(config.Node{ID: "n1", Protocol: "socks", Server: "127.0.0.1", Port: 1080}, 18080)
	if err != nil {
		t.Fatal(err)
	}
	var generated map[string]any
	if err := json.Unmarshal(data, &generated); err != nil {
		t.Fatal(err)
	}
	dns := generated["dns"].(map[string]any)
	servers := dns["servers"].([]any)
	server := servers[0].(map[string]any)
	if server["type"] != "local" || dns["final"] != "dns-local" {
		t.Fatalf("unexpected platform health DNS: %+v", dns)
	}
}

func testConfig(mode config.RoutingMode) config.Config {
	return config.Config{
		Version: 1, RoutingMode: mode, DNSMode: config.DNSAuto,
		CurrentNodeID: "n1",
		Nodes:         []config.Node{{ID: "n1", Name: "测试节点", Protocol: "socks", Server: "127.0.0.1", Port: 1080}},
	}
}

func TestRoutingModes(t *testing.T) {
	cases := []struct {
		mode  config.RoutingMode
		final string
	}{
		{config.RoutingSmart, "proxy"},
		{config.RoutingGlobal, "proxy"},
		{config.RoutingDirect, "direct"},
		{config.RoutingCustom, "proxy"},
	}
	for _, tc := range cases {
		data, err := BuildConfig(testConfig(tc.mode), "D:\\test")
		if err != nil {
			t.Fatal(err)
		}
		var runtime RuntimeConfig
		if err := json.Unmarshal(data, &runtime); err != nil {
			t.Fatal(err)
		}
		if got := runtime.Route["final"]; got != tc.final {
			t.Fatalf("mode %s final=%v want=%s", tc.mode, got, tc.final)
		}
	}
}

func TestSmartRoutingUsesChinaRuleSets(t *testing.T) {
	exeDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(exeDir, "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"geosite-geolocation-cn.srs", "geoip-cn.srs"} {
		if err := os.WriteFile(filepath.Join(exeDir, "rules", name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := BuildConfig(testConfig(config.RoutingSmart), exeDir)
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	sets, ok := runtime.Route["rule_set"].([]any)
	if !ok {
		t.Fatalf("rule_set type=%T", runtime.Route["rule_set"])
	}
	if len(sets) != 2 {
		t.Fatalf("rule_set count=%d want=2", len(sets))
	}
	routeRules := runtime.Route["rules"].([]any)
	if len(routeRules) != 6 {
		t.Fatalf("smart route rules=%d want=6", len(routeRules))
	}
	if routeRules[0].(map[string]any)["action"] != "sniff" || routeRules[1].(map[string]any)["action"] != "resolve" {
		t.Fatalf("smart route metadata rules missing: %+v", routeRules[:2])
	}
}

func TestCustomRoutingRule(t *testing.T) {
	cfg := testConfig(config.RoutingSmart)
	cfg.CustomRules = []config.Rule{{MatchType: "domain_suffix", Value: "example.com", Action: "direct"}, {MatchType: "port", Value: "443", Action: "proxy"}}
	data, err := BuildConfig(cfg, "D:\\test")
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	rules := runtime.Route["rules"].([]any)
	if len(rules) < 6 {
		t.Fatalf("rules count=%d want at least 6", len(rules))
	}
	if rules[3].(map[string]any)["domain_suffix"] == nil || rules[3].(map[string]any)["outbound"] != "direct" {
		t.Fatalf("custom domain rule is not first priority: %+v", rules)
	}
	if rules[4].(map[string]any)["port"] == nil || rules[4].(map[string]any)["outbound"] != "proxy" {
		t.Fatalf("custom port rule is not first priority: %+v", rules)
	}
}

func TestCustomDNSUsesModernServerShape(t *testing.T) {
	cfg := testConfig(config.RoutingGlobal)
	cfg.DNSMode = config.DNSCustom
	data, err := BuildConfig(cfg, "D:\\test")
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	servers := runtime.DNS["servers"].([]any)
	if len(servers) != 3 {
		t.Fatalf("dns servers count=%d want=3", len(servers))
	}
	server := servers[2].(map[string]any)
	if server["type"] != "https" {
		t.Fatalf("dns type=%v", server["type"])
	}
	if server["tag"] != "dns-remote" {
		t.Fatalf("dns tag=%v", server["tag"])
	}
	if server["server"] != "cloudflare-dns.com" || server["server_port"] != float64(443) {
		t.Fatalf("dns endpoint=%v:%v", server["server"], server["server_port"])
	}
	if server["domain_resolver"] != "dns-local" || server["detour"] != "proxy" {
		t.Fatalf("dns bootstrap=%v detour=%v", server["domain_resolver"], server["detour"])
	}
}

func TestSmartDNSUsesLocalRulesAndRemoteFallback(t *testing.T) {
	data, err := BuildConfig(testConfig(config.RoutingSmart), "D:\\test")
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	if runtime.DNS["final"] != "dns-remote" {
		t.Fatalf("dns final=%v", runtime.DNS["final"])
	}
	rules := runtime.DNS["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("dns rules=%d want=2", len(rules))
	}
	routeRules := runtime.Route["rules"].([]any)
	if routeRules[2].(map[string]any)["action"] != "hijack-dns" {
		t.Fatalf("dns interception rule missing")
	}
}

func TestTUNKeepsWindowsCompatibleRouteMode(t *testing.T) {
	data, err := BuildConfig(testConfig(config.RoutingGlobal), "D:\\test")
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	if runtime.Inbounds[0]["strict_route"] != false {
		t.Fatalf("strict_route=%v want=false", runtime.Inbounds[0]["strict_route"])
	}
	rules := runtime.Route["rules"].([]any)
	if rules[0].(map[string]any)["action"] != "sniff" || rules[1].(map[string]any)["action"] != "resolve" {
		t.Fatalf("route sniff/resolve actions missing: %+v", rules[:2])
	}
}

func TestBuildConfigSkipsInvalidBackupNode(t *testing.T) {
	cfg := testConfig(config.RoutingGlobal)
	cfg.Nodes = append(cfg.Nodes, config.Node{ID: "bad", Name: "坏节点", Protocol: "unsupported", Server: "bad.example", Port: 443})
	data, err := BuildConfig(cfg, "D:\\test")
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	selector := runtime.Outbounds[len(runtime.Outbounds)-3]
	if selector["type"] != "selector" {
		t.Fatalf("selector outbound missing: %+v", selector)
	}
	if len(selector["outbounds"].([]any)) != 1 {
		t.Fatalf("invalid backup node was not skipped: %+v", selector["outbounds"])
	}
	if selector["interrupt_exist_connections"] != true {
		t.Fatalf("selector must interrupt existing connections: %+v", selector)
	}
}
