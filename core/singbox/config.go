package singbox

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/caichengle666/sbtun/config"
)

// RuntimeConfig 是 sing-box runtime JSON 的最小结构。
// 使用 map 保留 sing-box 新版本字段扩展能力，同时避免把用户配置模型绑定到核心 JSON。
type RuntimeConfig struct {
	Schema     string           `json:"$schema,omitempty"`
	Log        map[string]any   `json:"log,omitempty"`
	DNS        map[string]any   `json:"dns,omitempty"`
	Inbounds   []map[string]any `json:"inbounds,omitempty"`
	Outbounds  []map[string]any `json:"outbounds,omitempty"`
	Route      map[string]any   `json:"route,omitempty"`
	Experimental map[string]any `json:"experimental,omitempty"`
}

// BuildConfig 根据用户配置生成 sing-box runtime 配置。
// 第一阶段只生成 TUN + 节点 + DNS + 路由闭环，不让 GUI 直接操作 JSON。
func BuildConfig(cfg config.Config) ([]byte, error) {
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

	outbounds := []map[string]any{proxy, {"type": "direct", "tag": "direct"}, {"type": "block", "tag": "block"}}
	route := buildRoute(cfg.RoutingMode)
	result := RuntimeConfig{
		Schema: "https://sing-box.sagernet.org/schema.json",
		Log: map[string]any{"level": "info", "timestamp": true},
		DNS: buildDNS(cfg.DNSMode),
		Inbounds: []map[string]any{{
			"type": "tun", "tag": "tun-in", "address": []string{"172.18.0.1/30"},
			"auto_route": true, "strict_route": true, "stack": "system",
			"dns_mode": "hijack", "dns_address": []string{"172.18.0.2"},
		}},
		Outbounds: outbounds,
		Route: route,
		Experimental: map[string]any{"cache_file": map[string]any{"enabled": true}},
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("生成 sing-box 配置失败: %w", err)
	}
	return data, nil
}

func findNode(nodes []config.Node, id string) (config.Node, bool) {
	for _, n := range nodes {
		if n.ID == id { return n, true }
	}
	return config.Node{}, false
}

func buildDNS(mode config.DNSMode) map[string]any {
	server := map[string]any{"tag": "dns-direct", "address": "local", "detour": "direct"}
	if mode == config.DNSCustom {
		server["address"] = "https://1.1.1.1/dns-query"
	}
	return map[string]any{"servers": []map[string]any{server}, "final": "dns-direct"}
}

func buildRoute(mode config.RoutingMode) map[string]any {
	rules := []map[string]any{{"ip_is_private": true, "outbound": "direct"}}
	switch mode {
	case config.RoutingGlobal:
		rules = append(rules, map[string]any{"inbound": []string{"tun-in"}, "outbound": "proxy"})
	case config.RoutingDirect:
		rules = append(rules, map[string]any{"inbound": []string{"tun-in"}, "outbound": "direct"})
	case config.RoutingSmart:
		// 国内规则集下一阶段由规则管理器提供；当前保留明确的默认代理兜底。
		rules = append(rules, map[string]any{"inbound": []string{"tun-in"}, "outbound": "proxy"})
	case config.RoutingCustom:
		rules = append(rules, map[string]any{"inbound": []string{"tun-in"}, "outbound": "proxy"})
	}
	return map[string]any{"auto_detect_interface": true, "rules": rules, "final": "proxy"}
}

func buildOutbound(n config.Node) (map[string]any, error) {
	protocol := strings.ToLower(strings.TrimSpace(n.Protocol))
	out := map[string]any{"type": protocol, "tag": "proxy", "server": n.Server, "server_port": n.Port}
	for k, v := range n.Settings { addSetting(out, k, v) }
	switch protocol {
	case "vless", "vmess", "trojan", "shadowsocks", "socks", "http":
		return out, nil
	default:
		return nil, fmt.Errorf("暂不支持的节点协议: %s", n.Protocol)
	}
}

func addSetting(out map[string]any, key, value string) {
	switch key {
	case "uuid", "password", "method", "flow", "security", "network", "alter_id", "username":
		out[key] = value
	case "tls":
		if strings.EqualFold(value, "true") { out[key] = map[string]any{"enabled": true} }
	case "server_name":
		tls, _ := out["tls"].(map[string]any); if tls == nil { tls = map[string]any{}; out["tls"] = tls }; tls["server_name"] = value
	case "transport":
		out["transport"] = map[string]any{"type": value}
	}
}
