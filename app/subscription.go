package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/caichengle666/sbtun/config"
)

func parseSubscription(link string) ([]config.Node, error) {
	link = normalizeNodeLink(link)
	if err := ValidateNodeSource(link); err != nil {
		return nil, err
	}
	if isSubscriptionURL(link) {
		return parseSubscriptionBody(link)
	}
	node, err := parseSubscriptionLine(link)
	if err != nil {
		return nil, err
	}
	return []config.Node{node}, nil
}

// isSubscriptionURL reports whether an http(s) link is a remote subscription
// rather than an HTTP proxy node: proxy nodes carry an explicit port and no
// path or query.
func isSubscriptionURL(link string) bool {
	link = normalizeNodeLink(link)
	if !hasScheme(link, "http", "https") {
		return false
	}
	parsed, err := url.Parse(link)
	return err == nil && (parsed.Port() == "" || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "")
}

// ValidateNodeSource checks whether input can be parsed without performing network access.
func ValidateNodeSource(link string) error {
	link = normalizeNodeLink(link)
	if link == "" {
		return fmt.Errorf("节点链接不能为空")
	}
	if hasScheme(link, "vmess", "vless", "trojan", "ss", "hysteria2", "tuic", "anytls", "wireguard", "socks", "socks4", "socks4a", "socks5", "socks5h", "http", "https") {
		return nil
	}
	return fmt.Errorf("无效节点链接: 支持 vmess、vless、trojan、ss、hy2/hysteria2、tuic、anytls、wireguard/wg、socks、http 或 https")
}

