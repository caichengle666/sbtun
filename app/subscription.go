package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/caichengle666/sbtun/config"
)

func parseSubscription(link string) ([]config.Node, error) {
	link = strings.TrimSpace(link)
	if link == "" {
		return nil, fmt.Errorf("订阅链接不能为空")
	}
	if strings.HasPrefix(link, "vmess://") {
		node, err := parseVMess(link)
		if err != nil {
			return nil, err
		}
		return []config.Node{node}, nil
	}
	if strings.HasPrefix(link, "vless://") {
		node, err := parseVLess(link)
		if err != nil {
			return nil, err
		}
		return []config.Node{node}, nil
	}
	if strings.HasPrefix(link, "trojan://") {
		node, err := parseTrojan(link)
		if err != nil {
			return nil, err
		}
		return []config.Node{node}, nil
	}
	if strings.HasPrefix(link, "ss://") {
		node, err := parseShadowsocks(link)
		if err != nil {
			return nil, err
		}
		return []config.Node{node}, nil
	}
	if strings.HasPrefix(link, "hysteria2://") {
		node, err := parseHysteria2(link)
		if err != nil {
			return nil, err
		}
		return []config.Node{node}, nil
	}
	if strings.HasPrefix(link, "socks://") || strings.HasPrefix(link, "socks5://") || strings.HasPrefix(link, "socks5h://") {
		node, err := parseHTTPStyle(link)
		if err != nil {
			return nil, err
		}
		return []config.Node{node}, nil
	}
	if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
		parsed, err := url.Parse(link)
		isSubscription := err == nil && (parsed.Port() == "" || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "")
		if !isSubscription {
			node, err := parseHTTPStyle(link)
			if err != nil {
				return nil, err
			}
			return []config.Node{node}, nil
		}
		// 普通 HTTP/HTTPS 地址没有代理端口时，按订阅地址下载并拆分节点。
		return parseSubscriptionBody(link)
	}
	return parseSubscriptionBody(link)
}

