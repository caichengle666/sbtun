package app

import (
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestParseCustomRulesText(t *testing.T) {
	rules, err := parseCustomRules([]byte("proxy,domain_suffix,example.com\ndirect,ip_cidr,10.0.0.0/8\n# comment\n"), config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].Action != "proxy" || rules[1].MatchType != "ip_cidr" {
		t.Fatalf("rules=%+v", rules)
	}
}

func TestParseCustomRulesJSON(t *testing.T) {
	rules, err := parseCustomRules([]byte(`[{"match_type":"port","value":"443","action":"proxy"}]`), config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Value != "443" {
		t.Fatalf("rules=%+v", rules)
	}
}

func TestParseClashRulesResolvesTargets(t *testing.T) {
	cfg := config.Config{
		Nodes:         []config.Node{{ID: "n1", Name: "JP-ChatGPT"}},
		Subscriptions: []config.Subscription{{ID: "s1", Name: "机场 B"}},
		NodeGroups:    []config.NodeGroup{{ID: "g1", Name: "流媒体"}},
	}
	text := "rules:\n" +
		"  - DOMAIN-SUFFIX,openai.com,🇺🇸 美国节点\n" +
		"  - DOMAIN-KEYWORD,claude,JP-ChatGPT\n" +
		"  - 'DOMAIN,www.netflix.com,流媒体'\n" +
		"  - DOMAIN-SUFFIX,b.example,机场 B\n" +
		"  - IP-CIDR,10.0.0.0/8,DIRECT,no-resolve\n" +
		"  - DOMAIN-SUFFIX,ads.example,REJECT\n" +
		"  - DOMAIN-SUFFIX,x.example,hy2\n" +
		"  - DOMAIN-SUFFIX,y.example,🚀 节点选择\n" +
		"  - GEOIP,CN,DIRECT\n" +
		"  - MATCH,🚀 节点选择\n"
	rules, err := parseCustomRules([]byte(text), cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"region:US", "node:n1", "group:g1", "sub:s1", "direct", "block", "proto:hysteria2", "proxy"}
	if len(rules) != len(want) {
		t.Fatalf("rules=%+v", rules)
	}
	for i, w := range want {
		if rules[i].Action != w {
			t.Fatalf("rule %d action=%q want %q (%+v)", i, rules[i].Action, w, rules[i])
		}
	}
	if rules[4].MatchType != "ip_cidr" || rules[4].Value != "10.0.0.0/8" {
		t.Fatalf("ip rule=%+v", rules[4])
	}
}
