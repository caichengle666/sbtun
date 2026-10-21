package singbox

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// outboundFields 是各出站类型允许出现的 sing-box 字段（type/tag/server/server_port/domain_resolver 之外）。
// 节点设置里多出来的键一律不写：sing-box 遇到不认识的字段会拒绝整份配置。
var outboundFields = map[string]string{
	"vmess":       "uuid security alter_id network packet_encoding multiplex tls transport",
	"vless":       "uuid flow network packet_encoding multiplex tls transport",
	"trojan":      "password network multiplex tls transport",
	"shadowsocks": "method password plugin plugin_opts network udp_over_tcp multiplex",
	"socks":       "version username password network udp_over_tcp",
	"http":        "username password path tls",
	"tuic":        "uuid password congestion_control udp_relay_mode network tls",
	"anytls":      "password tls",
}

func keepOutboundFields(out map[string]any) {
	allowed := " " + outboundFields[out["type"].(string)] + " "
	for k := range out {
		switch k {
		case "type", "tag", "server", "server_port", "domain_resolver":
			continue
		}
		if !strings.Contains(allowed, " "+k+" ") {
			delete(out, k)
		}
	}
	if out["type"] == "shadowsocks" {
		fixSS2022Key(out)
	}
}

// utlsFingerprint 返回 sing-box 认识的 uTLS 指纹；空或 none 表示不启用，不认识的按 chrome 处理。
func utlsFingerprint(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "", "none":
		return ""
	case "chrome", "firefox", "edge", "safari", "360", "qq", "ios", "android", "random", "randomized",
		"chrome_psk", "chrome_psk_shuffle", "chrome_padding_psk_shuffle", "chrome_pq", "chrome_pq_psk":
		return v
	}
	return "chrome"
}

// ss2022KeyLen 返回 Shadowsocks 2022 方法要求的密钥字节数，非 2022 方法返回 0。
func ss2022KeyLen(method string) int {
	switch method {
	case "2022-blake3-aes-128-gcm":
		return 16
	case "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
		return 32
	}
	return 0
}

// fixSS2022Key 把无填充或 URL 安全写法的 2022 密钥统一成 sing-box 要求的标准 base64。
// 多用户写法 "服务端密钥:用户密钥" 逐段处理；解不开的保持原样，交给 checkOutbound 报错。
func fixSS2022Key(out map[string]any) {
	method, _ := out["method"].(string)
	password, _ := out["password"].(string)
	if ss2022KeyLen(method) == 0 {
		return
	}
	parts := strings.Split(password, ":")
	for i, part := range parts {
		raw := strings.NewReplacer("-", "+", "_", "/").Replace(strings.TrimRight(part, "="))
		if key, err := base64.RawStdEncoding.DecodeString(raw); err == nil {
			parts[i] = base64.StdEncoding.EncodeToString(key)
		}
	}
	out["password"] = strings.Join(parts, ":")
}

var ssMethods = " none aes-128-gcm aes-192-gcm aes-256-gcm chacha20-ietf-poly1305 xchacha20-ietf-poly1305 " +
	"2022-blake3-aes-128-gcm 2022-blake3-aes-256-gcm 2022-blake3-chacha20-poly1305 " +
	"aes-128-ctr aes-192-ctr aes-256-ctr aes-128-cfb aes-192-cfb aes-256-cfb rc4-md5 chacha20-ietf xchacha20 "

// checkOutbound 对单个出站做低成本校验，只挡 sing-box 1.14 实测必然拒绝的取值
// （字段名已由 keepOutboundFields 过滤，传输字段由 addTransport 按类型生成）。
func checkOutbound(out map[string]any) error {
	str := func(k string) string { s, _ := out[k].(string); return s }
	oneOf := func(k, allowed string) error {
		if v := str(k); v != "" && !strings.Contains(" "+allowed+" ", " "+v+" ") {
			return fmt.Errorf("%s 取值无效: %s", k, v)
		}
		return nil
	}
	checks := []error{oneOf("network", "tcp udp"), oneOf("packet_encoding", "packetaddr xudp")}
	switch out["type"] {
	case "shadowsocks":
		checks = append(checks, oneOf("plugin", "obfs-local v2ray-plugin"), checkSSKey(str("method"), str("password")))
		if !strings.Contains(ssMethods, " "+str("method")+" ") {
			checks = append(checks, fmt.Errorf("不支持的 Shadowsocks 加密方式: %s", str("method")))
		}
	case "vmess":
		checks = append(checks, oneOf("security", "auto none zero aes-128-gcm chacha20-poly1305"))
		if _, bad := out["alter_id"].(string); bad {
			checks = append(checks, fmt.Errorf("alter_id 不是数字: %s", str("alter_id")))
		}
	case "vless":
		checks = append(checks, oneOf("flow", "xtls-rprx-vision"))
	case "socks":
		checks = append(checks, oneOf("version", "4 4a 5"))
	case "tuic":
		checks = append(checks, oneOf("congestion_control", "cubic new_reno bbr"))
		if id := strings.ReplaceAll(str("uuid"), "-", ""); len(id) != 32 || !isHex(id) {
			checks = append(checks, fmt.Errorf("TUIC uuid 无效: %s", str("uuid")))
		}
	case "hysteria", "hysteria2":
		checks = append(checks, checkHopInterval(str("hop_interval")))
		ports, _ := out["server_ports"].([]string)
		for _, p := range ports {
			from, to, _ := strings.Cut(p, ":")
			if _, err := strconv.ParseUint(from, 10, 16); err != nil || to != "" && !isDigits(to) {
				checks = append(checks, fmt.Errorf("端口范围无效: %s", p))
			}
		}
		if obfs, _ := out["obfs"].(map[string]any); obfs != nil && obfs["type"] != "salamander" {
			checks = append(checks, fmt.Errorf("不支持的混淆类型: %v", obfs["type"]))
		}
	}
	checks = append(checks, checkReality(out))
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}

func checkSSKey(method, password string) error {
	n := ss2022KeyLen(method)
	if n == 0 {
		return nil
	}
	for _, part := range strings.Split(password, ":") {
		if key, err := base64.StdEncoding.DecodeString(part); err != nil || len(key) != n {
			return fmt.Errorf("%s 密钥必须是 %d 字节的 base64", method, n)
		}
	}
	return nil
}

func checkHopInterval(v string) error {
	if v == "" {
		return nil
	}
	if d, ok := strings.CutSuffix(v, "d"); ok && isDigits(d) {
		return nil
	}
	if _, err := time.ParseDuration(v); err != nil {
		return fmt.Errorf("hop_interval 无效: %s", v)
	}
	return nil
}

// checkReality：公钥须为 32 字节 base64url；short_id 须为不超过 16 位的偶数长度十六进制
// （过长时 sing-box 1.14 会直接崩溃）。
func checkReality(out map[string]any) error {
	tls, _ := out["tls"].(map[string]any)
	reality, _ := tls["reality"].(map[string]any)
	if reality == nil {
		return nil
	}
	pbk, _ := reality["public_key"].(string)
	if key, err := base64.RawURLEncoding.DecodeString(pbk); err != nil || len(key) != 32 {
		return fmt.Errorf("REALITY 公钥无效: %s", pbk)
	}
	sid, _ := reality["short_id"].(string)
	if _, err := hex.DecodeString(sid); err != nil || len(sid) > 16 {
		return fmt.Errorf("REALITY short_id 无效: %s", sid)
	}
	return nil
}

func isDigits(s string) bool {
	_, err := strconv.ParseUint(s, 10, 64)
	return err == nil
}

func isHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}
