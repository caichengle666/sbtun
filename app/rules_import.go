package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/caichengle666/sbtun/config"
)

func (a *App) ImportCustomRules(source string) (int, error) {
	// 下载可能耗时（最长 30 秒），放在锁外，不能卡住切换节点、改设置等操作。
	data, err := loadRuleSource(strings.TrimSpace(source))
	if err != nil {
		return 0, err
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	count := 0
	err = a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		// 去向写的是节点名、节点组名、地区时，按当前配置换成对应的去向。
		rules, err := parseCustomRules(data, *next)
		if err != nil {
			return err
		}
		next.CustomRules = append(next.CustomRules, rules...)
		count = len(rules)
		return nil
	}, true)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func loadRuleSource(source string) ([]byte, error) {
	if source == "" {
		return nil, fmt.Errorf("规则内容或链接不能为空")
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Get(source)
		if err != nil {
			return nil, fmt.Errorf("下载规则失败: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("规则 HTTP %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxSubscriptionBytes+1))
		if err != nil {
			return nil, err
		}
		if len(body) > maxSubscriptionBytes {
			return nil, fmt.Errorf("规则内容超过 %d 字节上限", maxSubscriptionBytes)
		}
		return body, nil
	}
	if data, err := os.ReadFile(source); err == nil {
		return data, nil
	}
	return []byte(source), nil
}

// Clash / Surge / Quantumult X 的规则类型；GEOIP、MATCH、RULE-SET 这类本软件没有对应的直接跳过。
var foreignMatchTypes = map[string]string{
	"domain-suffix": "domain_suffix", "host-suffix": "domain_suffix",
	"domain-keyword": "domain_keyword", "host-keyword": "domain_keyword",
	"domain": "domain", "host": "domain",
	"ip-cidr": "ip_cidr", "ip-cidr6": "ip_cidr", "ip6-cidr": "ip_cidr",
	"dst-port": "port", "dest-port": "port",
}

var nativeMatchTypes = map[string]bool{"domain_suffix": true, "domain_keyword": true, "domain": true, "ip_cidr": true, "port": true}

// parseCustomRules 支持 JSON 数组、本软件的 “去向,类型,内容” / “类型,内容,去向”，
// 以及 Clash（含 YAML 里的 “- ” 列表）、Surge、Quantumult X 的 “类型,内容,策略”。
func parseCustomRules(data []byte, cfg config.Config) ([]config.Rule, error) {
	text := strings.TrimSpace(string(data))
	var rules []config.Rule
	if strings.HasPrefix(text, "[") {
		if err := json.Unmarshal([]byte(text), &rules); err != nil {
			return nil, fmt.Errorf("JSON 规则格式错误: %w", err)
		}
		for i := range rules {
			rules[i].Action = ResolveRuleTarget(rules[i].Action, cfg)
		}
	} else {
		for _, line := range strings.Split(text, "\n") {
			if rule, ok := parseRuleLine(line, cfg); ok {
				rules = append(rules, rule)
			}
		}
	}
	for _, rule := range rules {
		if !nativeMatchTypes[strings.ToLower(rule.MatchType)] || strings.TrimSpace(rule.Value) == "" {
			return nil, fmt.Errorf("无效规则: %s,%s,%s", rule.Action, rule.MatchType, rule.Value)
		}
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("未识别到有效规则")
	}
	return rules, nil
}

func parseRuleLine(line string, cfg config.Config) (config.Rule, bool) {
	line = strings.TrimSpace(line)
	line = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "- ")), `"'`)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
		return config.Rule{}, false
	}
	parts := strings.Split(line, ",")
	if len(parts) < 3 {
		parts = strings.Fields(line)
	}
	if len(parts) < 3 {
		return config.Rule{}, false
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	first := strings.ToLower(parts[0])
	if match, ok := foreignMatchTypes[first]; ok {
		return config.Rule{MatchType: match, Value: parts[1], Action: ResolveRuleTarget(parts[2], cfg)}, true
	}
	if nativeMatchTypes[first] {
		return config.Rule{MatchType: first, Value: parts[1], Action: ResolveRuleTarget(strings.Join(parts[2:], ","), cfg)}, true
	}
	if match := strings.ToLower(parts[1]); nativeMatchTypes[match] && config.IsBasicAction(first) {
		return config.Rule{MatchType: match, Value: strings.Join(parts[2:], ","), Action: first}, true
	}
	return config.Rule{}, false
}

var protocolAliases = map[string]string{
	"hy2": "hysteria2", "hysteria2": "hysteria2", "hysteria": "hysteria", "vless": "vless", "vmess": "vmess",
	"trojan": "trojan", "ss": "shadowsocks", "shadowsocks": "shadowsocks", "tuic": "tuic", "anytls": "anytls", "wireguard": "wireguard", "wg": "wireguard",
}

// ResolveRuleTarget 把外部规则里的策略名换成本软件的去向：直连/拒绝/代理，或者同名的
// 节点组、节点、订阅、协议、地区；都对不上（比如 “🚀 节点选择”）就走代理。
func ResolveRuleTarget(target string, cfg config.Config) string {
	target = strings.TrimSpace(target)
	lower := strings.ToLower(target)
	switch {
	case lower == "direct" || lower == "直连":
		return "direct"
	case lower == "block" || strings.HasPrefix(lower, "reject") || lower == "拒绝" || lower == "阻断":
		return "block"
	case lower == "proxy" || lower == "代理":
		return "proxy"
	}
	switch kind, _ := config.SplitAction(target); kind {
	case config.TargetNode, config.TargetGroup, config.TargetSub, config.TargetRegion, config.TargetProto:
		return target // 已经是 node:/group:/region: 这类写法，保存时统一校验
	}
	for _, g := range cfg.NodeGroups {
		if g.Name == target {
			return config.TargetGroup + ":" + g.ID
		}
	}
	for _, n := range cfg.Nodes {
		if n.Name == target {
			return config.TargetNode + ":" + n.ID
		}
	}
	for _, sub := range cfg.Subscriptions {
		if sub.Name == target {
			return config.TargetSub + ":" + sub.ID
		}
	}
	if proto, ok := protocolAliases[lower]; ok {
		return config.TargetProto + ":" + proto
	}
	if code := config.GuessRegion(target); code != "" {
		return config.TargetRegion + ":" + code
	}
	return "proxy"
}
