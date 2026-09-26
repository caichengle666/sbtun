package singbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

const (
	captureProxyPort    = 9081
	captureUpstreamPort = 9082
)

// BuildConfig 将用户配置转换为 sing-box 运行配置。
// 四种模式在这里形成明确、可验证的路由闭环：智能分流、全局代理、全局直连、自定义规则。
func BuildConfig(cfg config.Config, exeDir string) ([]byte, error) {
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}
	if cfg.CurrentNodeID == "" {
		return nil, fmt.Errorf("尚未选择节点")
	}
	if _, ok := findNode(cfg.Nodes, cfg.CurrentNodeID); !ok {
		return nil, fmt.Errorf("当前节点不存在: %s", cfg.CurrentNodeID)
	}
	proxy, err := buildSelector(cfg.Nodes, cfg.CurrentNodeID)
	if err != nil {
		return nil, err
	}

	outbounds := append(proxy, map[string]any{"type": "direct", "tag": "direct", "domain_resolver": "dns-local"}, map[string]any{"type": "block", "tag": "block"})
	if cfg.CaptureEnabled && len(normalizeCaptureDomains(cfg.CaptureDomains)) > 0 {
		outbounds = append(outbounds, map[string]any{
			"type": "http", "tag": "capture", "server": "127.0.0.1", "server_port": captureProxyPort,
		})
	}
	inbounds := []map[string]any{{
		"type": "tun", "tag": "tun-in",
		"address":    tunAddresses(cfg.IPv6Enabled),
		"auto_route": true, "strict_route": cfg.WebRTCProtectionEnabled, "stack": "system",
	}}
	if cfg.CaptureEnabled && len(normalizeCaptureDomains(cfg.CaptureDomains)) > 0 {
		inbounds = append(inbounds, map[string]any{
			"type": "mixed", "tag": "capture-upstream",
			"listen": "127.0.0.1", "listen_port": captureUpstreamPort,
		})
	}
	result := RuntimeConfig{
		Schema:    "https://sing-box.sagernet.org/schema.json",
		Log:       map[string]any{"level": "info", "timestamp": true},
		DNS:       buildDNS(cfg.DNSMode, cfg.RoutingMode),
		Inbounds:  inbounds,
		Outbounds: outbounds,
		Route:     routeForMode(cfg.RoutingMode, cfg.CustomRules, cfg.DNSFilterRules, cfg.CaptureEnabled, cfg.CaptureDomains, exeDir),
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

func buildSelector(nodes []config.Node, currentID string) ([]map[string]any, error) {
	tags := make([]string, 0, len(nodes))
	outbounds := make([]map[string]any, 0, len(nodes)+1)
	for _, node := range nodes {
		outbound, err := buildOutbound(node)
		if err != nil {
			if node.ID == currentID {
				return nil, fmt.Errorf("当前节点配置无效: %w", err)
			}
			// 备用节点损坏时跳过，不能阻止当前健康节点启动。
			continue
		}
		tag := nodeTag(node.ID)
		outbound["tag"] = tag
		outbounds = append(outbounds, outbound)
		tags = append(tags, tag)
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("没有可用节点")
	}
	outbounds = append(outbounds, map[string]any{
		"type":                        "selector",
		"tag":                         "proxy",
		"outbounds":                   tags,
		"default":                     nodeTag(currentID),
		"interrupt_exist_connections": true,
	})
	return outbounds, nil
}

func nodeTag(id string) string { return "node-" + id }

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
		{"type": "udp", "tag": "dns-local", "server": "223.5.5.5", "server_port": 53, "detour": "direct"},
		{"type": "udp", "tag": "dns-local-standby", "server": "119.29.29.29", "server_port": 53, "detour": "direct"},
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

func routeForMode(mode config.RoutingMode, custom, filterRules []config.Rule, captureEnabled bool, captureDomains []string, exeDir string) map[string]any {
	private := map[string]any{"ip_is_private": true, "outbound": "direct"}
	dns := map[string]any{"protocol": "dns", "action": "hijack-dns"}
	base := []map[string]any{
		{"inbound": []string{"tun-in"}, "action": "sniff", "timeout": "1s"},
		dns,
	}
	domains, keywords := splitCapturePatterns(captureDomains)
	if captureEnabled {
		if containsCaptureAll(captureDomains) {
			base = append(base,
				map[string]any{"inbound": []string{"tun-in"}, "network": "udp", "port": []int{443}, "outbound": "block"},
				map[string]any{"inbound": []string{"tun-in"}, "network": "tcp", "port": []int{80, 443}, "outbound": "capture"},
			)
		} else {
			if len(domains) > 0 {
				base = append(base,
					map[string]any{"inbound": []string{"tun-in"}, "domain_suffix": domains, "network": "udp", "port": []int{443}, "outbound": "block"},
					map[string]any{"inbound": []string{"tun-in"}, "domain_suffix": domains, "network": "tcp", "port": []int{80, 443}, "outbound": "capture"},
				)
			}
			if len(keywords) > 0 {
				base = append(base,
					map[string]any{"inbound": []string{"tun-in"}, "domain_keyword": keywords, "network": "udp", "port": []int{443}, "outbound": "block"},
					map[string]any{"inbound": []string{"tun-in"}, "domain_keyword": keywords, "network": "tcp", "port": []int{80, 443}, "outbound": "capture"},
				)
			}
		}
	}
	base = append(base, map[string]any{"inbound": []string{"tun-in"}, "action": "resolve", "strategy": "prefer_ipv4"})
	for _, r := range custom {
		if rr, ok := customRule(r); ok {
			base = append(base, rr)
		}
	}
	for _, r := range filterRules {
		if rr, ok := customRule(r); ok {
			base = append(base, rr)
		}
	}
	ruleSets := activeRuleSets(mode, exeDir)
	for _, set := range ruleSets {
		base = append(base, map[string]any{"rule_set": []string{set.ID}, "outbound": set.Action})
	}
	base = append(base, private)
	final := "proxy"

	switch mode {
	case config.RoutingGlobal:
		// 除本机/局域网地址外，全部进入代理。
		final = "proxy"
	case config.RoutingDirect:
		// 所有流量直连，不允许 proxy 成为默认兜底。
		final = "direct"
	case config.RoutingSmart:
		// 中国域名/IP 直连；非中国域名和其余流量代理。
		final = "proxy"
	case config.RoutingCustom:
		final = "proxy"
	}

	ruleSetConfigs := make([]map[string]any, 0, len(ruleSets))
	for _, set := range ruleSets {
		ruleSetConfigs = append(ruleSetConfigs, map[string]any{"type": "local", "tag": set.ID, "format": "binary", "path": filepath.Join(exeDir, "rules", filepath.Base(set.Path))})
	}
	return map[string]any{
		"auto_detect_interface":   true,
		"default_domain_resolver": "dns-local",
		"rule_set":                ruleSetConfigs,
		"rules":                   base,
		"final":                   final,
	}
}

type configuredRuleSet struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
	Source  string `json:"source"`
	Action  string `json:"action"`
}

