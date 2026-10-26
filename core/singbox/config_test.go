package singbox

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestGeneratedProtocolConfigsPassSingBoxCheck(t *testing.T) {
	binary := singBoxTestBinary(t)
	cases := []config.Node{
		{ID: "vless", Protocol: "vless", Server: "example.com", Port: 443, Settings: map[string]string{"uuid": "00000000-0000-0000-0000-000000000001", "tls": "true", "server_name": "example.com", "alpn": "h2,http/1.1", "transport_type": "ws", "transport_path": "/ws", "transport_host": "example.com"}},
		{ID: "vmess", Protocol: "vmess", Server: "example.com", Port: 443, Settings: map[string]string{"uuid": "00000000-0000-0000-0000-000000000001", "security": "auto", "tls": "true", "transport_type": "ws", "transport_path": "/ws", "transport_host": "example.com"}},
		{ID: "trojan", Protocol: "trojan", Server: "example.com", Port: 443, Settings: map[string]string{"password": "secret", "tls": "true", "transport_type": "grpc", "transport_service_name": "proxy"}},
		{ID: "shadowsocks", Protocol: "shadowsocks", Server: "example.com", Port: 8388, Settings: map[string]string{"method": "aes-256-gcm", "password": "secret", "network": "tcp"}},
		{ID: "socks", Protocol: "socks", Server: "example.com", Port: 1080, Settings: map[string]string{"version": "5", "username": "user", "password": "secret"}},
		{ID: "http", Protocol: "http", Server: "example.com", Port: 8080, Settings: map[string]string{"username": "user", "password": "secret", "tls": "true", "server_name": "example.com", "path": "/proxy"}},
		{ID: "hysteria2", Protocol: "hysteria2", Server: "example.com", Port: 443, Settings: map[string]string{"password": "secret", "sni": "example.com", "alpn": "h3", "server_ports": "2000-3000", "hop_interval": "30s", "obfs_type": "salamander", "obfs_password": "secret"}},
		{ID: "hysteria", Protocol: "hysteria", Server: "example.com", Port: 443, Settings: map[string]string{"password": "secret", "sni": "example.com", "server_ports": "2000-3000", "obfs_password": "secret", "insecure": "true"}},
		{ID: "hysteria-bw", Protocol: "hysteria", Server: "example.com", Port: 443, Settings: map[string]string{"up_mbps": "30", "down_mbps": "200", "alpn": "h3"}},
		{ID: "tuic", Protocol: "tuic", Server: "example.com", Port: 443, Settings: map[string]string{"uuid": "00000000-0000-0000-0000-000000000001", "password": "secret", "congestion_control": "bbr", "udp_relay_mode": "native", "server_name": "sni.example.com"}},
		{ID: "anytls", Protocol: "anytls", Server: "example.com", Port: 443, Settings: map[string]string{"password": "secret", "insecure": "true", "utls_fingerprint": "chrome"}},
		{ID: "wireguard", Protocol: "wireguard", Server: "example.com", Port: 51820, Settings: map[string]string{"private_key": "GBI17Q5In1YIWiw67IH5PS18N+FuH/xIren5DUH11WY=", "public_key": "fvrhwBf3ofI/oWcIZPIySYyB1SqHv/dOM8kTVGxYlS0=", "local_address": "172.16.0.2,fd01::2", "reserved": "1,2,3", "mtu": "1280"}},
	}
	for _, node := range cases {
		t.Run(node.ID, func(t *testing.T) {
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

// singBoxTestBinary 优先使用 SING_BOX_BINARY，否则使用仓库下载目录中的 Windows 内核。
func singBoxTestBinary(t *testing.T) string {
	t.Helper()
	binary := os.Getenv("SING_BOX_BINARY")
	if binary == "" {
		binary = filepath.Join("..", "..", "dist", "downloads", "sing-box", "sing-box-1.14.0-windows-amd64", "sing-box.exe")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Skip("local sing-box binary not available")
	}
	return binary
}

func TestHysteriaOutboundFields(t *testing.T) {
	out, err := buildOutbound(config.Node{Protocol: "hysteria", Server: "h.example", Port: 443, Settings: map[string]string{"password": "pw", "obfs_password": "op", "down_mbps": "300"}})
	if err != nil {
		t.Fatal(err)
	}
	if out["type"] != "hysteria" || out["auth_str"] != "pw" || out["obfs"] != "op" || out["up_mbps"] != defaultHysteriaUpMbps || out["down_mbps"] != 300 || out["password"] != nil {
		t.Fatalf("out=%v", out)
	}
}

func TestRealityDefaultsToChromeUTLS(t *testing.T) {
	out, err := buildOutbound(config.Node{Protocol: "vless", Server: "r.example", Port: 443, Settings: map[string]string{"uuid": "u", "tls": "true", "reality_public_key": "jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI+T4E7RoLJS0="}})
	if err != nil {
		t.Fatal(err)
	}
	utls, _ := out["tls"].(map[string]any)["utls"].(map[string]any)
	if utls["fingerprint"] != "chrome" {
		t.Fatalf("tls=%v", out["tls"])
	}
}

func TestWireGuardNodeBuildsEndpointUsableBySelector(t *testing.T) {
	cfg := testConfig(config.RoutingGlobal)
	cfg.Nodes = append(cfg.Nodes, config.Node{ID: "wg", Name: "wg", Protocol: "wireguard", Server: "wg.example.com", Port: 51820, Settings: map[string]string{
		"private_key": "GBI17Q5In1YIWiw67IH5PS18N+FuH/xIren5DUH11WY=", "public_key": "fvrhwBf3ofI/oWcIZPIySYyB1SqHv/dOM8kTVGxYlS0=", "local_address": "172.16.0.2",
	}})
	cfg.CurrentNodeID = "wg"
	data, err := BuildConfig(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var parsed RuntimeConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Endpoints) != 1 || parsed.Endpoints[0]["tag"] != "node-wg" {
		t.Fatalf("wireguard endpoint missing: %s", data)
	}
	for _, out := range parsed.Outbounds {
		if out["type"] == "wireguard" {
			t.Fatalf("wireguard must not be an outbound: %s", data)
		}
		if out["tag"] == "proxy" && out["default"] != "node-wg" {
			t.Fatalf("selector does not default to wireguard endpoint: %v", out)
		}
	}
	if binary := os.Getenv("SING_BOX_BINARY"); binary != "" {
		path := filepath.Join(t.TempDir(), "wg.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
			t.Fatalf("sing-box check failed: %v\n%s\n%s", err, output, data)
		}
	}
}

func TestParseWireGuardReserved(t *testing.T) {
	for _, value := range []string{"1,2,3", "AQID"} {
		got, err := parseWireGuardReserved(value)
		if err != nil || len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
			t.Fatalf("%q => %v, %v", value, got, err)
		}
	}
	if _, err := parseWireGuardReserved("1,2"); err == nil {
		t.Fatal("two bytes must be rejected")
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
	binary := singBoxTestBinary(t)
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

// 智能分流 + 自动 DNS：只有 geosite-cn 规则集启用且文件存在时，DNS 规则才能引用它；
// 关掉或缺文件时引用它会让 sing-box 报 rule-set not found 拒绝整份配置。
func TestSmartDNSUsesLocalRulesAndRemoteFallback(t *testing.T) {
	writeRules := func(t *testing.T, sets string) string {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "rules"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "rules", "geosite-geolocation-cn.srs"), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
		if sets != "" {
			if err := os.WriteFile(filepath.Join(dir, "rules", "sets.json"), []byte(sets), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	cases := []struct {
		name     string
		exeDir   string
		wantRule bool
		check    bool // 不引用真实规则文件时可直接交给 sing-box check
	}{
		{"规则文件缺失", t.TempDir(), false, true},
		{"规则集已启用", writeRules(t, ""), true, false},
		{"规则集被关闭", writeRules(t, `[{"id":"geosite-cn","path":"geosite-geolocation-cn.srs","enabled":false,"source":"default","action":"direct"}]`), false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := BuildConfig(testConfig(config.RoutingSmart), tc.exeDir)
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
			hasRule, hasPrivate := false, false
			for _, r := range runtime.DNS["rules"].([]any) {
				rule := r.(map[string]any)
				hasRule = hasRule || rule["rule_set"] != nil
				hasPrivate = hasPrivate || rule["ip_is_private"] == true
			}
			if hasRule != tc.wantRule || !hasPrivate {
				t.Fatalf("dns rules=%v", runtime.DNS["rules"])
			}
			routeRules := runtime.Route["rules"].([]any)
			if routeRules[1].(map[string]any)["action"] != "hijack-dns" {
				t.Fatalf("dns interception rule missing")
			}
			if binary := os.Getenv("SING_BOX_BINARY"); binary != "" && tc.check {
				path := filepath.Join(t.TempDir(), "c.json")
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
				if output, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
					t.Fatalf("sing-box check failed: %v\n%s", err, output)
				}
			}
		})
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
	if len(addresses) != 1 || addresses[0] != tunIPv4Address(localIPv4s()) {
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

func TestBuildConfigWithAuthProtectsLocalPorts(t *testing.T) {
	cfg := testConfig(config.RoutingSmart)
	cfg.CaptureEnabled = true
	cfg.CaptureDomains = []string{"example.com"}
	auth := LocalAuth{ClashSecret: "clash", ProxyUsername: "sbtun", ProxyPassword: "pw"}
	data, err := BuildConfigWithAuth(cfg, t.TempDir(), auth, nil)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Inbounds     []map[string]any `json:"inbounds"`
		Outbounds    []map[string]any `json:"outbounds"`
		Experimental struct {
			ClashAPI map[string]any `json:"clash_api"`
		} `json:"experimental"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Experimental.ClashAPI["secret"] != "clash" {
		t.Fatalf("clash_api=%v", parsed.Experimental.ClashAPI)
	}
	var sawInbound, sawOutbound bool
	for _, inbound := range parsed.Inbounds {
		if inbound["tag"] == "capture-upstream" {
			users, _ := inbound["users"].([]any)
			if len(users) != 1 {
				t.Fatalf("capture-upstream users=%v", inbound["users"])
			}
			user := users[0].(map[string]any)
			sawInbound = user["username"] == "sbtun" && user["password"] == "pw"
		}
	}
	for _, outbound := range parsed.Outbounds {
		if outbound["tag"] == "capture" {
			sawOutbound = outbound["username"] == "sbtun" && outbound["password"] == "pw"
		}
	}
	if !sawInbound || !sawOutbound {
		t.Fatalf("capture ports not authenticated: inbound=%v outbound=%v", sawInbound, sawOutbound)
	}
}

func TestNodeServersBypassTUNAndResolveLocally(t *testing.T) {
	cfg := testConfig(config.RoutingGlobal)
	cfg.Nodes = append(cfg.Nodes,
		config.Node{ID: "ip", Name: "ip", Protocol: "socks", Server: "203.0.113.7", Port: 1080},
		config.Node{ID: "v6", Name: "v6", Protocol: "socks", Server: "[2001:db8::1]", Port: 1080},
		config.Node{ID: "dn", Name: "dn", Protocol: "socks", Server: "Node.Example.com", Port: 1080},
		config.Node{ID: "dn2", Name: "dn2", Protocol: "socks", Server: "node.example.com", Port: 1081},
	)
	resolved := []netip.Addr{netip.MustParseAddr("198.51.100.9"), netip.MustParseAddr("203.0.113.7")}
	data, err := BuildConfigWithAuth(cfg, t.TempDir(), LocalAuth{}, resolved)
	if err != nil {
		t.Fatal(err)
	}
	var parsed RuntimeConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	exclude, _ := parsed.Inbounds[0]["route_exclude_address"].([]any)
	// 127.0.0.1 is skipped, duplicates merge, IPv6 is left out while IPv6 is off.
	if fmt.Sprint(exclude) != "[198.51.100.9/32 203.0.113.7/32]" {
		t.Fatalf("route_exclude_address=%v", exclude)
	}
	rules, _ := parsed.DNS["rules"].([]any)
	first, _ := rules[0].(map[string]any)
	if fmt.Sprint(first["domain"]) != "[node.example.com]" || first["server"] != "dns-local" {
		t.Fatalf("first dns rule=%v", first)
	}

	cfg.IPv6Enabled = true
	data, err = BuildConfigWithAuth(cfg, t.TempDir(), LocalAuth{}, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"2001:db8::1/128"`) {
		t.Fatalf("ipv6 node not excluded: %s", data)
	}
	if binary := os.Getenv("SING_BOX_BINARY"); binary != "" {
		path := filepath.Join(t.TempDir(), "bypass.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
			t.Fatalf("sing-box check failed: %v\n%s\n%s", err, output, data)
		}
	}
}

func BuildConfig(cfg config.Config, exeDir string) ([]byte, error) {
	return BuildConfigWithAuth(cfg, exeDir, LocalAuth{}, nil)
}

func TestMobileInboundPassesSingBoxCheck(t *testing.T) {
	binary := singBoxTestBinary(t)
	cfg := testConfig(config.RoutingGlobal)
	data, err := BuildConfigWithAuth(cfg, t.TempDir(), LocalAuth{ClashSecret: "s", ProxyUsername: "u", ProxyPassword: "p"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var parsed RuntimeConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	parsed.Inbounds[0] = mobileInbound(LocalAuth{ProxyUsername: "u", ProxyPassword: "p"})
	parsed.Route["auto_detect_interface"] = false
	data, _ = json.Marshal(parsed)
	path := filepath.Join(t.TempDir(), "mobile.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
		t.Fatalf("sing-box check failed: %v\n%s\n%s", err, output, data)
	}
}

func TestTunIPv4AddressSkipsUsed(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("只有 Windows / Linux 会顺延地址")
	}
	used := []net.IP{net.ParseIP("172.18.0.1"), net.ParseIP("172.19.0.2"), net.ParseIP("192.168.1.5")}
	if got := tunIPv4Address(used); got != "172.20.0.1/30" {
		t.Fatalf("got %s", got)
	}
	if got := tunIPv4Address(nil); got != "172.18.0.1/30" {
		t.Fatalf("got %s", got)
	}
}

// 屏蔽规则排最前；用户规则集可指向节点组，.json 规则集按 source 加载；直连模式只保留屏蔽类规则。
func TestMyRulesOrderAndDirectMode(t *testing.T) {
	exeDir := t.TempDir()
	rulesDir := filepath.Join(exeDir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ads.srs", "stream.json"} {
		if err := os.WriteFile(filepath.Join(rulesDir, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setsJSON := `[{"id":"ads","path":"ads.srs","enabled":true,"source":"custom","action":"block"},{"id":"stream","path":"stream.json","enabled":true,"source":"custom","action":"group:g1"}]`
	if err := os.WriteFile(filepath.Join(rulesDir, "sets.json"), []byte(setsJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	build := func(mode config.RoutingMode) (rules []map[string]any, ruleSets []any) {
		cfg := testConfig(mode)
		cfg.Nodes = append(cfg.Nodes, config.Node{ID: "n2", Name: "美国", Protocol: "socks", Server: "127.0.0.2", Port: 1080})
		cfg.NodeGroups = []config.NodeGroup{{ID: "g1", Name: "流媒体", Keyword: "美国"}}
		cfg.CustomRules = []config.Rule{
			{MatchType: "domain_suffix", Value: "ad.example", Action: "proxy"},
			{MatchType: "domain_suffix", Value: "bad.example", Action: "block"},
		}
		cfg.DNSFilterRules = []config.Rule{{MatchType: "domain_suffix", Value: "ad.example", Action: "block"}}
		data, err := BuildConfig(cfg, exeDir)
		if err != nil {
			t.Fatal(err)
		}
		var runtime RuntimeConfig
		if err := json.Unmarshal(data, &runtime); err != nil {
			t.Fatal(err)
		}
		for _, r := range runtime.Route["rules"].([]any) {
			if m := r.(map[string]any); m["action"] == nil {
				rules = append(rules, m)
			}
		}
		set, _ := runtime.Route["rule_set"].([]any)
		return rules, set
	}
	rules, sets := build(config.RoutingGlobal)
	got := []string{}
	for _, r := range rules {
		got = append(got, fmt.Sprintf("%v %v %v", r["domain_suffix"], r["rule_set"], r["outbound"]))
	}
	want := []string{"[ad.example] <nil> block", "[ad.example] <nil> proxy", "[bad.example] <nil> block", "<nil> [ads] block", "<nil> [stream] node-n2", "<nil> <nil> direct"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("global rules=%v", got)
	}
	if len(sets) != 2 || sets[1].(map[string]any)["format"] != "source" {
		t.Fatalf("rule_set=%v", sets)
	}
	rules, sets = build(config.RoutingDirect)
	got = got[:0]
	for _, r := range rules {
		got = append(got, fmt.Sprintf("%v %v %v", r["domain_suffix"], r["rule_set"], r["outbound"]))
	}
	want = []string{"[ad.example] <nil> block", "[bad.example] <nil> block", "<nil> [ads] block", "<nil> <nil> direct"}
	if strings.Join(got, "|") != strings.Join(want, "|") || len(sets) != 1 {
		t.Fatalf("direct rules=%v sets=%v", got, sets)
	}
}
