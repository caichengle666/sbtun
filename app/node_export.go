package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/caichengle666/sbtun/config"
)

func (a *App) ExportNodes(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("请选择需要导出的节点")
	}
	cfg, err := a.manager.Load()
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]config.Node, len(cfg.Nodes))
	for _, node := range cfg.Nodes {
		nodes[node.ID] = node
	}
	links := make([]string, 0, len(ids))
	for _, id := range ids {
		node, ok := nodes[id]
		if !ok {
			return nil, fmt.Errorf("节点不存在: %s", id)
		}
		link, err := nodeShareLink(node)
		if err != nil {
			return nil, fmt.Errorf("导出节点 %s 失败: %w", node.Name, err)
		}
		links = append(links, link)
	}
	return links, nil
}

func nodeShareLink(node config.Node) (string, error) {
	if node.Server == "" || node.Port == 0 {
		return "", fmt.Errorf("节点地址不完整")
	}
	switch node.Protocol {
	case "vmess":
		return vmessShareLink(node)
	case "vless":
		return vlessShareLink(node), nil
	case "trojan":
		return trojanShareLink(node), nil
	case "shadowsocks":
		method := node.Settings["method"]
		password := node.Settings["password"]
		if method == "" || password == "" {
			return "", fmt.Errorf("缺少加密方式或密码")
		}
		credentials := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + password))
		return "ss://" + credentials + "@" + nodeAddress(node) + "#" + url.PathEscape(node.Name), nil
	case "hysteria2":
		return hysteria2ShareLink(node), nil
	case "socks", "http":
		return httpStyleShareLink(node), nil
	default:
		return "", fmt.Errorf("不支持的协议: %s", node.Protocol)
	}
}

func vmessShareLink(node config.Node) (string, error) {
	transport := node.Settings["transport_type"]
	if transport == "" {
		transport = node.Settings["network"]
	}
	if transport == "" {
		transport = "tcp"
	}
	security := node.Settings["security"]
	if security == "" {
		security = "auto"
	}
	cfg := vmessConfig{
		V: "2", Ps: node.Name, Add: node.Server, Port: strconv.Itoa(int(node.Port)),
		ID: node.Settings["uuid"], Aid: node.Settings["alter_id"], Net: transport,
		Type: "none", Scy: security, Host: node.Settings["transport_host"],
		Path: node.Settings["transport_path"], SNI: node.Settings["server_name"],
		ALPN: node.Settings["alpn"], FP: node.Settings["utls_fingerprint"],
	}
	if settingEnabled(node.Settings, "tls") {
		cfg.TLS = "tls"
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(data), nil
}

func vlessShareLink(node config.Node) string {
	u := &url.URL{Scheme: "vless", User: url.User(node.Settings["uuid"]), Host: nodeAddress(node), Fragment: node.Name}
	q := url.Values{"encryption": []string{"none"}}
	setTransportQuery(q, node.Settings)
	setQuery(q, "flow", node.Settings["flow"])
	setQuery(q, "sni", node.Settings["server_name"])
	setQuery(q, "alpn", node.Settings["alpn"])
	setQuery(q, "pbk", node.Settings["reality_public_key"])
	setQuery(q, "sid", node.Settings["reality_short_id"])
	setQuery(q, "fp", node.Settings["utls_fingerprint"])
	if node.Settings["reality_public_key"] != "" {
		q.Set("security", "reality")
	} else if settingEnabled(node.Settings, "tls") {
		q.Set("security", "tls")
	}
	if settingEnabled(node.Settings, "insecure") {
		q.Set("insecure", "1")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func trojanShareLink(node config.Node) string {
	u := &url.URL{Scheme: "trojan", User: url.User(node.Settings["password"]), Host: nodeAddress(node), Fragment: node.Name}
	q := url.Values{}
	setTransportQuery(q, node.Settings)
	setQuery(q, "sni", node.Settings["server_name"])
	setQuery(q, "alpn", node.Settings["alpn"])
	if settingEnabled(node.Settings, "tls") {
		q.Set("security", "tls")
	}
	if settingEnabled(node.Settings, "insecure") {
		q.Set("insecure", "1")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func hysteria2ShareLink(node config.Node) string {
	u := &url.URL{Scheme: "hysteria2", User: url.User(node.Settings["password"]), Host: nodeAddress(node), Fragment: node.Name}
	q := url.Values{}
	for key, target := range map[string]string{
		"sni": "sni", "alpn": "alpn", "up_mbps": "up_mbps", "down_mbps": "down_mbps",
		"server_ports": "server_ports", "hop_interval": "hop_interval", "obfs_type": "obfs", "obfs_password": "obfs-password",
	} {
		setQuery(q, target, node.Settings[key])
	}
	if settingEnabled(node.Settings, "insecure") {
		q.Set("insecure", "1")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func httpStyleShareLink(node config.Node) string {
	scheme := node.Protocol
	if scheme == "socks" {
		scheme = "socks5"
	} else if settingEnabled(node.Settings, "tls") {
		scheme = "https"
	}
	u := &url.URL{Scheme: scheme, Host: nodeAddress(node), Fragment: node.Name}
	username, password := node.Settings["username"], node.Settings["password"]
	if username != "" || password != "" {
		u.User = url.UserPassword(username, password)
	}
	q := url.Values{}
	if settingEnabled(node.Settings, "tls") {
		q.Set("tls", "1")
	}
	setQuery(q, "sni", node.Settings["server_name"])
	setQuery(q, "path", node.Settings["path"])
	if settingEnabled(node.Settings, "insecure") {
		q.Set("insecure", "1")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func setTransportQuery(q url.Values, settings map[string]string) {
	transport := settings["transport_type"]
	if transport == "" {
		transport = settings["network"]
	}
	setQuery(q, "type", transport)
	setQuery(q, "host", settings["transport_host"])
	setQuery(q, "path", settings["transport_path"])
	setQuery(q, "serviceName", settings["transport_service_name"])
}

func setQuery(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

func settingEnabled(settings map[string]string, key string) bool {
	value := settings[key]
	return value == "1" || strings.EqualFold(value, "true")
}

func nodeAddress(node config.Node) string {
	host := strings.TrimSuffix(strings.TrimPrefix(node.Server, "["), "]")
	return net.JoinHostPort(host, strconv.Itoa(int(node.Port)))
}
