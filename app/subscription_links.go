package app

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/caichengle666/sbtun/config"
)

// parseURLNode 解析 scheme://userinfo@host:port?query#name 形式的通用部分。
func parseURLNode(link, protocol string) (*url.URL, uint16, string, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, 0, "", fmt.Errorf("%s 解析失败: %w", protocol, err)
	}
	if u.Hostname() == "" || u.Port() == "" {
		return nil, 0, "", fmt.Errorf("%s 缺少服务器地址或端口", protocol)
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return nil, 0, "", fmt.Errorf("%s 端口无效: %w", protocol, err)
	}
	name := u.Fragment
	if name == "" {
		name = protocol + "-" + u.Hostname()
	}
	return u, port, name, nil
}

func queryEnabled(vals url.Values, keys ...string) bool {
	for _, key := range keys {
		if v := vals.Get(key); v == "1" || strings.EqualFold(v, "true") {
			return true
		}
	}
	return false
}

func setLinkTLS(settings map[string]string, vals url.Values) {
	if v := vals.Get("sni"); v != "" {
		settings["server_name"] = v
	}
	if v := vals.Get("alpn"); v != "" {
		settings["alpn"] = v
	}
	if v := vals.Get("fp"); v != "" {
		settings["utls_fingerprint"] = v
	}
	if queryEnabled(vals, "insecure", "allow_insecure", "allowInsecure") {
		settings["insecure"] = "true"
	}
}

// tuic://uuid:password@host:port?congestion_control=bbr&udp_relay_mode=native&sni=&alpn=h3#name
func parseTUIC(link string) (config.Node, error) {
	u, port, name, err := parseURLNode(link, "tuic")
	if err != nil {
		return config.Node{}, err
	}
	uuid := u.User.Username()
	if uuid == "" {
		return config.Node{}, fmt.Errorf("tuic 缺少 UUID")
	}
	password, _ := u.User.Password()
	settings := map[string]string{"uuid": uuid, "password": password}
	vals := u.Query()
	setLinkTLS(settings, vals)
	if v := firstNonEmpty(vals.Get("congestion_control"), vals.Get("congestion-control")); v != "" {
		settings["congestion_control"] = v
	}
	if v := firstNonEmpty(vals.Get("udp_relay_mode"), vals.Get("udp-relay-mode")); v != "" {
		settings["udp_relay_mode"] = v
	}
	return config.Node{ID: "tuic-" + u.Host, Name: name, Protocol: "tuic", Server: u.Hostname(), Port: port, Settings: settings}, nil
}

// anytls://password@host:port?sni=&insecure=1&fp=chrome#name
func parseAnyTLS(link string) (config.Node, error) {
	u, port, name, err := parseURLNode(link, "anytls")
	if err != nil {
		return config.Node{}, err
	}
	password := u.User.Username()
	if p, ok := u.User.Password(); ok {
		password = u.User.Username() + ":" + p
	}
	if password == "" {
		return config.Node{}, fmt.Errorf("anytls 缺少密码")
	}
	settings := map[string]string{"password": password}
	setLinkTLS(settings, u.Query())
	return config.Node{ID: "anytls-" + u.Host, Name: name, Protocol: "anytls", Server: u.Hostname(), Port: port, Settings: settings}, nil
}

// wireguard://privatekey@host:port?publickey=&address=10.0.0.2/32,fd00::2/128&reserved=1,2,3&mtu=1280#name
func parseWireGuard(link string) (config.Node, error) {
	u, port, name, err := parseURLNode(link, "wireguard")
	if err != nil {
		return config.Node{}, err
	}
	vals := u.Query()
	settings := map[string]string{
		"private_key": wireGuardKey(firstNonEmpty(u.User.Username(), vals.Get("privatekey"), vals.Get("private_key"))),
		"public_key":  wireGuardKey(firstNonEmpty(vals.Get("publickey"), vals.Get("public_key"), vals.Get("peer_public_key"))),
	}
	if settings["private_key"] == "" || settings["public_key"] == "" {
		return config.Node{}, fmt.Errorf("wireguard 缺少私钥或对端公钥")
	}
	for target, keys := range map[string][]string{
		"pre_shared_key": {"presharedkey", "pre_shared_key", "psk"},
		"local_address":  {"address", "ip", "local_address"},
		"reserved":       {"reserved"},
		"mtu":            {"mtu"},
		"allowed_ips":    {"allowed_ips", "allowedips"},
	} {
		for _, key := range keys {
			if v := vals.Get(key); v != "" {
				if target == "pre_shared_key" {
					v = wireGuardKey(v)
				}
				settings[target] = v
				break
			}
		}
	}
	if settings["local_address"] == "" {
		return config.Node{}, fmt.Errorf("wireguard 缺少本机地址 address")
	}
	return config.Node{ID: "wireguard-" + u.Host, Name: name, Protocol: "wireguard", Server: u.Hostname(), Port: port, Settings: settings}, nil
}

// wireGuardKey 还原 base64 密钥中被查询串解码成空格的 "+"。
func wireGuardKey(key string) string {
	return strings.ReplaceAll(strings.TrimSpace(key), " ", "+")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
