package singbox

import (
	"github.com/caichengle666/sbtun/core/tun"

	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core/rules"
)

type RuntimeConfig struct {
	Schema       string           `json:"$schema,omitempty"`
	Log          map[string]any   `json:"log,omitempty"`
	DNS          map[string]any   `json:"dns,omitempty"`
	Inbounds     []map[string]any `json:"inbounds,omitempty"`
	Outbounds    []map[string]any `json:"outbounds,omitempty"`
	Endpoints    []map[string]any `json:"endpoints,omitempty"`
	Route        map[string]any   `json:"route,omitempty"`
	Experimental map[string]any   `json:"experimental,omitempty"`
}

const (
	captureProxyPort    = 9081
	captureUpstreamPort = 9082
)

// BuildConfigWithAuth 将用户配置转换为 sing-box 运行配置，并用本地凭据保护
// clash API 和抓包代理端口。resolved 是节点域名（小写）当前解析出的地址。
// 三种模式只决定内置的国内直连规则集开不开、匹配不到时走代理还是直连；
// 自己的规则（分流规则、规则集）在智能和全局下优先生效，直连模式下只保留屏蔽类。
func BuildConfigWithAuth(cfg config.Config, exeDir string, auth LocalAuth, resolved map[string][]netip.Addr) ([]byte, error) {
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

	ruleSets := activeRuleSets(cfg.RoutingMode, exeDir)
	targets, groups := ruleTargets(cfg, ruleSets, proxy)
	outbounds, endpoints := splitEndpoints(proxy)
	outbounds = append(outbounds, groups...)
	outbounds = append(outbounds, map[string]any{"type": "direct", "tag": "direct", "domain_resolver": "dns-local"}, map[string]any{"type": "block", "tag": "block"})
	if cfg.CaptureEnabled && len(normalizeCaptureDomains(cfg.CaptureDomains)) > 0 {
		captureOutbound := map[string]any{
			"type": "http", "tag": "capture", "server": "127.0.0.1", "server_port": captureProxyPort,
		}
		if auth.proxyEnabled() {
			captureOutbound["username"] = auth.ProxyUsername
			captureOutbound["password"] = auth.ProxyPassword
		}
		outbounds = append(outbounds, captureOutbound)
	}
	tunInbound := map[string]any{
		"type": "tun", "tag": "tun-in",
		"address":    tunAddresses(cfg.IPv6Enabled),
		"auto_route": true, "strict_route": cfg.WebRTCProtectionEnabled, "stack": "system",
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		tunInbound["interface_name"] = tun.InterfaceName
	}
	// 直连型节点的 IP 在系统路由层绕过 TUN，Ping/TCPing 测的是节点本身，不经当前节点。
	exclude, shared := nodeAddrs(cfg.Nodes, resolved, cfg.IPv6Enabled)
	if len(exclude) > 0 {
		tunInbound["route_exclude_address"] = exclude
	}
	if runtime.GOOS == "android" {
		tunInbound = mobileInbound(auth)
	}
	inbounds := []map[string]any{tunInbound}
	if cfg.CaptureEnabled && len(normalizeCaptureDomains(cfg.CaptureDomains)) > 0 {
		upstream := map[string]any{
			"type": "mixed", "tag": "capture-upstream",
			"listen": "127.0.0.1", "listen_port": captureUpstreamPort,
		}
		if auth.proxyEnabled() {
			upstream["users"] = []map[string]any{{"username": auth.ProxyUsername, "password": auth.ProxyPassword}}
		}
		inbounds = append(inbounds, upstream)
	}
	clashAPI := map[string]any{"external_controller": "127.0.0.1:9090"}
	if auth.ClashSecret != "" {
		clashAPI["secret"] = auth.ClashSecret
	}
	result := RuntimeConfig{
		Schema:    "https://sing-box.sagernet.org/schema.json",
		Log:       map[string]any{"level": "info", "timestamp": true},
		DNS:       buildDNS(cfg.DNSMode, cfg.RoutingMode, NodeDomains(cfg.Nodes), hasRuleSet(ruleSets, "geosite-cn")),
		Inbounds:  inbounds,
		Outbounds: outbounds,
		Endpoints: endpoints,
		Route:     routeForMode(cfg, ruleSets, targets, exeDir, shared),
		Experimental: map[string]any{
			"cache_file": map[string]any{"enabled": true},
			"clash_api":  clashAPI,
		},
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("生成 sing-box 配置失败: %w", err)
	}
	return data, nil
}

