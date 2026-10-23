package singbox

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestRuleTargetsBuildGroups(t *testing.T) {
	cfg := testConfig(config.RoutingCustom)
	cfg.Subscriptions = []config.Subscription{{ID: "s1", Name: "机场", URL: "https://sub.example/s"}}
	cfg.Nodes = []config.Node{
		{ID: "n1", Name: "美国 1", Protocol: "socks", Server: "127.0.0.1", Port: 1080, Country: "US", Subscription: "s1"},
		{ID: "n2", Name: "美国 2", Protocol: "socks", Server: "127.0.0.2", Port: 1080, Country: "US"},
		{ID: "n3", Name: "日本 IPLC", Protocol: "http", Server: "127.0.0.3", Port: 8080, Country: "JP", Subscription: "s1"},
		{ID: "bad", Name: "美国 坏", Protocol: "unsupported", Server: "bad.example", Port: 1, Country: "US"},
	}
	cfg.NodeGroups = []config.NodeGroup{{ID: "g1", Name: "机场 socks", Subscriptions: []string{"s1"}, Protocols: []string{"socks", "http"}, Exclude: "日本"}}
	cfg.CustomRules = []config.Rule{
		{MatchType: "domain_suffix", Value: "a.example", Action: "region:US"},
		{MatchType: "domain_suffix", Value: "b.example", Action: "group:g1"},
		{MatchType: "domain_suffix", Value: "c.example", Action: "node:gone"},
		{MatchType: "domain_suffix", Value: "d.example", Action: "sub:s1"},
		{MatchType: "domain_suffix", Value: "e.example", Action: "proto:vless"},
		{MatchType: "domain_suffix", Value: "f.example", Action: "Direct"},
		{MatchType: "domain_suffix", Value: "g.example", Action: "region:US"},
	}
	data, err := BuildConfig(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var parsed RuntimeConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	groups := map[string][]any{}
	for _, out := range parsed.Outbounds {
		if out["type"] == "urltest" {
			groups[out["tag"].(string)] = out["outbounds"].([]any)
		}
	}
	// 地区 US 有两个可用节点 → urltest（坏节点不在里面）；组 g1 只剩 n1 → 直接走节点；同一去向只建一个组。
	if len(groups) != 2 || len(groups["auto-region:US"]) != 2 || len(groups["auto-sub:s1"]) != 2 {
		t.Fatalf("groups=%v", groups)
	}
	want := map[string]string{"a.example": "auto-region:US", "b.example": "node-n1", "c.example": "proxy", "d.example": "auto-sub:s1", "e.example": "proxy", "f.example": "direct", "g.example": "auto-region:US"}
	rules := parsed.Route["rules"].([]any)
	for _, raw := range rules {
		rule := raw.(map[string]any)
		suffix, ok := rule["domain_suffix"].([]any)
		if !ok {
			continue
		}
		if got := rule["outbound"]; got != want[suffix[0].(string)] {
			t.Fatalf("%s -> %v, want %s", suffix[0], got, want[suffix[0].(string)])
		}
		delete(want, suffix[0].(string))
	}
	if len(want) != 0 {
		t.Fatalf("missing rules: %v", want)
	}
	if binary := os.Getenv("SING_BOX_BINARY"); binary != "" {
		path := filepath.Join(t.TempDir(), "targets.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
			t.Fatalf("sing-box check failed: %v\n%s", err, output)
		}
	}
}
