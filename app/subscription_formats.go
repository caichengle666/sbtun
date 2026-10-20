package app

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/caichengle666/sbtun/config"
)

// 订阅正文除了分享链接列表，还常见 Clash YAML 与 sing-box JSON 两种格式。
// 这里把它们统一转换成 config.Node，节点 ID 规则与分享链接一致，方便重复导入去重。

func looksLikeClashYAML(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "proxies:") {
			return true
		}
	}
	return false
}

func parseClashSubscription(text string) ([]config.Node, error) {
	var doc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("Clash 订阅解析失败: %w", err)
	}
	nodes := make([]config.Node, 0, len(doc.Proxies))
	for _, proxy := range doc.Proxies {
		if node, ok := clashNode(proxy); ok {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func clashNode(m map[string]any) (config.Node, bool) {
	kind := strings.ToLower(anyString(m["type"]))
	server, portText := anyString(m["server"]), anyString(m["port"])
	// mihomo 的 WireGuard 可以把地址写在 peers 列表里，取第一个 peer。
	var peer map[string]any
	if peers, ok := m["peers"].([]any); ok && len(peers) > 0 && kind == "wireguard" {
		peer = anyMap(peers[0])
		server = firstNonEmpty(anyString(peer["server"]), server)
		portText = firstNonEmpty(anyString(peer["port"]), portText)
	}
	port, err := parsePort(portText)
	if server == "" || err != nil {
		return config.Node{}, false
	}
	s := map[string]string{}
	set := func(target, key string) {
		if v := anyString(m[key]); v != "" {
			s[target] = v
		}
	}
	setTLS := func(sniKey string) {
		if anyBool(m["tls"]) {
			s["tls"] = "true"
		}
		set("server_name", sniKey)
		if anyBool(m["skip-cert-verify"]) {
			s["insecure"] = "true"
		}
		if alpn := anyList(m["alpn"]); len(alpn) > 0 {
			s["alpn"] = strings.Join(alpn, ",")
		}
		set("utls_fingerprint", "client-fingerprint")
	}
	protocol := kind
	switch kind {
	case "ss":
		protocol = "shadowsocks"
		set("method", "cipher")
		set("password", "password")
		if s["method"] == "" || s["password"] == "" {
			return config.Node{}, false
		}
		if !clashSSPlugin(s, anyString(m["plugin"]), anyMap(m["plugin-opts"])) {
			return config.Node{}, false
		}
		if anyBool(m["udp-over-tcp"]) {
			s["udp_over_tcp"] = "true"
		}
	case "vmess", "vless":
		set("uuid", "uuid")
		if s["uuid"] == "" {
			return config.Node{}, false
		}
		if kind == "vmess" {
			set("alter_id", "alterId")
			set("security", "cipher")
		} else {
			set("flow", "flow")
		}
		setTLS("servername")
		if reality := anyMap(m["reality-opts"]); reality != nil {
			s["tls"] = "true"
			if v := anyString(reality["public-key"]); v != "" {
				s["reality_public_key"] = v
			}
			if v := anyString(reality["short-id"]); v != "" {
				s["reality_short_id"] = v
			}
		}
		if !clashTransport(s, m) {
			return config.Node{}, false
		}
	case "trojan":
		set("password", "password")
		s["tls"] = "true"
		setTLS("sni")
		if !clashTransport(s, m) {
			return config.Node{}, false
		}
	case "hysteria2":
		set("password", "password")
		setTLS("sni")
		delete(s, "tls")
		if v := s["server_name"]; v != "" {
			s["sni"] = v
			delete(s, "server_name")
		}
		set("up_mbps", "up")
		set("down_mbps", "down")
		set("server_ports", "ports")
		set("obfs_type", "obfs")
		set("obfs_password", "obfs-password")
	case "hysteria":
		// sing-box 只支持 udp 协议，faketcp、wechat-video 跳过。
		if p := strings.ToLower(anyString(m["protocol"])); p != "" && p != "udp" {
			return config.Node{}, false
		}
		s["password"] = firstNonEmpty(anyString(m["auth-str"]), anyString(m["auth_str"]), decodeBase64Text(anyString(m["auth"])))
		setTLS("sni")
		delete(s, "tls")
		if v := s["server_name"]; v != "" {
			s["sni"] = v
			delete(s, "server_name")
		}
		s["up_mbps"] = bandwidthMbps(anyString(m["up"]))
		s["down_mbps"] = bandwidthMbps(anyString(m["down"]))
		set("server_ports", "ports")
		set("obfs_password", "obfs")
		for key, value := range s {
			if value == "" {
				delete(s, key)
			}
		}
	case "tuic":
		set("uuid", "uuid")
		set("password", "password")
		if s["uuid"] == "" {
			return config.Node{}, false
		}
		setTLS("sni")
		set("congestion_control", "congestion-controller")
		set("udp_relay_mode", "udp-relay-mode")
	case "anytls":
		set("password", "password")
		setTLS("sni")
	case "wireguard":
		set("private_key", "private-key")
		set("public_key", "public-key")
		set("pre_shared_key", "pre-shared-key")
		set("mtu", "mtu")
		addresses := []string{}
		for _, key := range []string{"ip", "ipv6"} {
			if v := anyString(m[key]); v != "" {
				addresses = append(addresses, v)
			}
		}
		s["local_address"] = strings.Join(addresses, ",")
		if reserved := anyList(m["reserved"]); len(reserved) > 0 {
			s["reserved"] = strings.Join(reserved, ",")
		}
		if ips := anyList(m["allowed-ips"]); len(ips) > 0 {
			s["allowed_ips"] = strings.Join(ips, ",")
		}
		if peer != nil {
			for target, key := range map[string]string{"public_key": "public-key", "pre_shared_key": "pre-shared-key"} {
				if v := anyString(peer[key]); v != "" {
					s[target] = v
				}
			}
			if v := anyList(peer["reserved"]); len(v) > 0 {
				s["reserved"] = strings.Join(v, ",")
			}
			if v := anyList(peer["allowed-ips"]); len(v) > 0 {
				s["allowed_ips"] = strings.Join(v, ",")
			}
		}
		if s["private_key"] == "" || s["public_key"] == "" || s["local_address"] == "" {
			return config.Node{}, false
		}
	case "socks5", "http":
		if kind == "socks5" {
			protocol = "socks"
			s["version"] = "5"
		}
		set("username", "username")
		set("password", "password")
		setTLS("sni")
		if protocol == "socks" && s["tls"] == "true" {
			return config.Node{}, false // sing-box 的 SOCKS 出站不支持 TLS
		}
	default:
		return config.Node{}, false
	}
	if kind == "vmess" || kind == "vless" {
		set("packet_encoding", "packet-encoding")
	}
	// 只有这几种协议支持 multiplex。
	if smux := anyMap(m["smux"]); smux != nil && anyBool(smux["enabled"]) && (kind == "vmess" || kind == "vless" || kind == "trojan" || kind == "ss") {
		s["multiplex"] = "true"
	}
	if (kind == "vmess" || kind == "vless" || kind == "socks5" || kind == "http") && s["tls"] != "true" {
		// 未开启 TLS 时 servername 等字段没有意义，留着会被当成开启 TLS。
		for _, key := range []string{"server_name", "insecure", "alpn", "utls_fingerprint"} {
			delete(s, key)
		}
	}
	name := anyString(m["name"])
	if name == "" {
		name = protocol + "-" + server
	}
	return config.Node{ID: subscriptionNodeID(protocol, server, port), Name: name, Protocol: protocol, Server: server, Port: port, Settings: s}, true
}

// clashTransport 返回 false 表示传输方式（如 xhttp）sbtun 暂不支持，节点应跳过。
func clashTransport(s map[string]string, m map[string]any) bool {
	network := strings.ToLower(anyString(m["network"]))
	switch network {
	case "", "tcp":
	case "ws":
		s["transport_type"] = "ws"
		opts := anyMap(m["ws-opts"])
		if anyBool(opts["v2ray-http-upgrade"]) {
			s["transport_type"] = "httpupgrade"
		}
		if v := anyString(opts["path"]); v != "" {
			s["transport_path"] = v
		}
		if headers := anyMap(opts["headers"]); headers != nil {
			if v := firstNonEmpty(anyString(headers["Host"]), anyString(headers["host"])); v != "" {
				s["transport_host"] = v
			}
		}
	case "grpc":
		s["transport_type"] = "grpc"
		if v := anyString(anyMap(m["grpc-opts"])["grpc-service-name"]); v != "" {
			s["transport_service_name"] = v
		}
	case "h2", "http":
		s["transport_type"] = "http"
		opts := anyMap(m[network+"-opts"])
		if hosts := anyList(opts["host"]); len(hosts) > 0 {
			s["transport_host"] = hosts[0]
		}
		if paths := anyList(opts["path"]); len(paths) > 0 {
			s["transport_path"] = paths[0]
		}
	default:
		return false
	}
	return true
}

// clashSSPlugin 把 Clash 的 obfs / v2ray-plugin 写法转换成 sing-box 的 plugin 与 plugin_opts；
// 其他插件（shadow-tls、restls 等）返回 false，节点跳过。
func clashSSPlugin(s map[string]string, plugin string, opts map[string]any) bool {
	switch plugin {
	case "":
	case "obfs":
		parts := []string{"obfs=" + firstNonEmpty(anyString(opts["mode"]), "http")}
		if v := anyString(opts["host"]); v != "" {
			parts = append(parts, "obfs-host="+v)
		}
		s["plugin"] = "obfs-local"
		s["plugin_opts"] = strings.Join(parts, ";")
	case "v2ray-plugin":
		parts := []string{"mode=" + firstNonEmpty(anyString(opts["mode"]), "websocket")}
		if anyBool(opts["tls"]) {
			parts = append(parts, "tls")
		}
		for _, key := range []string{"host", "path"} {
			if v := anyString(opts[key]); v != "" {
				parts = append(parts, key+"="+v)
			}
		}
		s["plugin"] = "v2ray-plugin"
		s["plugin_opts"] = strings.Join(parts, ";")
	default:
		return false
	}
	return true
}

func parseSingBoxSubscription(text string) ([]config.Node, error) {
	var doc struct {
		Outbounds []map[string]any `json:"outbounds"`
		Endpoints []map[string]any `json:"endpoints"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("sing-box 订阅解析失败: %w", err)
	}
	nodes := []config.Node{}
	for _, item := range append(doc.Outbounds, doc.Endpoints...) {
		if node, ok := singBoxNode(item); ok {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func singBoxNode(m map[string]any) (config.Node, bool) {
	protocol := strings.ToLower(anyString(m["type"]))
	server := anyString(m["server"])
	portText := anyString(m["server_port"])
	s := map[string]string{}
	for _, key := range []string{"uuid", "password", "method", "flow", "security", "alter_id", "username", "version", "plugin", "plugin_opts", "congestion_control", "udp_relay_mode", "up_mbps", "down_mbps", "hop_interval", "path", "network", "packet_encoding"} {
		if v := anyString(m[key]); v != "" {
			s[key] = v
		}
	}
	// multiplex、udp_over_tcp 在 sing-box 里可以是对象 {"enabled": true} 或布尔值。
	for _, key := range []string{"multiplex", "udp_over_tcp"} {
		if v, ok := m[key].(map[string]any); ok && anyBool(v["enabled"]) || anyBool(m[key]) {
			s[key] = "true"
		}
	}
	switch protocol {
	case "vmess", "vless", "trojan", "shadowsocks", "socks", "http", "tuic", "anytls":
	case "hysteria", "hysteria2":
		if ports := anyList(m["server_ports"]); len(ports) > 0 {
			// sing-box 写 2000:3000，链接和界面里统一写 2000-3000。
			for i := range ports {
				ports[i] = strings.Replace(ports[i], ":", "-", 1)
			}
			s["server_ports"] = strings.Join(ports, ",")
		}
		if protocol == "hysteria" {
			s["password"] = firstNonEmpty(anyString(m["auth_str"]), decodeBase64Text(anyString(m["auth"])))
			s["obfs_password"] = anyString(m["obfs"])
			if s["up_mbps"] == "" {
				s["up_mbps"] = bandwidthMbps(anyString(m["up"]))
			}
			if s["down_mbps"] == "" {
				s["down_mbps"] = bandwidthMbps(anyString(m["down"]))
			}
			for key, value := range s {
				if value == "" {
					delete(s, key)
				}
			}
			break
		}
		if obfs := anyMap(m["obfs"]); obfs != nil {
			s["obfs_type"] = anyString(obfs["type"])
			s["obfs_password"] = anyString(obfs["password"])
		}
	case "wireguard":
		addresses := anyList(m["address"])
		if len(addresses) == 0 {
			addresses = anyList(m["local_address"]) // 旧版 wireguard 出站写法
		}
		s["local_address"] = strings.Join(addresses, ",")
		s["private_key"] = anyString(m["private_key"])
		s["public_key"] = anyString(m["peer_public_key"])
		s["pre_shared_key"] = anyString(m["pre_shared_key"])
		s["mtu"] = anyString(m["mtu"])
		s["reserved"] = strings.Join(anyList(m["reserved"]), ",")
		if peers, ok := m["peers"].([]any); ok && len(peers) > 0 {
			peer := anyMap(peers[0])
			server = anyString(peer["address"])
			portText = anyString(peer["port"])
			s["public_key"] = anyString(peer["public_key"])
			s["pre_shared_key"] = anyString(peer["pre_shared_key"])
			s["reserved"] = strings.Join(anyList(peer["reserved"]), ",")
			s["allowed_ips"] = strings.Join(anyList(peer["allowed_ips"]), ",")
		}
		for key, value := range s {
			if value == "" {
				delete(s, key)
			}
		}
		if s["private_key"] == "" || s["public_key"] == "" || s["local_address"] == "" {
			return config.Node{}, false
		}
	default:
		return config.Node{}, false
	}
	port, err := parsePort(portText)
	if server == "" || err != nil {
		return config.Node{}, false
	}
	if tls := anyMap(m["tls"]); tls != nil && anyBool(tls["enabled"]) {
		s["tls"] = "true"
		if v := anyString(tls["server_name"]); v != "" {
			s["server_name"] = v
		}
		if anyBool(tls["insecure"]) {
			s["insecure"] = "true"
		}
		if alpn := anyList(tls["alpn"]); len(alpn) > 0 {
			s["alpn"] = strings.Join(alpn, ",")
		}
		if utls := anyMap(tls["utls"]); utls != nil && anyBool(utls["enabled"]) {
			s["utls_fingerprint"] = anyString(utls["fingerprint"])
		}
		if reality := anyMap(tls["reality"]); reality != nil && anyBool(reality["enabled"]) {
			s["reality_public_key"] = anyString(reality["public_key"])
			s["reality_short_id"] = anyString(reality["short_id"])
		}
		if protocol == "hysteria" || protocol == "hysteria2" {
			delete(s, "tls")
			if v := s["server_name"]; v != "" {
				s["sni"] = v
				delete(s, "server_name")
			}
		}
	}
	if transport := anyMap(m["transport"]); transport != nil {
		s["transport_type"] = anyString(transport["type"])
		if v := anyString(transport["path"]); v != "" {
			s["transport_path"] = v
		}
		headers := anyMap(transport["headers"])
		if hosts := anyList(firstPresent(headers["Host"], headers["host"], transport["host"])); len(hosts) > 0 {
			s["transport_host"] = hosts[0]
		}
		if v := anyString(transport["service_name"]); v != "" {
			s["transport_service_name"] = v
		}
	}
	name := anyString(m["tag"])
	if name == "" {
		name = protocol + "-" + server
	}
	return config.Node{ID: subscriptionNodeID(protocol, server, port), Name: name, Protocol: protocol, Server: server, Port: port, Settings: s}, true
}

// bandwidthMbps 把 "30 Mbps"、"30"、"1 Gbps" 这样的带宽换算成 Mbps 数字。
func bandwidthMbps(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	end := 0
	for end < len(value) && (value[end] >= '0' && value[end] <= '9' || value[end] == '.') {
		end++
	}
	n, err := strconv.ParseFloat(value[:end], 64)
	if err != nil || n <= 0 {
		return ""
	}
	unit := strings.TrimSpace(value[end:])
	switch {
	case strings.HasPrefix(unit, "g"):
		n *= 1000
	case strings.HasPrefix(unit, "k"):
		n /= 1000
	}
	if n < 1 {
		n = 1
	}
	return strconv.Itoa(int(n))
}

func decodeBase64Text(value string) string {
	if value == "" {
		return ""
	}
	decoded, err := tryBase64Decode(value)
	if err != nil {
		return ""
	}
	return decoded
}

// subscriptionNodeID 与各分享链接解析器生成的 ID 保持一致。
func subscriptionNodeID(protocol, server string, port uint16) string {
	portText := strconv.Itoa(int(port))
	switch protocol {
	case "vmess":
		return "vmess-" + server + "-" + portText
	case "shadowsocks":
		return "ss-" + server + "-" + portText
	}
	return protocol + "-" + net.JoinHostPort(server, portText)
}

func anyString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return ""
	}
}

func firstPresent(values ...any) any {
	for _, v := range values {
		if anyString(v) != "" || len(anyList(v)) > 0 {
			return v
		}
	}
	return nil
}

func anyBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "1" || strings.EqualFold(t, "true")
	case int:
		return t != 0
	case float64:
		return t != 0
	}
	return false
}

func anyMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// anyList 接受数组，或逗号分隔的字符串（Clash 的 reserved / alpn 两种写法都有）。
func anyList(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s := anyString(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		return strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ' ' })
	case nil:
		return nil
	default:
		if s := anyString(t); s != "" {
			return []string{s}
		}
		return nil
	}
}
