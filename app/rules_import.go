package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/caichengle666/sbtun/config"
	"gopkg.in/yaml.v3"
)

// RuleImportResult 说明导入了什么：单条规则进分流规则列表，引用的外部列表各自成为一个规则集。
type RuleImportResult struct {
	Rules    int      `json:"rules"`
	RuleSets int      `json:"rule_sets"`
	Existing int      `json:"existing"`
	Failed   []string `json:"failed,omitempty"`
}

func (a *App) ImportCustomRules(source string) (RuleImportResult, error) {
	var result RuleImportResult
	// 下载可能耗时（最长 30 秒），放在锁外，不能卡住切换节点、改设置等操作。
	data, err := loadRuleSource(strings.TrimSpace(source))
	if err != nil {
		return result, err
	}
	cfg := a.GetConfig()
	parsed, refs, err := parseRuleImport(data, cfg)
	if err != nil {
		return result, err
	}
	urls := make([]string, len(refs))
	for i, ref := range refs {
		urls[i] = ref.URL
	}
	fetched := a.fetchRuleURLs(urls)

	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	var added []string
	for i, ref := range refs {
		switch {
		case a.rulesManager == nil:
			return result, errNoRulesManager
		case a.rulesManager.HasURL(ref.URL):
			result.Existing++
		case fetched[i].err != nil:
			result.Failed = append(result.Failed, ref.Name+": "+fetched[i].err.Error())
		default:
			if id, err := a.rulesManager.Add(ref.Name, ref.URL, ref.Action, fetched[i].data, a.validateRuleSet); err != nil {
				result.Failed = append(result.Failed, ref.Name+": "+err.Error())
			} else {
				added = append(added, id)
			}
		}
	}
	result.RuleSets = len(added)
	if len(parsed) == 0 {
		if result.RuleSets == 0 {
			if len(result.Failed) > 0 {
				return result, fmt.Errorf("规则集下载失败: %s", strings.Join(result.Failed, "；"))
			}
			return result, nil
		}
		return result, a.reloadIfRunningLocked()
	}
	// 新规则集已写进规则清单，生成的配置跟着变，这里的重载会一并带上。
	err = a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		next.CustomRules = append(next.CustomRules, parsed...)
		return nil
	}, true)
	if err != nil {
		// 规则没保存上，这次加的规则集也撤掉，不留一半。
		for _, id := range added {
			_ = a.rulesManager.Delete(id)
		}
		if len(added) > 0 {
			a.resyncRuntimeLocked()
		}
		result.RuleSets = 0
		return result, err
	}
	result.Rules = len(parsed)
	return result, nil
}