func parseSubscriptionBody(link string) ([]config.Node, error) {
	body, err := fetchURL(link)
	if err != nil {
		return nil, fmt.Errorf("下载订阅失败: %w", err)
	}
	text := strings.TrimSpace(strings.TrimPrefix(body, "\ufeff"))
	if text == "" {
		return nil, fmt.Errorf("订阅内容为空")
	}
	switch {
	case strings.HasPrefix(text, "{"):
		return parseSingBoxSubscription(text)
	case looksLikeClashYAML(text):
		return parseClashSubscription(text)
	}
	if decoded, err := tryBase64Decode(text); err == nil && decoded != "" {
		text = decoded
	}
	var nodes []config.Node
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		node, err := parseSubscriptionLine(line)
		if err != nil {
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func parseSubscriptionLine(line string) (config.Node, error) {
	line = normalizeNodeLink(line)
	switch {
	case strings.HasPrefix(line, "vmess://"):
		return parseVMess(line)
	case strings.HasPrefix(line, "vless://"):
		return parseVLess(line)
	case strings.HasPrefix(line, "trojan://"):
		return parseTrojan(line)
	case strings.HasPrefix(line, "ss://"):
		return parseShadowsocks(line)
	case strings.HasPrefix(line, "hysteria2://"):
		return parseHysteria2(line)
	case strings.HasPrefix(line, "tuic://"):
		return parseTUIC(line)
	case strings.HasPrefix(line, "anytls://"):
		return parseAnyTLS(line)
	case strings.HasPrefix(line, "wireguard://"):
		return parseWireGuard(line)
	case hasScheme(line, "socks", "socks4", "socks4a", "socks5", "socks5h", "http", "https"):
		return parseHTTPStyle(line)
	default:
		return config.Node{}, fmt.Errorf("不支持的协议前缀: %s", line)
	}
}

func normalizeNodeLink(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	separator := strings.Index(link, "://")
	if separator <= 0 {
		return link
	}
	scheme := strings.ToLower(link[:separator])
	switch scheme {
	case "hy2":
		scheme = "hysteria2"
	case "wg":
		scheme = "wireguard"
	}
	return scheme + link[separator:]
}

func hasScheme(link string, schemes ...string) bool {
	separator := strings.Index(link, "://")
	if separator <= 0 {
		return false
	}
	scheme := strings.ToLower(link[:separator])
	for _, candidate := range schemes {
		if scheme == candidate {
			return true
		}
	}
	return false
}

func tryBase64Decode(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// vmessConfig 是 v2rayN 的 vmess 分享格式。port、aid 等字段在不同客户端里
// 可能是字符串也可能是数字，所以用 looseString 接收。
type vmessConfig struct {
	V             looseString `json:"v"`
	Ps            looseString `json:"ps"`
	Add           looseString `json:"add"`
	Port          looseString `json:"port"`
	ID            looseString `json:"id"`
	Aid           looseString `json:"aid"`
	Net           looseString `json:"net"`
	Type          looseString `json:"type"` // 伪装类型，导入时不使用
	Scy           looseString `json:"scy"`
	Host          looseString `json:"host"`
	Path          looseString `json:"path"`
	TLS           looseString `json:"tls"`
	SNI           looseString `json:"sni"`
	ALPN          looseString `json:"alpn"`
	FP            looseString `json:"fp"`
	AllowInsecure looseString `json:"allowInsecure"`
}

type looseString string

func (s *looseString) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*s = looseString(strings.TrimSpace(anyString(v)))
	return nil
}

func parseVMess(link string) (config.Node, error) {
	raw := strings.TrimPrefix(link, "vmess://")
	if idx := strings.IndexByte(raw, '#'); idx >= 0 {
		raw = raw[:idx]
	}
	decoded, err := tryBase64Decode(raw)
	if err != nil {
		return config.Node{}, fmt.Errorf("vmess base64 解码失败: %w", err)
	}
	var cfg vmessConfig
	if err := json.Unmarshal([]byte(decoded), &cfg); err != nil {
		return config.Node{}, fmt.Errorf("vmess JSON 解析失败: %w", err)
	}
	port, err := parsePort(string(cfg.Port))
	if err != nil {
		return config.Node{}, fmt.Errorf("vmess 端口无效: %s", cfg.Port)
	}
	settings := map[string]string{"uuid": string(cfg.ID)}
	set := func(key string, v looseString) {
		if v != "" {
			settings[key] = string(v)
		}
	}
	set("alter_id", cfg.Aid)
	// "type" 是伪装类型（none/http），不是加密方式，不能拿来当 security。
	set("security", cfg.Scy)
	setLinkTransport(settings, string(cfg.Net))
	set("transport_host", cfg.Host)
	if settings["transport_type"] == "grpc" {
		set("transport_service_name", cfg.Path)
	} else {
		set("transport_path", cfg.Path)
	}
	if strings.EqualFold(string(cfg.TLS), "tls") {
		settings["tls"] = "true"
	}
	set("server_name", cfg.SNI)
	set("alpn", cfg.ALPN)
	set("utls_fingerprint", cfg.FP)
	if cfg.AllowInsecure == "1" || strings.EqualFold(string(cfg.AllowInsecure), "true") {
		settings["insecure"] = "true"
	}
	return config.Node{
		ID:       "vmess-" + string(cfg.Add) + "-" + string(cfg.Port),
		Name:     string(cfg.Ps),
		Protocol: "vmess",
		Server:   string(cfg.Add),
		Port:     port,
		Settings: settings,
	}, nil
}

func parseVLess(link string) (config.Node, error) {
	u, err := url.Parse(link)
	if err != nil {
		return config.Node{}, fmt.Errorf("vless 解析失败: %w", err)
	}
	if u.Port() == "" {
		return config.Node{}, fmt.Errorf("vless 缺少端口")
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return config.Node{}, fmt.Errorf("vless 端口无效: %w", err)
	}
	settings := map[string]string{"uuid": u.User.Username()}
	vals := u.Query()
	setLinkTransport(settings, vals.Get("type"))
	if vals.Get("host") != "" {
		settings["transport_host"] = vals.Get("host")
	}
	if vals.Get("path") != "" {
		settings["transport_path"] = vals.Get("path")
	}
	if vals.Get("serviceName") != "" {
		settings["transport_service_name"] = vals.Get("serviceName")
	}
	if vals.Get("flow") != "" {
		settings["flow"] = vals.Get("flow")
	}
	if vals.Get("packetEncoding") != "" {
		settings["packet_encoding"] = vals.Get("packetEncoding")
	}
	if vals.Get("sni") != "" {
		settings["server_name"] = vals.Get("sni")
	}
	if vals.Get("security") == "tls" || vals.Get("security") == "reality" {
		settings["tls"] = "true"
	}
	if vals.Get("alpn") != "" {
		settings["alpn"] = vals.Get("alpn")
	}
	if queryEnabled(vals, "insecure", "allowInsecure", "allow_insecure") {
		settings["insecure"] = "true"
	}
	if vals.Get("pbk") != "" {
		settings["reality_public_key"] = vals.Get("pbk")
	}
	if vals.Get("sid") != "" {
		settings["reality_short_id"] = vals.Get("sid")
	}
	if vals.Get("fp") != "" {
		settings["utls_fingerprint"] = vals.Get("fp")
	}
	name := u.Fragment
	if name == "" {
		name = "vless-" + u.Hostname()
	}
	return config.Node{
		ID:       "vless-" + u.Host,
		Name:     name,
		Protocol: "vless",
		Server:   u.Hostname(),
		Port:     port,
		Settings: settings,
	}, nil
}

func parseTrojan(link string) (config.Node, error) {
	u, err := url.Parse(link)
	if err != nil {
		return config.Node{}, fmt.Errorf("trojan 解析失败: %w", err)
	}
	if u.Port() == "" {
		return config.Node{}, fmt.Errorf("trojan 缺少端口")
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return config.Node{}, fmt.Errorf("trojan 端口无效: %w", err)
	}
	password, _ := u.User.Password()
	if password == "" {
		password = u.User.Username()
	}
	settings := map[string]string{"password": password}
	vals := u.Query()
	setLinkTransport(settings, vals.Get("type"))
	if vals.Get("host") != "" {
		settings["transport_host"] = vals.Get("host")
	}
	if vals.Get("path") != "" {
		settings["transport_path"] = vals.Get("path")
	}
	if vals.Get("serviceName") != "" {
		settings["transport_service_name"] = vals.Get("serviceName")
	}
	if vals.Get("sni") != "" {
		settings["server_name"] = vals.Get("sni")
	}
	// Trojan 默认就是 TLS，链接里通常不写 security；只有明确写 none 才关闭。
	if !strings.EqualFold(vals.Get("security"), "none") {
		settings["tls"] = "true"
	}
	if vals.Get("fp") != "" {
		settings["utls_fingerprint"] = vals.Get("fp")
	}
	if vals.Get("alpn") != "" {
		settings["alpn"] = vals.Get("alpn")
	}
	if queryEnabled(vals, "insecure", "allowInsecure", "allow_insecure") {
		settings["insecure"] = "true"
	}
	name := u.Fragment
	if name == "" {
		name = "trojan-" + u.Hostname()
	}
	return config.Node{
		ID:       "trojan-" + u.Host,
		Name:     name,
		Protocol: "trojan",
		Server:   u.Hostname(),
		Port:     port,
		Settings: settings,
	}, nil
}

func parseShadowsocks(link string) (config.Node, error) {
	payload := strings.TrimPrefix(link, "ss://")
	name := ""
	if raw, fragment, ok := strings.Cut(payload, "#"); ok {
		payload = raw
		name, _ = url.PathUnescape(fragment)
	}
	if !strings.Contains(payload, "@") {
		decoded, err := tryBase64Decode(payload)
		if err != nil {
			return config.Node{}, fmt.Errorf("ss base64 解码失败: %w", err)
		}
		payload = decoded
	}
	atIdx := strings.LastIndex(payload, "@")
	if atIdx < 0 {
		return config.Node{}, fmt.Errorf("ss 格式错误: 缺少 @")
	}
	userPart := payload[:atIdx]
	address, query, _ := strings.Cut(payload[atIdx+1:], "?")
	address = strings.TrimSuffix(address, "/")
	if decoded, err := tryBase64Decode(userPart); err == nil && strings.Contains(decoded, ":") {
		userPart = decoded
	}
	method, password, ok := strings.Cut(userPart, ":")
	if !ok || method == "" || password == "" {
		return config.Node{}, fmt.Errorf("ss 格式错误: 缺少加密方式或密码")
	}
	server, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return config.Node{}, fmt.Errorf("ss 地址无效: %w", err)
	}
	port, err := parsePort(portStr)
	if err != nil {
		return config.Node{}, fmt.Errorf("ss 端口无效: %s", portStr)
	}
	settings := map[string]string{"method": method, "password": password}
	// SIP002：?plugin=obfs-local;obfs=http;obfs-host=example.com
	if vals, err := url.ParseQuery(query); err == nil && vals.Get("plugin") != "" {
		plugin, opts, _ := strings.Cut(vals.Get("plugin"), ";")
		switch plugin {
		case "obfs-local", "simple-obfs":
			plugin = "obfs-local"
		case "v2ray-plugin":
		default:
			return config.Node{}, fmt.Errorf("ss 插件暂不支持: %s", plugin)
		}
		settings["plugin"] = plugin
		if opts != "" {
			settings["plugin_opts"] = opts
		}
	}
	if name == "" {
		name = "ss-" + server
	}
	return config.Node{
		ID:       "ss-" + server + "-" + portStr,
		Name:     name,
		Protocol: "shadowsocks",
		Server:   server,
		Port:     port,
		Settings: settings,
	}, nil
}

func parseHysteria2(link string) (config.Node, error) {
	u, err := url.Parse(link)
	if err != nil {
		return config.Node{}, fmt.Errorf("hysteria2 解析失败: %w", err)
	}
	if u.Port() == "" {
		return config.Node{}, fmt.Errorf("hysteria2 缺少端口")
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return config.Node{}, fmt.Errorf("hysteria2 端口无效: %w", err)
	}
	password := u.User.Username()
	if _, hasPass := u.User.Password(); hasPass {
		password, _ = u.User.Password()
	}
	settings := map[string]string{"password": password}
	vals := u.Query()
	if v := vals.Get("sni"); v != "" {
		settings["sni"] = v
	}
	if queryEnabled(vals, "insecure", "allowInsecure", "allow_insecure") {
		settings["insecure"] = "true"
	}
	if v := vals.Get("up_mbps"); v != "" {
		settings["up_mbps"] = v
	}
	if v := vals.Get("down_mbps"); v != "" {
		settings["down_mbps"] = v
	}
	for _, key := range []string{"server_ports", "mport", "hop_interval", "alpn", "obfs", "obfs-password"} {
		if v := vals.Get(key); v != "" {
			name := strings.ReplaceAll(key, "-", "_")
			if key == "mport" {
				name = "server_ports"
			}
			if key == "obfs" {
				name = "obfs_type"
			}
			settings[name] = v
		}
	}
	name := u.Fragment
	if name == "" {
		name = "hysteria2-" + u.Hostname()
	}
	return config.Node{
		ID:       "hysteria2-" + u.Host,
		Name:     name,
		Protocol: "hysteria2",
		Server:   u.Hostname(),
		Port:     port,
		Settings: settings,
	}, nil
}

func parseHTTPStyle(link string) (config.Node, error) {
	u, err := url.Parse(link)
	if err != nil {
		return config.Node{}, fmt.Errorf("代理链接解析失败: %w", err)
	}
	if u.Port() == "" {
		return config.Node{}, fmt.Errorf("代理链接缺少端口")
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return config.Node{}, fmt.Errorf("代理端口无效: %w", err)
	}
	protocol := "http"
	if hasScheme(link, "socks", "socks4", "socks4a", "socks5", "socks5h") {
		protocol = "socks"
	}
	settings := map[string]string{}
	if protocol == "socks" {
		scheme := strings.ToLower(u.Scheme)
		version := map[string]string{"socks4": "4", "socks4a": "4a", "socks5": "5", "socks5h": "5"}[scheme]
		if version == "" {
			version = u.Query().Get("version")
		}
		if version == "" {
			version = "5"
		}
		if version != "4" && version != "4a" && version != "5" {
			return config.Node{}, fmt.Errorf("SOCKS 版本无效: %s", version)
		}
		settings["version"] = version
	}
	if u.User != nil {
		username, password := u.User.Username(), ""
		if pw, ok := u.User.Password(); ok {
			password = pw
		} else if decoded, err := tryBase64Decode(username); err == nil && strings.Contains(decoded, ":") {
			// v2rayN 的 socks 链接把 user:pass 整体 base64 编码。
			username, password, _ = strings.Cut(decoded, ":")
		}
		if username != "" {
			settings["username"] = username
		}
		if password != "" {
			settings["password"] = password
		}
	}
	vals := u.Query()
	if vals.Get("tls") == "1" || strings.EqualFold(vals.Get("tls"), "true") {
		settings["tls"] = "true"
	}
	if vals.Get("sni") != "" {
		settings["server_name"] = vals.Get("sni")
	}
	if queryEnabled(vals, "insecure", "allowInsecure", "allow_insecure") {
		settings["insecure"] = "true"
	}
	if vals.Get("path") != "" {
		settings["path"] = vals.Get("path")
	}
	name := u.Fragment
	if name == "" {
		name = protocol + "-" + u.Hostname()
	}
	return config.Node{
		ID:       protocol + "-" + u.Host,
		Name:     name,
		Protocol: protocol,
		Server:   u.Hostname(),
		Port:     port,
		Settings: settings,
	}, nil
}

func parsePort(s string) (uint16, error) {
	var p uint16
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &p)
	if err != nil || p == 0 {
		return 0, fmt.Errorf("无效端口: %s", s)
	}
	return p, nil
}

func setLinkTransport(settings map[string]string, value string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "tcp", "udp":
		settings["network"] = strings.ToLower(strings.TrimSpace(value))
	case "ws", "http", "grpc", "httpupgrade":
		settings["transport_type"] = strings.ToLower(strings.TrimSpace(value))
	}
}

// maxSubscriptionBytes caps a subscription download so a broken or hostile
// server cannot exhaust memory. Real subscriptions are far below this.
const maxSubscriptionBytes = 8 << 20

func fetchURL(link string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "sbtun/"+Version())
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("订阅 HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxSubscriptionBytes {
		return "", fmt.Errorf("订阅内容过大: %d 字节，上限 %d 字节", resp.ContentLength, maxSubscriptionBytes)
	}
	buf := new(strings.Builder)
	n, err := io.Copy(buf, io.LimitReader(resp.Body, maxSubscriptionBytes+1))
	if err != nil {
		return "", err
	}
	if n > maxSubscriptionBytes {
		return "", fmt.Errorf("订阅内容超过 %d 字节上限", maxSubscriptionBytes)
	}
	return buf.String(), nil
}