// MobileProxyPort 是安卓版 sing-box 的本地入站端口。安卓上 TUN 由系统 VpnService 创建，
// APK 把 TUN 流量经 tun2socks 转到这个端口；标签仍叫 tun-in，路由规则不用改。
const MobileProxyPort = 10853

func mobileInbound(auth LocalAuth) map[string]any {
	in := map[string]any{"type": "mixed", "tag": "tun-in", "listen": "127.0.0.1", "listen_port": MobileProxyPort}
	if auth.proxyEnabled() {
		in["users"] = []map[string]any{{"username": auth.ProxyUsername, "password": auth.ProxyPassword}}
	}
	return in
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

func hasRuleSet(sets []configuredRuleSet, id string) bool {
	for _, set := range sets {
		if set.ID == id {
			return true
		}
	}
	return false
}

// buildDNS 生成 DNS 配置。geositeCN 表示 route 里确实有 geosite-cn 规则集（启用且文件存在），
// 否则 DNS 规则不能引用它，sing-box 会报 rule-set not found 拒绝整份配置。
func buildDNS(mode config.DNSMode, routeMode config.RoutingMode, nodeDomains []string, geositeCN bool) map[string]any {
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
	// Node domains always resolve locally: tests must not depend on the
	// current node, which may be the one that is down.
	if len(nodeDomains) > 0 {
		rules = append(rules, map[string]any{"domain": nodeDomains, "server": "dns-local"})
	}
	if mode == config.DNSAuto && routeMode == config.RoutingSmart {
		if geositeCN {
			rules = append(rules, map[string]any{"rule_set": []string{"geosite-cn"}, "server": "dns-local"})
		}
		rules = append(rules, map[string]any{"ip_is_private": true, "server": "dns-local"})
	}
	if mode == config.DNSCustom {
		final = "dns-remote"
	}
	return map[string]any{"servers": servers, "rules": rules, "final": final, "strategy": "prefer_ipv4"}
}

// shared 是走 CDN 的节点 IP：不能整体绕过 TUN，只让发往它们的 Ping（ICMP）直连，代理出站转发不了 ICMP。
func routeForMode(cfg config.Config, ruleSets []configuredRuleSet, targets map[string]string, exeDir string, shared []string) map[string]any {
	mode, captureEnabled, captureDomains := cfg.RoutingMode, cfg.CaptureEnabled, cfg.CaptureDomains
	private := map[string]any{"ip_is_private": true, "outbound": "direct"}
	dns := map[string]any{"protocol": "dns", "action": "hijack-dns"}
	base := []map[string]any{
		{"inbound": []string{"tun-in"}, "action": "sniff", "timeout": "1s"},
		dns,
	}
	if len(shared) > 0 && runtime.GOOS != "android" {
		base = append(base, map[string]any{"network": "icmp", "ip_cidr": shared, "outbound": "direct"})
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
	// 屏蔽规则最先匹配，不会被分流规则抢走。
	for _, r := range cfg.DNSFilterRules {
		if rr, ok := customRule(r, nil); ok {
			base = append(base, rr)
		}
	}
	for _, r := range cfg.CustomRules {
		if mode == config.RoutingDirect && !config.IsBlockAction(r.Action) {
			continue
		}
		if rr, ok := customRule(r, targets); ok {
			base = append(base, rr)
		}
	}
	for _, set := range ruleSets {
		base = append(base, map[string]any{"rule_set": []string{set.ID}, "outbound": ruleOutbound(set.Action, targets)})
	}
	base = append(base, private)
	final := "proxy"
	if mode == config.RoutingDirect {
		final = "direct"
	}

	ruleSetConfigs := make([]map[string]any, 0, len(ruleSets))
	for _, set := range ruleSets {
		ruleSetConfigs = append(ruleSetConfigs, map[string]any{"type": "local", "tag": set.ID, "format": rules.FileFormat(set.Path), "path": filepath.Join(exeDir, "rules", filepath.Base(set.Path))})
	}
	return map[string]any{
		// 安卓上 sing-box 进程已被 VPN 排除，不需要（也未必有权限）监听网卡变化。
		"auto_detect_interface":   runtime.GOOS != "android",
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

// activeRuleSets 返回要写进配置的规则集：用户添加的排在前面且优先，直连模式下只留屏蔽类；
// 内置的国内/国外规则集只在智能模式下用。
func activeRuleSets(mode config.RoutingMode, exeDir string) []configuredRuleSet {
	sets := configuredRuleSets(exeDir)
	userSets := make([]configuredRuleSet, 0, len(sets))
	defaultSets := make([]configuredRuleSet, 0, len(sets))
	for _, set := range sets {
		if !set.Enabled || !ruleSetExists(exeDir, filepath.Base(set.Path)) {
			continue
		}
		switch {
		case set.Source == "default":
			if mode == config.RoutingSmart {
				defaultSets = append(defaultSets, set)
			}
		case mode != config.RoutingDirect || config.IsBlockAction(set.Action):
			userSets = append(userSets, set)
		}
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

func customRule(r config.Rule, targets map[string]string) (map[string]any, bool) {
	value := strings.TrimSpace(r.Value)
	if value == "" {
		return nil, false
	}
	outbound := ruleOutbound(r.Action, targets)
	key := strings.ToLower(strings.TrimSpace(r.MatchType))
	rule := map[string]any{"outbound": outbound}
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

// ruleOutbound 把规则去向换成出站标签；指向节点/节点组的目标没了（节点删除、组里没有可用节点）
// 就退回当前节点。
func ruleOutbound(action string, targets map[string]string) string {
	if config.IsBasicAction(action) {
		kind, _ := config.SplitAction(action)
		return kind
	}
	if tag := targets[strings.TrimSpace(action)]; tag != "" {
		return tag
	}
	return "proxy"
}

func parsePort(s string) int {
	var p int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &p); err != nil || p < 1 || p > 65535 {
		return 0
	}
	return p
}

func buildOutbound(n config.Node) (map[string]any, error) {
	var out map[string]any
	switch protocol := strings.ToLower(strings.TrimSpace(n.Protocol)); protocol {
	case "hysteria2":
		out = buildHysteria2(map[string]any{"tag": "proxy"}, n)
	case "hysteria":
		out = buildHysteria(map[string]any{"tag": "proxy"}, n)
	case "wireguard":
		return buildWireGuard(n)
	case "vless", "vmess", "trojan", "shadowsocks", "socks", "http", "tuic", "anytls":
		out = buildStandardOutbound(protocol, n)
	default:
		return nil, fmt.Errorf("暂不支持的节点协议: %s", n.Protocol)
	}
	// 单节点校验：sing-box 必然拒绝的取值在这里报错，由 buildSelector 跳过该节点。
	if err := checkOutbound(out); err != nil {
		return nil, err
	}
	return out, nil
}

func buildStandardOutbound(protocol string, n config.Node) map[string]any {
	out := map[string]any{"type": protocol, "tag": "proxy", "server": n.Server, "server_port": n.Port, "domain_resolver": "dns-local"}
	// TLS 类参数只在 tls 开关打开时生效：security=none 的链接常带着 sni/fp，不能因此开 TLS。
	tlsOn := tlsEnabled(protocol, n.Settings)
	for k, v := range n.Settings {
		if strings.HasPrefix(k, "transport_") || isTLSKey(k) && !tlsOn {
			continue
		}
		addSetting(out, k, v)
	}
	if tls, _ := out["tls"].(map[string]any); tls != nil && tls["reality"] != nil && tls["utls"] == nil {
		// sing-box 的 REALITY 客户端必须开 uTLS，链接里没写 fp 时用 chrome。
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": "chrome"}
	}
	if protocol == "tuic" || protocol == "anytls" {
		// 这两个协议的 TLS 是必填项，分享链接通常不单独写 tls=1。
		tls, _ := out["tls"].(map[string]any)
		if tls == nil {
			tls = map[string]any{}
			out["tls"] = tls
		}
		tls["enabled"] = true
		if tls["server_name"] == nil {
			tls["server_name"] = n.Server
		}
		if protocol == "tuic" && tls["alpn"] == nil {
			tls["alpn"] = []string{"h3"}
		}
	}
	addTransport(out, n.Settings)
	keepOutboundFields(out)
	return out
}

func settingOn(v string) bool { return v == "1" || strings.EqualFold(v, "true") }

func tlsEnabled(protocol string, settings map[string]string) bool {
	return protocol == "tuic" || protocol == "anytls" || settingOn(settings["tls"]) || settings["reality_public_key"] != ""
}

func isTLSKey(k string) bool {
	switch k {
	case "server_name", "insecure", "alpn", "utls_fingerprint", "reality_public_key", "reality_short_id":
		return true
	}
	return false
}

// buildWireGuard 生成 sing-box 1.11+ 的 WireGuard endpoint（旧的 wireguard 出站已废弃）。
func buildWireGuard(n config.Node) (map[string]any, error) {
	settings := n.Settings
	addresses := splitList(settings["local_address"])
	if settings["private_key"] == "" || settings["public_key"] == "" || len(addresses) == 0 {
		return nil, fmt.Errorf("WireGuard 节点缺少私钥、对端公钥或本机地址")
	}
	for i, address := range addresses {
		if !strings.Contains(address, "/") {
			if strings.Contains(address, ":") {
				addresses[i] = address + "/128"
			} else {
				addresses[i] = address + "/32"
			}
		}
	}
	allowed := splitList(settings["allowed_ips"])
	if len(allowed) == 0 {
		allowed = []string{"0.0.0.0/0", "::/0"}
	}
	peer := map[string]any{
		"address": n.Server, "port": n.Port,
		"public_key": settings["public_key"], "allowed_ips": allowed,
	}
	if v := settings["pre_shared_key"]; v != "" {
		peer["pre_shared_key"] = v
	}
	if v := settings["reserved"]; v != "" {
		reserved, err := parseWireGuardReserved(v)
		if err != nil {
			return nil, err
		}
		peer["reserved"] = reserved
	}
	out := map[string]any{
		"type": "wireguard", "tag": "proxy",
		"address": addresses, "private_key": settings["private_key"],
		"peers": []map[string]any{peer}, "domain_resolver": "dns-local",
	}
	if v, err := strconv.Atoi(settings["mtu"]); err == nil && v > 0 {
		out["mtu"] = v
	}
	return out, nil
}

// parseWireGuardReserved 接受 "1,2,3" 或 Clash 常见的 base64 写法（如 "AQID"）。
func parseWireGuardReserved(value string) ([]int, error) {
	parts := splitList(value)
	reserved := make([]int, 0, 3)
	if len(parts) == 1 {
		if raw, err := base64.StdEncoding.DecodeString(parts[0]); err == nil && len(raw) == 3 {
			for _, b := range raw {
				reserved = append(reserved, int(b))
			}
			return reserved, nil
		}
	}
	for _, part := range parts {
		b, err := strconv.Atoi(part)
		if err != nil || b < 0 || b > 255 {
			return nil, fmt.Errorf("WireGuard reserved 无效: %s", value)
		}
		reserved = append(reserved, b)
	}
	if len(reserved) != 3 {
		return nil, fmt.Errorf("WireGuard reserved 必须是 3 个字节: %s", value)
	}
	return reserved, nil
}

// splitEndpoints 把 WireGuard 这类 endpoint 从出站列表中拆出来，selector 仍可按 tag 引用它们。
func splitEndpoints(items []map[string]any) (outbounds, endpoints []map[string]any) {
	for _, item := range items {
		if item["type"] == "wireguard" {
			endpoints = append(endpoints, item)
		} else {
			outbounds = append(outbounds, item)
		}
	}
	return outbounds, endpoints
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
			if _, err := strconv.Atoi(v); err == nil {
				v += "s" // 链接里常写纯数字秒数，sing-box 要求带单位
			}
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

// Hysteria 第一版必须填上下行带宽，节点没写时用这两个默认值。
const (
	defaultHysteriaUpMbps   = 20
	defaultHysteriaDownMbps = 100
)

// buildHysteria 生成 Hysteria 第一版出站：密码对应 auth_str，混淆只有一个密码字段。
func buildHysteria(out map[string]any, n config.Node) map[string]any {
	out = buildHysteria2(out, n)
	out["type"] = "hysteria"
	delete(out, "password")
	delete(out, "obfs")
	delete(out, "disable_path_mtu_discovery")
	if v := n.Settings["password"]; v != "" {
		out["auth_str"] = v
	}
	if v := n.Settings["obfs_password"]; v != "" {
		out["obfs"] = v
	}
	if v, _ := out["up_mbps"].(int); v <= 0 {
		out["up_mbps"] = defaultHysteriaUpMbps
	}
	if v, _ := out["down_mbps"].(int); v <= 0 {
		out["down_mbps"] = defaultHysteriaDownMbps
	}
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
	case "uuid", "password", "network", "username", "version", "packet_encoding", "plugin", "plugin_opts", "path", "congestion_control", "udp_relay_mode":
		out[key] = value
	case "method", "security":
		out[key] = strings.ToLower(value)
	case "flow":
		if value != "none" {
			out[key] = value
		}
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
			if key == "reality_public_key" {
				// sing-box 只认 base64url 无填充写法，标准 base64 公钥顺手转换。
				value = strings.NewReplacer("+", "-", "/", "_").Replace(strings.TrimRight(value, "="))
			}
			reality[strings.TrimPrefix(key, "reality_")] = value
		case "utls_fingerprint":
			if fp := utlsFingerprint(value); fp != "" {
				tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
			}
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

// addTransport 按传输类型只写 sing-box 认识的字段：gRPC 没有 host/path，多写一个字段整份配置都会被拒。
func addTransport(out map[string]any, settings map[string]string) {
	typeName := settings["transport_type"]
	if typeName == "" {
		typeName = settings["transport"]
	}
	transport := map[string]any{"type": typeName}
	path, host := settings["transport_path"], settings["transport_host"]
	switch typeName {
	case "ws", "httpupgrade":
		if path != "" {
			transport["path"] = path
		}
		if host != "" {
			transport["headers"] = map[string]any{"Host": host}
		}
	case "http":
		if path != "" {
			transport["path"] = path
		}
		if host != "" {
			transport["host"] = splitList(host)
		}
	case "grpc":
		if service := firstNonEmpty(settings["transport_service_name"], strings.TrimPrefix(path, "/")); service != "" {
			transport["service_name"] = service
		}
	case "quic":
	default:
		return // 未设置或不认识的传输类型不写
	}
	out["transport"] = transport
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func splitList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })
	return parts
}

// normalizePortRanges 把 "443,2000-3000" 转成 sing-box 的 ["443:443","2000:3000"]；单个端口也必须写成范围。
func normalizePortRanges(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })
	for i, part := range parts {
		part = strings.Replace(part, "-", ":", 1)
		if !strings.Contains(part, ":") {
			part += ":" + part
		}
		parts[i] = part
	}
	return parts
}

// NodeDomains returns the distinct node servers that are domain names.
func NodeDomains(nodes []config.Node) []string {
	seen := make(map[string]struct{})
	var domains []string
	for _, node := range nodes {
		server := strings.ToLower(strings.TrimSpace(node.Server))
		if server == "" {
			continue
		}
		if _, ok := literalIP(server); ok {
			continue
		}
		if _, ok := seen[server]; ok {
			continue
		}
		seen[server] = struct{}{}
		domains = append(domains, server)
	}
	sort.Strings(domains)
	return domains
}

// literalIP reports whether a node server is an IP literal, with or without
// IPv6 brackets.
func literalIP(server string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(strings.Trim(strings.TrimSpace(server), "[]"))
	return addr, err == nil
}

// cloudflareRanges 是 Cloudflare 公布的网段（https://www.cloudflare.com/ips-v4 、ips-v6）。
var cloudflareRanges = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range strings.Fields(`173.245.48.0/20 103.21.244.0/22 103.22.200.0/22 103.31.4.0/22 141.101.64.0/18
		108.162.192.0/18 190.93.240.0/20 188.114.96.0/20 197.234.240.0/22 198.41.128.0/17 162.158.0.0/15
		104.16.0.0/13 104.24.0.0/14 172.64.0.0/13 131.0.72.0/22 2400:cb00::/32 2606:4700::/32 2803:f800::/32
		2405:b500::/32 2405:8100::/32 2a06:98c0::/29 2c0f:f248::/32`) {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// BypassesTUN 报告运行中发往节点地址 addr 的流量是否绕过 TUN：内网地址本来就不进 TUN；
// 公网地址只有直连型节点才写进 route_exclude_address。走 CDN 的节点（ws/httpupgrade/grpc/xhttp 加 TLS，或 IP 在
// Cloudflare 网段）的 IP 是很多网站共用的，排除会让这些网站的流量全部绕过分流和屏蔽规则。
func BypassesTUN(node config.Node, addr netip.Addr) bool {
	addr = addr.Unmap().WithZone("")
	if !entersTUN(addr) {
		return true
	}
	switch firstNonEmpty(node.Settings["transport_type"], node.Settings["transport"]) {
	case "ws", "httpupgrade", "grpc", "xhttp", "splithttp":
		if tlsEnabled(strings.ToLower(strings.TrimSpace(node.Protocol)), node.Settings) {
			return false
		}
	}
	for _, prefix := range cloudflareRanges {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

// entersTUN 报告地址会不会被 TUN 接管：内网、回环、链路本地地址不会。
func entersTUN(addr netip.Addr) bool {
	return addr.IsValid() && !addr.IsLoopback() && !addr.IsPrivate() && !addr.IsUnspecified() && !addr.IsLinkLocalUnicast()
}

// nodeAddrs 把节点 IP（字面 IP 或解析结果）分成两组主机前缀：exclude 写进 route_exclude_address，
// shared 是走 CDN 的节点 IP。不进 TUN 的地址跳过；IPv6 关闭时跳过 IPv6。
// 同一个 IP 两组都有时按 shared 算，宁可测速多绕一层也不漏掉网站流量。
func nodeAddrs(nodes []config.Node, resolved map[string][]netip.Addr, ipv6Enabled bool) (exclude, shared []string) {
	bypass := make(map[netip.Addr]bool)
	for _, node := range nodes {
		addrs := resolved[strings.ToLower(strings.TrimSpace(node.Server))]
		if addr, ok := literalIP(node.Server); ok {
			addrs = []netip.Addr{addr}
		}
		for _, addr := range addrs {
			addr = addr.Unmap().WithZone("")
			if !entersTUN(addr) || (addr.Is6() && !ipv6Enabled) {
				continue
			}
			ok, seen := bypass[addr]
			bypass[addr] = BypassesTUN(node, addr) && (ok || !seen)
		}
	}
	for addr, ok := range bypass {
		prefix := netip.PrefixFrom(addr, addr.BitLen()).String()
		if ok {
			exclude = append(exclude, prefix)
		} else {
			shared = append(shared, prefix)
		}
	}
	sort.Strings(exclude)
	sort.Strings(shared)
	return exclude, shared
}
