package app

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/caichengle666/sbtun/config"
)

// 这里处理分享链接以外的几种文本格式：
//   - SIP008：Shadowsocks 的 JSON 订阅 {"version":1,"servers":[...]}
//   - Surge / Loon 配置行：名称 = 类型, 地址, 端口, 参数...
//   - Quantumult X 配置行：类型=地址:端口, 参数..., tag=名称

// isSIP008 判断 JSON 订阅是不是 SIP008 格式。
func isSIP008(text string) bool {
	var doc struct {
		Servers   []json.RawMessage `json:"servers"`
		Outbounds []json.RawMessage `json:"outbounds"`
	}
	return json.Unmarshal([]byte(text), &doc) == nil && len(doc.Servers) > 0 && len(doc.Outbounds) == 0
}

func parseSIP008Subscription(text string) ([]config.Node, error) {
	var doc struct {
		Servers []map[string]any `json:"servers"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("SIP008 订阅解析失败: %w", err)
	}
	nodes := make([]config.Node, 0, len(doc.Servers))
	for _, m := range doc.Servers {
		server := anyString(m["server"])
		port, err := parsePort(anyString(m["server_port"]))
		s := map[string]string{"method": anyString(m["method"]), "password": anyString(m["password"])}
		if server == "" || err != nil || s["method"] == "" || s["password"] == "" {
			continue
		}
		switch plugin := anyString(m["plugin"]); plugin {
		case "":
		case "obfs-local", "simple-obfs":
			s["plugin"] = "obfs-local"
		case "v2ray-plugin":
			s["plugin"] = plugin
		default:
			continue // shadow-tls 等插件不支持
		}
		if s["plugin"] != "" {
			if opts := anyString(m["plugin_opts"]); opts != "" {
				s["plugin_opts"] = opts
			}
		}
		nodes = append(nodes, config.Node{
			ID:       subscriptionNodeID("shadowsocks", server, port),
			Name:     firstNonEmpty(anyString(m["remarks"]), "ss-"+server),
			Protocol: "shadowsocks",
			Server:   server,
			Port:     port,
			Settings: s,
		})
	}
	return nodes, nil
}

// configLine 是拆开后的一行配置：位置参数和 key=value 参数。
type configLine struct {
	name, kind, server string
	port               uint16
	pos                []string
	kv                 map[string]string
}

func (l configLine) get(keys ...string) string {
	for _, key := range keys {
		if v := l.kv[key]; v != "" {
			return v
		}
	}
	return ""
}

func (l configLine) enabled(keys ...string) bool {
	v := strings.ToLower(l.get(keys...))
	return v == "true" || v == "1"
}

func (l configLine) arg(i int) string {
	if i < len(l.pos) {
		return l.pos[i]
	}
	return ""
}

var quantumultTypes = map[string]bool{"shadowsocks": true, "vmess": true, "vless": true, "trojan": true, "http": true, "socks5": true}

// splitConfigLine 解析 Surge / Loon / Quantumult X 的一行节点配置。
func splitConfigLine(line string) (configLine, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "[") || strings.Contains(line, "://") {
		return configLine{}, false
	}
	left, right, ok := strings.Cut(line, "=")
	if !ok {
		return configLine{}, false
	}
	left = strings.TrimSpace(left)
	parts := splitQuoted(right)
	if len(parts) == 0 {
		return configLine{}, false
	}
	l := configLine{kv: map[string]string{}}
	var rest []string
	if kind := strings.ToLower(left); quantumultTypes[kind] && strings.Contains(parts[0], ":") {
		// Quantumult X：vmess=example.com:443, method=..., tag=名称
		host, portText, err := net.SplitHostPort(parts[0])
		if err != nil {
			return configLine{}, false
		}
		l.kind, l.server, rest = "qx-"+kind, host, parts[1:]
		port, err := parsePort(portText)
		if err != nil {
			return configLine{}, false
		}
		l.port = port
	} else {
		// Surge / Loon：名称 = 类型, 地址, 端口, ...
		if len(parts) < 3 {
			return configLine{}, false
		}
		port, err := parsePort(parts[2])
		if err != nil || parts[1] == "" {
			return configLine{}, false
		}
		l.name, l.kind, l.server, l.port, rest = left, strings.ToLower(parts[0]), strings.Trim(parts[1], "[]"), port, parts[3:]
	}
	for _, part := range rest {
		if key, value, ok := strings.Cut(part, "="); ok && !strings.ContainsAny(key, " \"") {
			l.kv[strings.ToLower(strings.TrimSpace(key))] = unquote(strings.TrimSpace(value))
		} else {
			l.pos = append(l.pos, unquote(part))
		}
	}
	if strings.HasPrefix(l.kind, "qx-") {
		l.name = l.kv["tag"]
	}
	return l, true
}

// splitQuoted 按逗号拆分，双引号里的逗号不拆。
func splitQuoted(s string) []string {
	var parts []string
	var b strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			b.WriteRune(r)
		case r == ',' && !quoted:
			parts = append(parts, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if last := strings.TrimSpace(b.String()); last != "" || len(parts) > 0 {
		parts = append(parts, last)
	}
	return parts
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// parseConfigLine 把 Surge / Loon / Quantumult X 的节点配置行转换成节点；不认识的类型返回 false。
func parseConfigLine(line string) (config.Node, bool) {
	l, ok := splitConfigLine(line)
	if !ok {
		return config.Node{}, false
	}
	var protocol string
	var s map[string]string
	if strings.HasPrefix(l.kind, "qx-") {
		protocol, s, ok = quantumultNode(l)
	} else {
		protocol, s, ok = surgeNode(l)
	}
	if !ok {
		return config.Node{}, false
	}
	for key, value := range s {
		if value == "" {
			delete(s, key)
		}
	}
	if (protocol == "vmess" || protocol == "vless" || protocol == "socks" || protocol == "http") && s["tls"] != "true" {
		for _, key := range []string{"server_name", "insecure", "alpn", "utls_fingerprint"} {
			delete(s, key)
		}
	}
	name := l.name
	if name == "" {
		name = protocol + "-" + l.server
	}
	return config.Node{ID: subscriptionNodeID(protocol, l.server, l.port), Name: name, Protocol: protocol, Server: l.server, Port: l.port, Settings: s}, true
}

// surgeNode 处理 Surge 和 Loon 的写法。两者格式相同，区别是 Loon 把加密方式、密码等写成位置参数。
func surgeNode(l configLine) (string, map[string]string, bool) {
	s := map[string]string{}
	tlsCommon := func() {
		s["server_name"] = l.get("sni", "tls-name", "peer", "servername")
		if l.enabled("skip-cert-verify", "allow-insecure") {
			s["insecure"] = "true"
		}
		s["alpn"] = l.get("alpn")
		s["utls_fingerprint"] = l.get("client-fingerprint", "fingerprint-type")
	}
	// transport 返回 false 表示传输方式不支持。
	transport := func() bool {
		kind := strings.ToLower(l.get("transport"))
		if l.enabled("ws") {
			kind = "ws"
		}
		switch kind {
		case "", "tcp":
			return true
		case "ws", "http", "httpupgrade":
			s["transport_type"] = kind
			s["transport_path"] = l.get("ws-path", "path")
			s["transport_host"] = l.get("host")
			// Surge：ws-headers=Host:"example.com"|User-Agent:"..."
			for _, header := range strings.Split(l.get("ws-headers"), "|") {
				if key, value, ok := strings.Cut(header, ":"); ok && strings.EqualFold(strings.TrimSpace(key), "host") {
					s["transport_host"] = unquote(value)
				}
			}
		case "grpc":
			s["transport_type"] = "grpc"
			s["transport_service_name"] = l.get("grpc-service-name", "service-name", "path")
		default:
			return false
		}
		return true
	}
	switch l.kind {
	case "ss", "shadowsocks":
		s["method"] = firstNonEmpty(l.get("encrypt-method", "method"), l.arg(0))
		s["password"] = firstNonEmpty(l.get("password"), l.arg(1))
		if s["method"] == "" || s["password"] == "" {
			return "", nil, false
		}
		if obfs := strings.ToLower(l.get("obfs", "obfs-name")); obfs != "" && obfs != "none" {
			if obfs != "http" && obfs != "tls" {
				return "", nil, false
			}
			opts := []string{"obfs=" + obfs}
			if v := l.get("obfs-host"); v != "" {
				opts = append(opts, "obfs-host="+v)
			}
			if v := l.get("obfs-uri"); v != "" && obfs == "http" {
				opts = append(opts, "obfs-uri="+v)
			}
			s["plugin"], s["plugin_opts"] = "obfs-local", strings.Join(opts, ";")
		}
		if l.enabled("udp-over-tcp") {
			s["udp_over_tcp"] = "true"
		}
		return "shadowsocks", s, true
	case "vmess":
		// Surge：username=uuid；Loon：VMess,地址,端口,加密方式,"uuid"
		if l.get("username", "uuid") != "" {
			s["uuid"] = l.get("username", "uuid")
			s["security"] = firstNonEmpty(l.get("encrypt-method", "method"), l.arg(0), "auto")
		} else {
			s["uuid"] = firstNonEmpty(l.arg(1), l.arg(0))
			if len(l.pos) >= 2 {
				s["security"] = l.arg(0)
			}
		}
		s["security"] = vmessSecurity(firstNonEmpty(s["security"], "auto"))
		s["alter_id"] = firstNonEmpty(l.get("alterid", "alter-id"), "0")
		if l.enabled("tls", "over-tls") {
			s["tls"] = "true"
		}
		tlsCommon()
		if s["uuid"] == "" || !transport() {
			return "", nil, false
		}
		return "vmess", s, true
	case "vless":
		s["uuid"] = firstNonEmpty(l.get("uuid", "username"), l.arg(0))
		s["flow"] = l.get("flow")
		if l.enabled("tls", "over-tls") {
			s["tls"] = "true"
		}
		if pbk := l.get("public-key"); pbk != "" {
			s["tls"], s["reality_public_key"], s["reality_short_id"] = "true", pbk, l.get("short-id")
		}
		tlsCommon()
		if s["uuid"] == "" || !transport() {
			return "", nil, false
		}
		return "vless", s, true
	case "trojan":
		s["password"] = firstNonEmpty(l.get("password"), l.arg(0))
		s["tls"] = "true"
		tlsCommon()
		if s["password"] == "" || !transport() {
			return "", nil, false
		}
		return "trojan", s, true
	case "hysteria2":
		s["password"] = firstNonEmpty(l.get("password"), l.arg(0))
		tlsCommon()
		s["sni"] = s["server_name"]
		delete(s, "server_name")
		delete(s, "utls_fingerprint")
		s["up_mbps"] = bandwidthMbps(l.get("upload-bandwidth", "up"))
		s["down_mbps"] = bandwidthMbps(l.get("download-bandwidth", "down"))
		s["server_ports"] = l.get("port-hopping")
		if v := l.get("port-hopping-interval"); v != "" {
			s["hop_interval"] = strings.TrimSuffix(v, "s") + "s"
		}
		if v := l.get("salamander-password", "obfs-password"); v != "" {
			s["obfs_type"], s["obfs_password"] = "salamander", v
		}
		if s["password"] == "" {
			return "", nil, false
		}
		return "hysteria2", s, true
	case "tuic", "tuic-v5":
		s["uuid"], s["password"] = l.get("uuid"), l.get("password")
		tlsCommon()
		delete(s, "utls_fingerprint")
		s["congestion_control"] = l.get("congestion-controller", "congestion-control")
		if s["uuid"] == "" || s["password"] == "" {
			return "", nil, false // TUIC v4 的 token 写法 sing-box 不支持
		}
		return "tuic", s, true
	case "anytls":
		s["password"] = firstNonEmpty(l.get("password"), l.arg(0))
		tlsCommon()
		if s["password"] == "" {
			return "", nil, false
		}
		return "anytls", s, true
	case "socks5", "socks5-tls", "http", "https":
		protocol := "http"
		if strings.HasPrefix(l.kind, "socks5") {
			protocol = "socks"
			s["version"] = "5"
		}
		s["username"] = firstNonEmpty(l.get("username"), l.arg(0))
		s["password"] = firstNonEmpty(l.get("password"), l.arg(1))
		if l.kind == "socks5-tls" || l.kind == "https" || l.enabled("over-tls", "tls") {
			if protocol == "socks" {
				return "", nil, false // sing-box 的 SOCKS 出站不支持 TLS
			}
			s["tls"] = "true"
		}
		tlsCommon()
		return protocol, s, true
	}
	return "", nil, false
}

// quantumultNode 处理 Quantumult X 的写法。
func quantumultNode(l configLine) (string, map[string]string, bool) {
	s := map[string]string{}
	obfs := strings.ToLower(l.get("obfs"))
	tlsCommon := func() {
		s["server_name"] = l.get("tls-host", "obfs-host")
		if strings.EqualFold(l.get("tls-verification"), "false") {
			s["insecure"] = "true"
		}
		s["alpn"] = l.get("tls-alpn")
	}
	// transport 处理 vmess/vless/trojan 的 obfs：ws、wss、over-tls。
	transport := func() bool {
		switch obfs {
		case "", "none":
		case "over-tls":
			s["tls"] = "true"
		case "ws", "wss":
			s["transport_type"] = "ws"
			s["transport_host"] = l.get("obfs-host")
			s["transport_path"] = l.get("obfs-uri")
			if obfs == "wss" {
				s["tls"] = "true"
			}
		default:
			return false // vmess 的 http 伪装 sing-box 不支持
		}
		if l.enabled("over-tls") {
			s["tls"] = "true"
		}
		tlsCommon()
		return true
	}
	switch l.kind {
	case "qx-shadowsocks":
		s["method"], s["password"] = l.get("method"), l.get("password")
		if s["method"] == "" || s["password"] == "" {
			return "", nil, false
		}
		switch obfs {
		case "", "none":
		case "http", "tls":
			opts := []string{"obfs=" + obfs}
			if v := l.get("obfs-host"); v != "" {
				opts = append(opts, "obfs-host="+v)
			}
			if v := l.get("obfs-uri"); v != "" && obfs == "http" {
				opts = append(opts, "obfs-uri="+v)
			}
			s["plugin"], s["plugin_opts"] = "obfs-local", strings.Join(opts, ";")
		case "ws", "wss":
			opts := []string{"mode=websocket"}
			if obfs == "wss" {
				opts = append(opts, "tls")
			}
			if v := l.get("obfs-host"); v != "" {
				opts = append(opts, "host="+v)
			}
			if v := l.get("obfs-uri"); v != "" {
				opts = append(opts, "path="+v)
			}
			s["plugin"], s["plugin_opts"] = "v2ray-plugin", strings.Join(opts, ";")
		default:
			return "", nil, false
		}
		return "shadowsocks", s, true
	case "qx-vmess":
		s["uuid"] = l.get("password")
		s["security"] = vmessSecurity(firstNonEmpty(l.get("method"), "auto"))
		s["alter_id"] = "0"
		if s["uuid"] == "" || !transport() {
			return "", nil, false
		}
		return "vmess", s, true
	case "qx-vless":
		s["uuid"] = l.get("password")
		s["flow"] = l.get("vless-flow")
		if !transport() || s["uuid"] == "" {
			return "", nil, false
		}
		if pbk := l.get("reality-base64-pubkey"); pbk != "" {
			s["tls"], s["reality_public_key"], s["reality_short_id"] = "true", pbk, l.get("reality-hex-shortid")
		}
		return "vless", s, true
	case "qx-trojan":
		s["password"] = l.get("password")
		if s["password"] == "" || !transport() {
			return "", nil, false
		}
		// Trojan 默认 TLS，只有明文 ws 才不开。
		if obfs != "ws" || l.enabled("over-tls") {
			s["tls"] = "true"
		}
		return "trojan", s, true
	case "qx-http", "qx-socks5":
		protocol := "http"
		if l.kind == "qx-socks5" {
			protocol = "socks"
			s["version"] = "5"
		}
		s["username"], s["password"] = l.get("username"), l.get("password")
		if l.enabled("over-tls") {
			if protocol == "socks" {
				return "", nil, false // sing-box 的 SOCKS 出站不支持 TLS
			}
			s["tls"] = "true"
		}
		tlsCommon()
		return protocol, s, true
	}
	return "", nil, false
}

// vmessSecurity 把 Surge/QX 的加密方式名换成 sing-box 认识的名字。
func vmessSecurity(method string) string {
	switch strings.ToLower(method) {
	case "chacha20-ietf-poly1305", "chacha20-poly1305":
		return "chacha20-poly1305"
	}
	return strings.ToLower(method)
}
