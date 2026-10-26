package rules

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
)

// FileFormat 返回规则集文件交给 sing-box 时的格式：文本列表转换后存成 .json（source），其余是 .srs（binary）。
func FileFormat(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return "source"
	}
	return "binary"
}

func isBinaryRuleSet(data []byte) bool { return bytes.HasPrefix(data, []byte("SRS")) }

var textRuleTypes = map[string]string{
	"DOMAIN": "domain", "HOST": "domain",
	"DOMAIN-SUFFIX": "domain_suffix", "HOST-SUFFIX": "domain_suffix",
	"DOMAIN-KEYWORD": "domain_keyword", "HOST-KEYWORD": "domain_keyword",
	"DOMAIN-REGEX": "domain_regex",
	"IP-CIDR":      "ip_cidr", "IP-CIDR6": "ip_cidr", "IP6-CIDR": "ip_cidr",
	"DST-PORT": "port", "DEST-PORT": "port",
	"PROCESS-NAME": "process_name",
}

// ConvertText 把文本规则列表转换成 sing-box 源格式规则集，返回 JSON 和规则条数。认得：
// Clash 规则集（classical / domain / ipcidr，YAML 的 payload 或纯文本）、Surge 列表、
// 每行一个域名或网段、AdGuard 的 ||example.com^、hosts 文件。列表里写的策略会被忽略，去向由规则集决定。
func ConvertText(data []byte) ([]byte, int, error) {
	fields := map[string][]string{}
	var ports []int
	seen := map[string]bool{}
	count := 0
	add := func(kind, value string) {
		if key := kind + "\x00" + value; !seen[key] {
			seen[key] = true
			fields[kind] = append(fields[kind], value)
			count++
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		kind, value, ok := parseTextRule(line)
		if !ok {
			continue
		}
		if kind == "port" {
			p, err := strconv.Atoi(value)
			if err != nil || p < 1 || p > 65535 || seen["port\x00"+value] {
				continue
			}
			seen["port\x00"+value] = true
			ports = append(ports, p)
			count++
			continue
		}
		add(kind, value)
	}
	if count == 0 {
		return nil, 0, errors.New("没有识别到可用的规则（支持 .srs、Clash/Surge 规则列表、域名或 IP 段列表）")
	}
	// 域名和 IP 段写在同一条里是“任一命中”；端口、进程名在 sing-box 里与它们是“同时满足”，所以各自单独成条。
	var out []map[string]any
	main := map[string]any{}
	for _, kind := range []string{"domain", "domain_suffix", "domain_keyword", "domain_regex", "ip_cidr"} {
		if len(fields[kind]) > 0 {
			main[kind] = fields[kind]
		}
	}
	if len(main) > 0 {
		out = append(out, main)
	}
	if len(ports) > 0 {
		out = append(out, map[string]any{"port": ports})
	}
	if len(fields["process_name"]) > 0 {
		out = append(out, map[string]any{"process_name": fields["process_name"]})
	}
	encoded, err := json.Marshal(map[string]any{"version": 2, "rules": out})
	return encoded, count, err
}

func parseTextRule(line string) (kind, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || line == "payload:" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") ||
		strings.HasPrefix(line, "//") || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "[") {
		return "", "", false
	}
	line = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "- ")), `"'`)
	if strings.Contains(line, ",") {
		parts := strings.Split(line, ",")
		kind = textRuleTypes[strings.ToUpper(strings.TrimSpace(parts[0]))]
		value = strings.TrimSpace(parts[1])
		if kind == "" || value == "" {
			return "", "", false
		}
		if kind == "ip_cidr" {
			return cidrRule(value)
		}
		return kind, value, true
	}
	if parts := strings.Fields(line); len(parts) == 2 {
		// hosts 文件：0.0.0.0 ads.example.com
		if _, err := netip.ParseAddr(parts[0]); err == nil && isDomain(parts[1]) && parts[1] != "localhost" {
			return "domain", strings.ToLower(parts[1]), true
		}
		return "", "", false
	} else if len(parts) != 1 {
		return "", "", false
	}
	if strings.HasPrefix(line, "||") {
		// AdGuard：||example.com^ 及其子域名；带路径或通配的规则无法对应，跳过。
		line = strings.TrimPrefix(line, "||")
		if i := strings.IndexAny(line, "^$"); i >= 0 {
			line = line[:i]
		}
		if isDomain(line) {
			return "domain_suffix", strings.ToLower(line), true
		}
		return "", "", false
	}
	for _, prefix := range []string{"+.", "*.", "."} {
		if strings.HasPrefix(line, prefix) && isDomain(line[len(prefix):]) {
			return "domain_suffix", strings.ToLower(line[len(prefix):]), true
		}
	}
	if kind, value, ok := cidrRule(line); ok {
		return kind, value, true
	}
	if isDomain(line) {
		return "domain", strings.ToLower(line), true
	}
	return "", "", false
}

func cidrRule(value string) (string, string, bool) {
	if prefix, err := netip.ParsePrefix(value); err == nil {
		return "ip_cidr", prefix.Masked().String(), true
	}
	if addr, err := netip.ParseAddr(value); err == nil {
		return "ip_cidr", netip.PrefixFrom(addr, addr.BitLen()).String(), true
	}
	return "", "", false
}

func isDomain(s string) bool {
	if !strings.Contains(s, ".") || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}
	for _, r := range s {
		if !(r == '.' || r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127) {
			return false
		}
	}
	return true
}
