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
		Nodes: []config.Node{{ID: "n1", Name: "测试节点", Protocol: "socks", Server: "127.0.0.1", Port: 1080}},
	}
}

func TestRoutingModes(t *testing.T) {
	cases := []struct{ mode config.RoutingMode; final string }{
		{config.RoutingSmart, "proxy"},
		{config.RoutingGlobal, "proxy"},
		{config.RoutingDirect, "direct"},
		{config.RoutingCustom, "proxy"},
	}
	for _, tc := range cases {
		data, err := BuildConfig(testConfig(tc.mode)); if err != nil { t.Fatal(err) }
		var runtime RuntimeConfig
		if err := json.Unmarshal(data, &runtime); err != nil { t.Fatal(err) }
		if got := runtime.Route["final"]; got != tc.final { t.Fatalf("mode %s final=%v want=%s", tc.mode, got, tc.final) }
	}
}

func TestSmartRoutingUsesChinaRuleSets(t *testing.T) {
	data, err := BuildConfig(testConfig(config.RoutingSmart)); if err != nil { t.Fatal(err) }
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil { t.Fatal(err) }
	sets, ok := runtime.Route["rule_set"].([]any); if !ok { t.Fatalf("rule_set type=%T", runtime.Route["rule_set"]) }
	if len(sets) != 2 { t.Fatalf("rule_set count=%d want=2", len(sets)) }
}

func TestCustomRoutingRule(t *testing.T) {
	cfg := testConfig(config.RoutingCustom)
	cfg.CustomRules = []config.Rule{{MatchType: "domain_suffix", Value: "example.com", Action: "direct"}, {MatchType: "port", Value: "443", Action: "proxy"}}
	data, err := BuildConfig(cfg); if err != nil { t.Fatal(err) }
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil { t.Fatal(err) }
	rules := runtime.Route["rules"].([]any)
	if len(rules) != 3 { t.Fatalf("rules count=%d want=3", len(rules)) }
}

func TestCustomDNSUsesModernServerShape(t *testing.T) {
	cfg := testConfig(config.RoutingGlobal); cfg.DNSMode = config.DNSCustom
	data, err := BuildConfig(cfg); if err != nil { t.Fatal(err) }
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil { t.Fatal(err) }
	servers := runtime.DNS["servers"].([]any)
	server := servers[0].(map[string]any)
	if server["type"] != "https" { t.Fatalf("dns type=%v", server["type"]) }
}