func activeRuleSets(mode config.RoutingMode, exeDir string) []configuredRuleSet {
	sets := configuredRuleSets(exeDir)
	userSets := make([]configuredRuleSet, 0, len(sets))
	defaultSets := make([]configuredRuleSet, 0, len(sets))
	for _, set := range sets {
		if !set.Enabled || !ruleSetExists(exeDir, filepath.Base(set.Path)) {
			continue
		}
		if set.Source == "default" {
			if mode == config.RoutingSmart {
				defaultSets = append(defaultSets, set)
			}
			continue
		}
		userSets = append(userSets, set)
	}
	return append(userSets, defaultSets...)
}

func configuredRuleSets(exeDir string) []configuredRuleSet {
	data, err := os.ReadFile(filepath.Join(exeDir, "rules", "sets.json"))
	if err == nil {
		var sets []configuredRuleSet
		if json.Unmarshal(data, &sets) == nil {
			for i := range sets {
				if sets[i].Action == "" {
					sets[i].Action = "block"
				}
				if sets[i].Source == "" {
					sets[i].Source = "custom"
				}
			}
			return sets
		}
	}
	return []configuredRuleSet{
		{ID: "geosite-cn", Path: "geosite-geolocation-cn.srs", Enabled: true, Source: "default", Action: "direct"},
		{ID: "geoip-cn", Path: "geoip-cn.srs", Enabled: true, Source: "default", Action: "direct"},
		{ID: "geosite-non-cn", Path: "geosite-geolocation-!cn.srs", Enabled: true, Source: "default", Action: "proxy"},
	}
}

func normalizeCaptureDomains(domains []string) []string {
	seen := make(map[string]struct{}, len(domains))
	result := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
		if domain == "" {
			continue
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		result = append(result, domain)
	}
	return result
}

