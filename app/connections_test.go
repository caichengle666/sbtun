package app

import (
	"encoding/json"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestConnectionLabels(t *testing.T) {
	cfg := config.Config{
		Nodes:          []config.Node{{ID: "a", Name: "东京01", Country: "JP"}, {ID: "b", Name: "洛杉矶"}},
		DNSFilterRules: []config.Rule{{MatchType: "domain_keyword", Value: "ads", Action: "block"}},
	}
	l := connectionLabeler{cfg: cfg, ruleSets: map[string]string{"geosite-cn": "国内网站"}}
	// 以下 rule/chains 取自 sing-box 1.14 clash API 的真实输出。
	raw := `[
	{"chains":["direct"],"rule":"rule_set=geosite-cn => route(direct)","metadata":{"host":"ip111.cn","destinationPort":"443"}},
	{"chains":["node-a","auto-region:JP"],"rule":"domain_suffix=openai.com => route(auto-region:JP)","metadata":{"host":"api.openai.com"}},
	{"chains":["node-b","proxy"],"rule":"final","metadata":{"destinationIP":"8.8.8.8"}},
	{"chains":["block"],"rule":"domain_keyword=ads => route(block)","metadata":{"host":"ads.example.com"}},
	{"chains":["direct"],"rule":"ip_is_private=true => route(direct)","metadata":{"destinationIP":"192.168.1.1"}},
	{"chains":["capture"],"rule":"inbound=tun-in domain_suffix=[a.com b.com] network=tcp => route(capture)","metadata":{"host":"a.com"}}
	]`
	var conns []clashConnection
	if err := json.Unmarshal([]byte(raw), &conns); err != nil {
		t.Fatal(err)
	}
	want := [][4]string{
		{"ip111.cn", "规则集 国内网站", "直连", ""},
		{"api.openai.com", "分流规则 openai.com", "东京01", "地区 JP"},
		{"8.8.8.8", "未命中规则（默认）", "洛杉矶", "当前节点"},
		{"ads.example.com", "过滤规则 ads", "拦截", ""},
		{"192.168.1.1", "局域网地址", "直连", ""},
		{"a.com", "流量分析 a.com b.com", "流量分析", ""},
	}
	for i, c := range conns {
		got := l.describe(c)
		if got.Host != want[i][0] || got.Rule != want[i][1] || got.Target != want[i][2] || got.Via != want[i][3] {
			t.Errorf("%d: got %q %q %q %q", i, got.Host, got.Rule, got.Target, got.Via)
		}
	}
}

func TestRouteTestHost(t *testing.T) {
	for in, want := range map[string]string{"https://WWW.Google.com/search?q=1": "www.google.com", "openai.com:443": "openai.com", " ip111.cn/ ": "ip111.cn", "[2001:db8::1]:443": "2001:db8::1"} {
		if got, err := routeTestHost(in); err != nil || got != want {
			t.Errorf("%q => %q %v", in, got, err)
		}
	}
	if _, err := routeTestHost("a b"); err == nil {
		t.Error("expected error")
	}
}
