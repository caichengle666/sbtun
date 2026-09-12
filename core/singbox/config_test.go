package singbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

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
	cfg := testConfig(config.RoutingCustom)
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
	if len(rules) != 6 {
		t.Fatalf("rules count=%d want=6", len(rules))
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
	if len(servers) != 2 {
		t.Fatalf("dns servers count=%d want=2", len(servers))
	}
	server := servers[1].(map[string]any)
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
}
