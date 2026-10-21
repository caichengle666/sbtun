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
	if a.manager == nil {
		return nil, fmt.Errorf("配置管理器未初始化")
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
		if strings.HasPrefix(method, "2022-") {
			// SIP002：2022 系列方法的 userinfo 必须用百分号编码，不能用 base64。
			credentials = url.UserPassword(method, password).String()
		}
		query := ""
		if plugin := node.Settings["plugin"]; plugin != "" {
			if opts := node.Settings["plugin_opts"]; opts != "" {
				plugin += ";" + opts
			}
			query = "/?" + url.Values{"plugin": []string{plugin}}.Encode()
		}
		return "ss://" + credentials + "@" + nodeAddress(node) + query + "#" + url.PathEscape(node.Name), nil
	case "hysteria2":
		return hysteria2ShareLink(node), nil
	case "hysteria":
		return hysteriaShareLink(node), nil
	case "tuic":
		return tuicShareLink(node), nil
	case "anytls":
		return anyTLSShareLink(node), nil
	case "wireguard":
		return wireGuardShareLink(node), nil
	case "socks", "http":
		return httpStyleShareLink(node)
	default:
		return "", fmt.Errorf("不支持的协议: %s", node.Protocol)
	}
}

func vmessShareLink(node config.Node) (string, error) {
	transport := node.Settings["transport_type"]
	switch transport {
	case "":
		transport = "tcp"
	case "http":
		transport = "h2" // v2rayN 把 HTTP/2 传输写作 h2
	}
	security := node.Settings["security"]
	if security == "" {
		security = "auto"
	}
	path := node.Settings["transport_path"]
	if transport == "grpc" {
		path = node.Settings["transport_service_name"]
	}
	cfg := vmessConfig{
		V: "2", Ps: looseString(node.Name), Add: looseString(node.Server), Port: looseString(strconv.Itoa(int(node.Port))),
		ID: looseString(node.Settings["uuid"]), Aid: looseString(node.Settings["alter_id"]), Net: looseString(transport),
		Type: "none", Scy: looseString(security), Host: looseString(node.Settings["transport_host"]),
		Path: looseString(path), SNI: looseString(node.Settings["server_name"]),
		ALPN: looseString(node.Settings["alpn"]), FP: looseString(node.Settings["utls_fingerprint"]),
	}
	if settingEnabled(node.Settings, "tls") {
		cfg.TLS = "tls"
	}
	if settingEnabled(node.Settings, "insecure") {
		cfg.AllowInsecure = "1"
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
	setQuery(q, "packetEncoding", node.Settings["packet_encoding"])
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
	setQuery(q, "fp", node.Settings["utls_fingerprint"])
	// trojan 链接不写 security 时默认 TLS，明文节点必须写明 none。
	if settingEnabled(node.Settings, "tls") {
		q.Set("security", "tls")
	} else {
		q.Set("security", "none")
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

func hysteriaShareLink(node config.Node) string {
	u := &url.URL{Scheme: "hysteria", Host: nodeAddress(node), Fragment: node.Name}
	q := url.Values{}
	q.Set("protocol", "udp")
	for key, target := range map[string]string{
		"password": "auth", "sni": "peer", "alpn": "alpn", "up_mbps": "upmbps", "down_mbps": "downmbps",
		"server_ports": "mport", "obfs_password": "obfsParam",
	} {
		setQuery(q, target, node.Settings[key])
	}
	if node.Settings["obfs_password"] != "" {
		q.Set("obfs", "xplus")
	}
	if settingEnabled(node.Settings, "insecure") {
		q.Set("insecure", "1")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func tuicShareLink(node config.Node) string {
	u := &url.URL{Scheme: "tuic", User: url.UserPassword(node.Settings["uuid"], node.Settings["password"]), Host: nodeAddress(node), Fragment: node.Name}
	q := tlsShareQuery(node.Settings)
	setQuery(q, "congestion_control", node.Settings["congestion_control"])
	setQuery(q, "udp_relay_mode", node.Settings["udp_relay_mode"])
	u.RawQuery = q.Encode()
	return u.String()
}

func anyTLSShareLink(node config.Node) string {
	u := &url.URL{Scheme: "anytls", User: url.User(node.Settings["password"]), Host: nodeAddress(node), Fragment: node.Name}
	u.RawQuery = tlsShareQuery(node.Settings).Encode()
	return u.String()
}

func wireGuardShareLink(node config.Node) string {
	u := &url.URL{Scheme: "wireguard", User: url.User(node.Settings["private_key"]), Host: nodeAddress(node), Fragment: node.Name}
	q := url.Values{}
	for key, target := range map[string]string{
		"public_key": "publickey", "pre_shared_key": "presharedkey", "local_address": "address",
		"reserved": "reserved", "mtu": "mtu", "allowed_ips": "allowed_ips",
	} {
		setQuery(q, target, node.Settings[key])
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func tlsShareQuery(settings map[string]string) url.Values {
	q := url.Values{}
	setQuery(q, "sni", settings["server_name"])
	setQuery(q, "alpn", settings["alpn"])
	setQuery(q, "fp", settings["utls_fingerprint"])
	if settingEnabled(settings, "insecure") {
		q.Set("insecure", "1")
	}
	return q
}

func httpStyleShareLink(node config.Node) (string, error) {
	scheme := node.Protocol
	if scheme == "socks" {
		switch strings.ToLower(strings.TrimSpace(node.Settings["version"])) {
		case "4":
			scheme = "socks4"
		case "4a":
			scheme = "socks4a"
		default:
			scheme = "socks5"
		}
	} else if settingEnabled(node.Settings, "tls") {
		scheme = "https"
	}
	u := &url.URL{Scheme: scheme, Host: nodeAddress(node), Fragment: node.Name}
	username, password := node.Settings["username"], node.Settings["password"]
	if username != "" || password != "" {
		if strings.HasPrefix(scheme, "socks4") && password != "" {
			return "", fmt.Errorf("SOCKS%s 不支持密码认证，请改用 SOCKS5", strings.TrimPrefix(scheme, "socks"))
		}
		if password == "" {
			u.User = url.User(username)
		} else {
			u.User = url.UserPassword(username, password)
		}
	}
	q := url.Values{}
	if scheme != "https" && settingEnabled(node.Settings, "tls") {
		q.Set("tls", "1")
	}
	setQuery(q, "sni", node.Settings["server_name"])
	setQuery(q, "path", node.Settings["path"])
	if settingEnabled(node.Settings, "insecure") {
		q.Set("insecure", "1")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func setTransportQuery(q url.Values, settings map[string]string) {
	setQuery(q, "type", settings["transport_type"])
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