func splitCapturePatterns(patterns []string) (domains, keywords []string) {
	for _, pattern := range normalizeCaptureDomains(patterns) {
		if pattern == "*" {
			continue
		}
		if strings.Contains(pattern, ".") {
			domains = append(domains, pattern)
		} else {
			keywords = append(keywords, pattern)
		}
	}
	return domains, keywords
}

func containsCaptureAll(patterns []string) bool {
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "*" {
			return true
		}
	}
	return false
}

func ruleSetExists(exeDir, name string) bool {
	info, err := os.Stat(filepath.Join(exeDir, "rules", name))
	return err == nil && !info.IsDir() && info.Size() > 0
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
		if strings.HasPrefix(k, "transport_") {
			continue
		}
		addSetting(out, k, v)
	}
	addTransport(out, n.Settings)
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
	out["domain_resolver"] = "dns-local"
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
		if v := n.Settings["server_ports"]; v != "" {
			out["server_ports"] = normalizePortRanges(v)
		}
		if v := n.Settings["hop_interval"]; v != "" {
			out["hop_interval"] = v
		}
		if v := n.Settings["network"]; v == "tcp" || v == "udp" {
			out["network"] = v
		}
		if strings.EqualFold(n.Settings["disable_path_mtu_discovery"], "true") || n.Settings["disable_path_mtu_discovery"] == "1" {
			out["disable_path_mtu_discovery"] = true
		}
		if v := n.Settings["obfs_type"]; v != "" {
			out["obfs"] = map[string]any{"type": v, "password": n.Settings["obfs_password"]}
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
	if v := n.Settings["alpn"]; v != "" {
		tls["alpn"] = splitList(v)
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
	case "uuid", "password", "method", "flow", "security", "network", "username", "version", "packet_encoding", "plugin", "plugin_opts", "path":
		out[key] = value
	case "alter_id":
		if v, err := strconv.Atoi(value); err == nil {
			out[key] = v
		} else {
			out[key] = value
		}
	case "server_ports", "hop_interval":
		out[key] = value
	case "disable_path_mtu_discovery", "udp_over_tcp", "multiplex":
		if strings.EqualFold(value, "true") || value == "1" {
			if key == "multiplex" {
				out[key] = map[string]any{"enabled": true}
			} else {
				out[key] = true
			}
		}
	case "tls":
		if strings.EqualFold(value, "true") || value == "1" {
			tls, _ := out["tls"].(map[string]any)
			if tls == nil {
				tls = map[string]any{}
			}
			tls["enabled"] = true
			out["tls"] = tls
		}
	case "server_name", "insecure", "alpn", "reality_public_key", "reality_short_id", "utls_fingerprint":
		tls, _ := out["tls"].(map[string]any)
		if tls == nil {
			tls = map[string]any{"enabled": true}
			out["tls"] = tls
		}
		switch key {
		case "server_name":
			tls["server_name"] = value
		case "insecure":
			if strings.EqualFold(value, "true") || value == "1" {
				tls["insecure"] = true
			}
		case "alpn":
			tls["alpn"] = splitList(value)
		case "reality_public_key", "reality_short_id":
			reality, _ := tls["reality"].(map[string]any)
			if reality == nil {
				reality = map[string]any{"enabled": true}
				tls["reality"] = reality
			}
			reality[strings.TrimPrefix(key, "reality_")] = value
		case "utls_fingerprint":
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": value}
		}
	case "obfs_type":
		out["obfs"] = map[string]any{"type": value}
	case "obfs_password":
		obfs, _ := out["obfs"].(map[string]any)
		if obfs == nil {
			obfs = map[string]any{}
			out["obfs"] = obfs
		}
		obfs["password"] = value
	}
}

func addTransport(out map[string]any, settings map[string]string) {
	typeName := settings["transport_type"]
	if typeName == "" {
		typeName = settings["transport"]
	}
	if typeName == "" {
		return
	}
	transport := map[string]any{"type": typeName}
	if path := settings["transport_path"]; path != "" {
		transport["path"] = path
	}
	if host := settings["transport_host"]; host != "" {
		switch typeName {
		case "ws", "httpupgrade":
			transport["headers"] = map[string]any{"Host": host}
		default:
			transport["host"] = host
		}
	}
	if service := settings["transport_service_name"]; service != "" && typeName == "grpc" {
		transport["service_name"] = service
	}
	out["transport"] = transport
}

func splitList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })
	return parts
}

func normalizePortRanges(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })
	for i, part := range parts {
		parts[i] = strings.Replace(part, "-", ":", 1)
	}
	return parts
}
