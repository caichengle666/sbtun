package app

import (
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestParseCustomRulesText(t *testing.T) {
	rules, _, err := parseRuleImport([]byte("proxy,domain_suffix,example.com\ndirect,ip_cidr,10.0.0.0/8\n# comment\n"), config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].Action != "proxy" || rules[1].MatchType != "ip_cidr" {
		t.Fatalf("rules=%+v", rules)
	}
}

func TestParseCustomRulesJSON(t *testing.T) {
	rules, _, err := parseRuleImport([]byte(`[{"match_type":"port","value":"443","action":"proxy"}]`), config.Config{})
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
	rules, _, err := parseRuleImport([]byte(text), cfg)
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

// 引用外部列表的写法各自成为一个规则集，策略换成去向；单条规则照旧。
func TestParseRuleImportRuleSetRefs(t *testing.T) {
	cfg := config.Config{NodeGroups: []config.NodeGroup{{ID: "g1", Name: "流媒体"}}}
	text := "rule-providers:\n" +
		"  reject:\n    type: http\n    behavior: domain\n    url: \"https://example.com/reject.txt\"\n    path: ./ruleset/reject.yaml\n" +
		"  local:\n    type: file\n    behavior: domain\n    path: ./local.yaml\n" +
		"rules:\n" +
		"  - RULE-SET,reject,REJECT\n" +
		"  - RULE-SET,local,DIRECT\n" +
		"  - RULE-SET,https://example.com/Netflix.list,流媒体\n" +
		"  - DOMAIN-SET,https://example.com/proxy.txt?x=1,🚀 节点选择\n" +
		"  - DOMAIN-SUFFIX,a.example,DIRECT\n" +
		"  - RULE-SET,reject,REJECT\n"
	rules, refs, err := parseRuleImport([]byte(text), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Value != "a.example" {
		t.Fatalf("rules=%+v", rules)
	}
	want := []ruleSetRef{
		{"reject", "https://example.com/reject.txt", "block"},
		{"Netflix", "https://example.com/Netflix.list", "group:g1"},
		{"proxy", "https://example.com/proxy.txt?x=1", "proxy"},
	}
	if len(refs) != len(want) {
		t.Fatalf("refs=%+v", refs)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("ref %d=%+v want %+v", i, refs[i], want[i])
		}
	}
}

// subconverter 模板（ACL4SSR 等）的 ruleset= 写法。
func TestParseRuleImportSubconverter(t *testing.T) {
	text := "[custom]\n" +
		"ruleset=🎯 全球直连,clash-domain:https://example.com/LocalAreaNetwork.list\n" +
		"ruleset=🛑 广告拦截,https://example.com/BanAD.list\n" +
		"ruleset=🎯 全球直连,[]GEOIP,CN\n" +
		"ruleset=🐟 漏网之鱼,[]FINAL\n"
	_, refs, err := parseRuleImport([]byte(text), config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	want := []ruleSetRef{
		{"LocalAreaNetwork", "https://example.com/LocalAreaNetwork.list", "direct"},
		{"BanAD", "https://example.com/BanAD.list", "block"},
	}
	if len(refs) != len(want) || refs[0] != want[0] || refs[1] != want[1] {
		t.Fatalf("refs=%+v", refs)
	}
}
