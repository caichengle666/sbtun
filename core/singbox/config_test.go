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
	if routeRules[0].(map[string]any)["action"] != "sniff" || routeRules[1].(map[string]any)["action"] != "hijack-dns" || routeRules[2].(map[string]any)["action"] != "resolve" {
		t.Fatalf("smart route metadata rules missing: %+v", routeRules[:3])
	}
}

func TestUserRuleSetsPrecedeDefaultRuleSets(t *testing.T) {
	exeDir := t.TempDir()
	rulesDir := filepath.Join(exeDir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{"geosite-geolocation-cn.srs", "geoip-cn.srs", "geosite-geolocation-!cn.srs", "ads-a.srs", "ads-b.srs"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(rulesDir, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sets := `[{"id":"geosite-cn","path":"geosite-geolocation-cn.srs","enabled":true,"source":"default","action":"direct"},{"id":"geoip-cn","path":"geoip-cn.srs","enabled":true,"source":"default","action":"direct"},{"id":"geosite-non-cn","path":"geosite-geolocation-!cn.srs","enabled":true,"source":"default","action":"proxy"},{"id":"ads-a","path":"ads-a.srs","enabled":true,"source":"custom","action":"block"},{"id":"ads-disabled","path":"missing.srs","enabled":false,"source":"custom","action":"block"},{"id":"ads-b","path":"ads-b.srs","enabled":true,"source":"custom","action":"block"}]`
	if err := os.WriteFile(filepath.Join(rulesDir, "sets.json"), []byte(sets), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := BuildConfig(testConfig(config.RoutingSmart), exeDir)
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	rules := runtime.Route["rules"].([]any)
	want := []string{"ads-a", "ads-b", "geosite-cn", "geoip-cn", "geosite-non-cn"}
	got := make([]string, 0, len(want))
	for _, raw := range rules {
		rule := raw.(map[string]any)
		refs, ok := rule["rule_set"].([]any)
		if ok && len(refs) > 0 {
			got = append(got, refs[0].(string))
		}
	}
	if len(got) != len(want) {
		t.Fatalf("route rule-set order=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("route rule-set order=%v want=%v", got, want)
		}
	}
	declared := runtime.Route["rule_set"].([]any)
	for i, id := range want {
		if declared[i].(map[string]any)["tag"] != id {
			t.Fatalf("declared rule-set order[%d]=%v want=%s", i, declared[i].(map[string]any)["tag"], id)
		}
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

func TestCaptureRulesHavePriorityAndAvoidLoop(t *testing.T) {
	cfg := testConfig(config.RoutingSmart)
	cfg.CaptureEnabled = true
	cfg.CaptureDomains = []string{"example.com", "google"}
	cfg.CustomRules = []config.Rule{{MatchType: "domain_suffix", Value: "example.com", Action: "direct"}}
	data, err := BuildConfig(cfg, "D:\\test")
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	rules := runtime.Route["rules"].([]any)
	if rules[2].(map[string]any)["domain_suffix"] == nil || rules[2].(map[string]any)["outbound"] != "block" {
		t.Fatalf("capture domain UDP rule missing: %+v", rules[2])
	}
	if rules[3].(map[string]any)["outbound"] != "capture" || rules[3].(map[string]any)["inbound"] == nil {
		t.Fatalf("capture domain TCP rule missing or not limited to tun-in: %+v", rules[3])
	}
	if rules[4].(map[string]any)["domain_keyword"] == nil || rules[4].(map[string]any)["outbound"] != "block" || rules[5].(map[string]any)["outbound"] != "capture" {
		t.Fatalf("capture keyword rules missing: %+v", rules[4:6])
	}
	if rules[6].(map[string]any)["action"] != "resolve" {
		t.Fatalf("domain resolution must run after capture rules: %+v", rules[6])
	}
	if rules[7].(map[string]any)["outbound"] != "direct" {
		t.Fatalf("custom rule must run after capture on the second inbound: %+v", rules[7])
	}
	foundCapture := false
	for _, raw := range runtime.Outbounds {
		if raw["tag"] == "capture" && raw["server_port"] == float64(captureProxyPort) {
			foundCapture = true
		}
	}
	if !foundCapture {
		t.Fatal("capture outbound missing")
	}
	foundUpstream := false
	for _, inbound := range runtime.Inbounds {
		if inbound["tag"] == "capture-upstream" && inbound["type"] == "mixed" && inbound["listen_port"] == float64(captureUpstreamPort) {
			foundUpstream = true
		}
	}
	if !foundUpstream {
		t.Fatal("capture mixed upstream inbound missing")
	}
}

func TestCaptureAllRoutesHTTPOnlyFromTun(t *testing.T) {
	cfg := testConfig(config.RoutingSmart)
	cfg.CaptureEnabled = true
	cfg.CaptureDomains = []string{"*"}
	data, err := BuildConfig(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	rules := runtime.Route["rules"].([]any)
	udp := rules[2].(map[string]any)
	tcp := rules[3].(map[string]any)
	if udp["outbound"] != "block" || udp["network"] != "udp" || tcp["outbound"] != "capture" || tcp["network"] != "tcp" {
		t.Fatalf("capture all rules=%+v", rules[2:4])
	}
	if tcp["domain_suffix"] != nil || tcp["domain_keyword"] != nil || len(tcp["inbound"].([]any)) != 1 || tcp["inbound"].([]any)[0] != "tun-in" {
		t.Fatalf("capture all must only intercept tun-in without a domain filter: %+v", tcp)
	}
	if len(tcp["port"].([]any)) != 2 || rules[4].(map[string]any)["action"] != "resolve" {
		t.Fatalf("capture all must be limited to HTTP ports before resolution: %+v", rules[2:5])
	}
}

func TestCaptureConfigPassesSingBoxCheck(t *testing.T) {
	binary := filepath.Join("..", "..", "dist", "downloads", "sing-box", "sing-box-1.14.0-windows-amd64", "sing-box.exe")
	if _, err := os.Stat(binary); err != nil {
		t.Skip("local sing-box binary not available")
	}
	cfg := testConfig(config.RoutingGlobal)
	cfg.CaptureEnabled = true
	cfg.CaptureDomains = []string{"example.com"}
	data, err := BuildConfig(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "capture.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "check", "-c", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("capture config failed sing-box check: %v\n%s\n%s", err, output, data)
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
	if routeRules[1].(map[string]any)["action"] != "hijack-dns" {
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
	addresses := runtime.Inbounds[0]["address"].([]any)
	if len(addresses) != 1 || addresses[0] != "172.18.0.1/30" {
		t.Fatalf("tun addresses=%v want IPv4 only by default", runtime.Inbounds[0]["address"])
	}
	rules := runtime.Route["rules"].([]any)
	if rules[0].(map[string]any)["action"] != "sniff" || rules[2].(map[string]any)["action"] != "resolve" {
		t.Fatalf("route sniff/resolve actions missing: %+v", rules[:3])
	}
}

func TestTUNIPv6FallsBackWhenUnavailable(t *testing.T) {
	cfg := testConfig(config.RoutingGlobal)
	cfg.IPv6Enabled = true
	data, err := BuildConfig(cfg, "D:\\test")
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	addresses := runtime.Inbounds[0]["address"].([]any)
	want := 1
	if HasUsableIPv6() {
		want = 2
	}
	if len(addresses) != want {
		t.Fatalf("tun addresses=%v want %d addresses", addresses, want)
	}
}

func TestTUNWebRTCProtectionEnablesStrictRoute(t *testing.T) {
	cfg := testConfig(config.RoutingGlobal)
	cfg.WebRTCProtectionEnabled = true
	data, err := BuildConfig(cfg, "D:\\test")
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	if runtime.Inbounds[0]["strict_route"] != true {
		t.Fatalf("strict_route=%v want=true", runtime.Inbounds[0]["strict_route"])
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
