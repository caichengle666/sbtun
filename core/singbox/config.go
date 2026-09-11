package singbox

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/caichengle666/sbtun/config"
)

type RuntimeConfig struct {
	Schema       string           `json:"$schema,omitempty"`
	Log          map[string]any   `json:"log,omitempty"`
	DNS          map[string]any   `json:"dns,omitempty"`
	Inbounds     []map[string]any `json:"inbounds,omitempty"`
	Outbounds    []map[string]any `json:"outbounds,omitempty"`
	Route        map[string]any   `json:"route,omitempty"`
	Experimental map[string]any   `json:"experimental,omitempty"`
}

// BuildConfig 将用户配置转换为 sing-box 运行配置。
// 四种模式在这里形成明确、可验证的路由闭环：智能分流、全局代理、全局直连、自定义规则。
func BuildConfig(cfg config.Config, exeDir string) ([]byte, error) {
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}
	if cfg.CurrentNodeID == "" {
		return nil, fmt.Errorf("尚未选择节点")
	}
	node, ok := findNode(cfg.Nodes, cfg.CurrentNodeID)
	if !ok {
		return nil, fmt.Errorf("当前节点不存在: %s", cfg.CurrentNodeID)
	}
	proxy, err := buildOutbound(node)
	if err != nil {
		return nil, err
	}

	result := RuntimeConfig{
		Schema: "https://sing-box.sagernet.org/schema.json",
		Log:    map[string]any{"level": "info", "timestamp": true},
		DNS:    buildDNS(cfg.DNSMode, cfg.RoutingMode),
		Inbounds: []map[string]any{{
			"type": "tun", "tag": "tun-in",
			"address":    []string{"172.18.0.1/30"},
			"auto_route": true, "strict_route": false, "stack": "system",
		}},
		Outbounds: []map[string]any{proxy, {"type": "direct", "tag": "direct", "domain_resolver": "dns-local"}, {"type": "block", "tag": "block"}},
		Route:     routeForMode(cfg.RoutingMode, cfg.CustomRules, exeDir),
		Experimental: map[string]any{
			"cache_file": map[string]any{"enabled": true},
			"clash_api":  map[string]any{"external_controller": "127.0.0.1:9090"},
		},
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("生成 sing-box 配置失败: %w", err)
	}
	return data, nil
}

func findNode(nodes []config.Node, id string) (config.Node, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return config.Node{}, false
}

func buildDNS(mode config.DNSMode, routeMode config.RoutingMode) map[string]any {
	servers := []map[string]any{
		{"type": "local", "tag": "dns-local", "detour": "direct"},
		{
			"type": "https", "tag": "dns-remote", "server": "cloudflare-dns.com", "server_port": 443,
			"path": "/dns-query", "domain_resolver": "dns-local", "detour": "proxy",
			"tls": map[string]any{"enabled": true, "server_name": "cloudflare-dns.com"},
		},
	}
	final := "dns-remote"
	if mode == config.DNSSystem || routeMode == config.RoutingDirect {
		final = "dns-local"
	}
	rules := []map[string]any{}
	if mode == config.DNSAuto && routeMode == config.RoutingSmart {
		rules = append(rules,
			map[string]any{"rule_set": []string{"geosite-cn"}, "server": "dns-local"},
			map[string]any{"ip_is_private": true, "server": "dns-local"},
		)
	}
	if mode == config.DNSCustom {
		final = "dns-remote"
	}
	return map[string]any{"servers": servers, "rules": rules, "final": final, "strategy": "prefer_ipv4"}
}

