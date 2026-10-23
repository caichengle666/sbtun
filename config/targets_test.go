package config

import "testing"

func TestGuessRegion(t *testing.T) {
	cases := map[string]string{
		"🇺🇸 美国 01": "US", "香港 IPLC 02": "HK", "JP-Tokyo": "JP", "US-Netflix 解锁": "US",
		"Plus 套餐": "", "印度尼西亚 01": "ID", "新加坡→香港中转": "SG", "剩余流量 10G": "", "sg1": "SG",
	}
	for name, want := range cases {
		if got := GuessRegion(name); got != want {
			t.Errorf("GuessRegion(%q)=%q want %q", name, got, want)
		}
	}
}

func TestValidateAction(t *testing.T) {
	for _, ok := range []string{"proxy", "Direct", "block", "node:a-b", "group:g1", "sub:s1", "region:US", "proto:hysteria2"} {
		if err := ValidateAction(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "proxy:x", "node:", "region:us", "region:USA", "foo"} {
		if ValidateAction(bad) == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestGroupMatches(t *testing.T) {
	g := NodeGroup{Regions: []string{"US", "JP"}, Protocols: []string{"hysteria2"}, Keyword: "iplc 高速", Exclude: "过期"}
	cases := []struct {
		node Node
		want bool
	}{
		{Node{Name: "美国 IPLC", Country: "US", Protocol: "hysteria2"}, true},
		{Node{Name: "日本 高速", Country: "JP", Protocol: "hysteria2"}, true},
		{Node{Name: "美国 IPLC", Country: "US", Protocol: "vless"}, false},
		{Node{Name: "香港 IPLC", Country: "HK", Protocol: "hysteria2"}, false},
		{Node{Name: "美国 普通", Country: "US", Protocol: "hysteria2"}, false},
		{Node{Name: "美国 IPLC 过期", Country: "US", Protocol: "hysteria2"}, false},
	}
	for _, c := range cases {
		if got := g.Matches(c.node); got != c.want {
			t.Errorf("%+v: got %v", c.node, got)
		}
	}
}