// ruleSetRef 是导入内容里引用的外部规则列表（Clash 的 RULE-SET + rule-providers、Surge 的 RULE-SET/DOMAIN-SET、
// subconverter 的 ruleset=），导入后成为一个规则集，整体更新、整体改去向。
type ruleSetRef struct {
	Name, URL, Action string
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

// parseRuleImport 支持 JSON 数组、本软件的 “去向,类型,内容” / “类型,内容,去向”，
// 以及 Clash（含 YAML 里的 “- ” 列表）、Surge、Quantumult X 的 “类型,内容,策略”。
// 引用外部列表的行（RULE-SET、DOMAIN-SET、ruleset=）单独返回，由调用方下载成规则集。
func parseRuleImport(data []byte, cfg config.Config) ([]config.Rule, []ruleSetRef, error) {
	text := strings.TrimSpace(string(data))
	var rules []config.Rule
	var refs []ruleSetRef
	if strings.HasPrefix(text, "[") && json.Valid([]byte(text)) {
		if err := json.Unmarshal([]byte(text), &rules); err != nil {
			return nil, nil, fmt.Errorf("JSON 规则格式错误: %w", err)
		}
		for i := range rules {
			rules[i].Action = ResolveRuleTarget(rules[i].Action, cfg)
		}
	} else {
		providers := clashRuleProviders(data)
		seen := map[string]bool{}
		for _, line := range strings.Split(text, "\n") {
			if ref, ok := parseRuleSetRef(line, providers, cfg); ok {
				if !seen[ref.URL] {
					seen[ref.URL] = true
					refs = append(refs, ref)
				}
			} else if rule, ok := parseRuleLine(line, cfg); ok {
				rules = append(rules, rule)
			}
		}
	}
	for _, rule := range rules {
		if !nativeMatchTypes[strings.ToLower(rule.MatchType)] || strings.TrimSpace(rule.Value) == "" {
			return nil, nil, fmt.Errorf("无效规则: %s,%s,%s", rule.Action, rule.MatchType, rule.Value)
		}
	}
	if len(rules) == 0 && len(refs) == 0 {
		return nil, nil, fmt.Errorf("未识别到有效规则（只有域名或 IP 列表、没写去向的，请在“规则集”里添加并选择去向）")
	}
	return rules, refs, nil
}

// clashRuleProviders 读出 Clash 配置里 rule-providers 的 名称 → 下载地址。
func clashRuleProviders(data []byte) map[string]string {
	var doc struct {
		Providers map[string]struct {
			URL string `yaml:"url"`
		} `yaml:"rule-providers"`
	}
	if !bytes.Contains(data, []byte("rule-providers")) || yaml.Unmarshal(data, &doc) != nil {
		return nil
	}
	out := map[string]string{}
	for name, p := range doc.Providers {
		if isHTTPURL(p.URL) {
			out[name] = p.URL
		}
	}
	return out
}

// parseRuleSetRef 认出引用外部列表的一行：
// Clash “RULE-SET,名称,策略”、Surge “RULE-SET,链接,策略” / “DOMAIN-SET,链接,策略”、subconverter “ruleset=策略,链接”。
func parseRuleSetRef(line string, providers map[string]string, cfg config.Config) (ruleSetRef, bool) {
	line = strings.Trim(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- ")), `"'`)
	var source, policy string
	if rest, ok := cutPrefixFold(line, "ruleset="); ok {
		policy, source, _ = strings.Cut(rest, ",")
		for _, prefix := range []string{"clash-domain:", "clash-ipcidr:", "clash-classic:", "clash-classical:", "surge:", "quanx:"} {
			source, _ = cutPrefixFold(source, prefix)
		}
	} else {
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			return ruleSetRef{}, false
		}
		kind := strings.ToUpper(strings.TrimSpace(parts[0]))
		if kind != "RULE-SET" && kind != "DOMAIN-SET" {
			return ruleSetRef{}, false
		}
		source, policy = strings.TrimSpace(parts[1]), parts[2]
	}
	source = strings.TrimSpace(source)
	name := source
	if url, ok := providers[source]; ok {
		source = url
	} else if isHTTPURL(source) {
		name = ruleSetNameFromURL(source)
	}
	if !isHTTPURL(source) {
		return ruleSetRef{}, false
	}
	return ruleSetRef{Name: name, URL: source, Action: ResolveRuleTarget(policy, cfg)}, true
}

func ruleSetNameFromURL(raw string) string {
	name := raw
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.IndexAny(name, "?#"); i >= 0 {
		name = name[:i]
	}
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	if name == "" {
		return raw
	}
	return name
}

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
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
	// 常见规则模板里的策略组名，如 “🎯 全球直连”“🛑 广告拦截”“🍃 应用净化”。
	switch {
	case strings.Contains(lower, "直连") || strings.Contains(lower, "direct"):
		return "direct"
	case strings.Contains(lower, "拦截") || strings.Contains(lower, "屏蔽") || strings.Contains(lower, "净化") || strings.Contains(lower, "reject"):
		return "block"
	}
	if proto, ok := protocolAliases[lower]; ok {
		return config.TargetProto + ":" + proto
	}
	if code := config.GuessRegion(target); code != "" {
		return config.TargetRegion + ":" + code
	}
	return "proxy"
}