func routeForMode(mode config.RoutingMode, custom []config.Rule, exeDir string) map[string]any {
	private := map[string]any{"ip_is_private": true, "outbound": "direct"}
	dns := map[string]any{"protocol": "dns", "action": "hijack-dns"}
	base := []map[string]any{dns, private}
	final := "proxy"

	switch mode {
	case config.RoutingGlobal:
		// 除本机/局域网地址外，全部进入代理。
		final = "proxy"
	case config.RoutingDirect:
		// 所有流量直连，不允许 proxy 成为默认兜底。
		final = "direct"
	case config.RoutingSmart:
		// 中国域名/IP 直连；非中国流量代理。规则集由发布包中的 rules/ 提供。
		base = append(base,
			map[string]any{"rule_set": []string{"geosite-cn"}, "outbound": "direct"},
			map[string]any{"rule_set": []string{"geoip-cn"}, "outbound": "direct"},
		)
		final = "proxy"
	case config.RoutingCustom:
		for _, r := range custom {
			if rr, ok := customRule(r); ok {
				base = append(base, rr)
			}
		}
		final = "proxy"
	}

	ruleSets := []map[string]any{}
	if mode == config.RoutingSmart {
		ruleSets = []map[string]any{
			{"type": "local", "tag": "geosite-cn", "format": "binary", "path": filepath.Join(exeDir, "rules", "geosite-geolocation-cn.srs")},
			{"type": "local", "tag": "geoip-cn", "format": "binary", "path": filepath.Join(exeDir, "rules", "geoip-cn.srs")},
		}
	}
	return map[string]any{
		"auto_detect_interface":   true,
		"default_domain_resolver": "dns-local",
		"rule_set":                ruleSets,
		"rules":                   base,
		"final":                   final,
	}
}

func customRule(r config.Rule) (map[string]any, bool) {
	value := strings.TrimSpace(r.Value)
	if value == "" {
		return nil, false
	}
	action := strings.ToLower(strings.TrimSpace(r.Action))
	if action != "proxy" && action != "direct" && action != "block" {
		return nil, false
	}
	key := strings.ToLower(strings.TrimSpace(r.MatchType))
	rule := map[string]any{"outbound": action}
	switch key {
	case "domain_suffix":
		rule["domain_suffix"] = []string{value}
	case "domain_keyword":
		rule["domain_keyword"] = []string{value}
	case "domain":
		rule["domain"] = []string{value}
	case "ip_cidr":
		rule["ip_cidr"] = []string{value}
	case "port":
		rule["port"] = []int{parsePort(value)}
	default:
		return nil, false
	}
	if key == "port" && parsePort(value) == 0 {
		return nil, false
	}
	return rule, true
}

func parsePort(s string) int {
	var p int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &p); err != nil || p < 1 || p > 65535 {
		return 0
	}
	return p
}

func buildOutbound(n config.Node) (map[string]any, error) {
	protocol := strings.ToLower(strings.TrimSpace(n.Protocol))
	if protocol == "hysteria2" {
		return buildHysteria2(map[string]any{"tag": "proxy"}, n), nil
	}
	out := map[string]any{"type": protocol, "tag": "proxy", "server": n.Server, "server_port": n.Port, "domain_resolver": "dns-local"}
	for k, v := range n.Settings {
		addSetting(out, k, v)
	}
	switch protocol {
	case "vless", "vmess", "trojan", "shadowsocks", "socks", "http", "hysteria2":
		return out, nil
	default:
		return nil, fmt.Errorf("暂不支持的节点协议: %s", n.Protocol)
	}
}

func buildHysteria2(out map[string]any, n config.Node) map[string]any {
	out["type"] = "hysteria2"
	out["server"] = n.Server
	out["server_port"] = n.Port
	if n.Settings != nil {
		if v := n.Settings["password"]; v != "" {
			out["password"] = v
		}
		if v := n.Settings["up_mbps"]; v != "" {
			out["up_mbps"] = parseBandwidth(v)
		}
		if v := n.Settings["down_mbps"]; v != "" {
			out["down_mbps"] = parseBandwidth(v)
		}
	}
	tls := map[string]any{"enabled": true}
	if v := n.Settings["sni"]; v != "" {
		tls["server_name"] = v
	} else {
		tls["server_name"] = n.Server
	}
	if v := n.Settings["insecure"]; strings.EqualFold(v, "true") || v == "1" {
		tls["insecure"] = true
	}
	out["tls"] = tls
	return out
}

func parseBandwidth(s string) int {
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n); err == nil {
		return n
	}
	return 0
}

func addSetting(out map[string]any, key, value string) {
	switch key {
	case "uuid", "password", "method", "flow", "security", "network", "alter_id", "username":
		out[key] = value
	case "tls":
		if strings.EqualFold(value, "true") {
			out["tls"] = map[string]any{"enabled": true}
		}
	case "server_name":
		tls, _ := out["tls"].(map[string]any)
		if tls == nil {
			tls = map[string]any{}
			out["tls"] = tls
		}
		tls["server_name"] = value
	case "transport":
		out["transport"] = map[string]any{"type": value}
	}
}