func parseSubscriptionBody(link string) ([]config.Node, error) {
	body, err := fetchURL(link)
	if err != nil {
		return nil, fmt.Errorf("下载订阅失败: %w", err)
	}
	text := strings.TrimSpace(body)
	if text == "" {
		return nil, fmt.Errorf("订阅内容为空")
	}
	decoded, err := tryBase64Decode(text)
	if err == nil && decoded != "" {
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
	case strings.HasPrefix(line, "socks://"), strings.HasPrefix(line, "socks5://"), strings.HasPrefix(line, "socks5h://"), strings.HasPrefix(line, "http://"), strings.HasPrefix(line, "https://"):
		return parseHTTPStyle(line)
	default:
		return config.Node{}, fmt.Errorf("不支持的协议前缀: %s", line)
	}
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

type vmessConfig struct {
	V    string `json:"v"`
	Ps   string `json:"ps"`
	Add  string `json:"add"`
	Port string `json:"port"`
	ID   string `json:"id"`
	Aid  string `json:"aid"`
	Net  string `json:"net"`
	Type string `json:"type"`
	Scy  string `json:"scy"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
	ALPN string `json:"alpn"`
	FP   string `json:"fp"`
}

func parseVMess(link string) (config.Node, error) {
	payload := strings.TrimPrefix(link, "vmess://")
	decoded, err := tryBase64Decode(payload)
	if err != nil {
		return config.Node{}, fmt.Errorf("vmess base64 解码失败: %w", err)
	}
	var cfg vmessConfig
	if err := json.Unmarshal([]byte(decoded), &cfg); err != nil {
		return config.Node{}, fmt.Errorf("vmess JSON 解析失败: %w", err)
	}
	port, err := parsePort(cfg.Port)
	if err != nil {
		return config.Node{}, fmt.Errorf("vmess 端口无效: %s", cfg.Port)
	}
	settings := map[string]string{"uuid": cfg.ID}
	if cfg.Aid != "" {
		settings["alter_id"] = cfg.Aid
	}
	security := cfg.Scy
	if security == "" {
		security = cfg.Type
	}
	if security != "" {
		settings["security"] = security
	}
	setLinkTransport(settings, cfg.Net)
	if cfg.Host != "" {
		settings["transport_host"] = cfg.Host
	}
	if cfg.Path != "" {
		settings["transport_path"] = cfg.Path
	}
	if strings.EqualFold(cfg.TLS, "tls") {
		settings["tls"] = "true"
	}
	if cfg.SNI != "" {
		settings["server_name"] = cfg.SNI
	}
	if cfg.ALPN != "" {
		settings["alpn"] = cfg.ALPN
	}
	if cfg.FP != "" {
		settings["utls_fingerprint"] = cfg.FP
	}
	return config.Node{
		ID:       "vmess-" + cfg.Add + "-" + cfg.Port,
		Name:     cfg.Ps,
		Protocol: "vmess",
		Server:   cfg.Add,
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
	if vals.Get("sni") != "" {
		settings["server_name"] = vals.Get("sni")
	}
	if vals.Get("security") == "tls" || vals.Get("security") == "reality" {
		settings["tls"] = "true"
	}
	if vals.Get("alpn") != "" {
		settings["alpn"] = vals.Get("alpn")
	}
	if vals.Get("insecure") == "1" || strings.EqualFold(vals.Get("insecure"), "true") {
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
	if vals.Get("sni") != "" {
		settings["server_name"] = vals.Get("sni")
	}
	if vals.Get("security") == "tls" {
		settings["tls"] = "true"
	}
	if vals.Get("alpn") != "" {
		settings["alpn"] = vals.Get("alpn")
	}
	if vals.Get("insecure") == "1" || strings.EqualFold(vals.Get("insecure"), "true") {
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
	decoded, err := tryBase64Decode(payload)
	if err != nil {
		decoded = payload
	}
	atIdx := strings.LastIndex(decoded, "@")
	if atIdx < 0 {
		return config.Node{}, fmt.Errorf("ss 格式错误: 缺少 @")
	}
	userPart := decoded[:atIdx]
	rest := decoded[atIdx+1:]
	hostIdx := strings.LastIndex(rest, ":")
	if hostIdx < 0 {
		return config.Node{}, fmt.Errorf("ss 格式错误: 缺少端口")
	}
	server := rest[:hostIdx]
	portStr := rest[hostIdx+1:]
	if idx := strings.Index(portStr, "#"); idx >= 0 {
		portStr = portStr[:idx]
	}
	port, err := parsePort(portStr)
	if err != nil {
		return config.Node{}, fmt.Errorf("ss 端口无效: %s", portStr)
	}
	colonIdx := strings.LastIndex(userPart, ":")
	method := userPart[:colonIdx]
	password := userPart[colonIdx+1:]
	return config.Node{
		ID:       "ss-" + server + "-" + portStr,
		Name:     "ss-" + server,
		Protocol: "shadowsocks",
		Server:   server,
		Port:     port,
		Settings: map[string]string{"method": method, "password": password},
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
	if v := vals.Get("insecure"); v == "1" || strings.EqualFold(v, "true") {
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
	if strings.HasPrefix(link, "socks://") || strings.HasPrefix(link, "socks5://") || strings.HasPrefix(link, "socks5h://") {
		protocol = "socks"
	}
	settings := map[string]string{}
	if u.User != nil {
		settings["username"] = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			settings["password"] = pw
		}
	}
	vals := u.Query()
	if vals.Get("tls") == "1" || strings.EqualFold(vals.Get("tls"), "true") {
		settings["tls"] = "true"
	}
	if vals.Get("sni") != "" {
		settings["server_name"] = vals.Get("sni")
	}
	if vals.Get("insecure") == "1" || strings.EqualFold(vals.Get("insecure"), "true") {
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

func fetchURL(link string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "sbtun/0.1.1")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("订阅 HTTP %d", resp.StatusCode)
	}
	buf := new(strings.Builder)
	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
