package singbox

import (
	"encoding/json"
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
	data, err := BuildConfig(testConfig(config.RoutingSmart), "D:\\test")
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
	if len(rules) != 4 {
		t.Fatalf("rules count=%d want=4", len(rules))
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
	if routeRules[0].(map[string]any)["action"] != "hijack-dns" {
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
}
