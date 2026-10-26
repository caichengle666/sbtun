package singbox

import (
	"strings"

	"github.com/caichengle666/sbtun/config"
)

// ruleTargets 给规则里指向节点、节点组、订阅、地区、协议的去向分配出站：
// 只剩一个可用节点就直接用 node-<id>；多个就建一个 urltest，自动用延迟最低的，挂了换下一个。
// built 是 buildSelector 的结果，坏节点已被跳过，不会出现在组里。
func ruleTargets(cfg config.Config, sets []configuredRuleSet, built []map[string]any) (map[string]string, []map[string]any) {
	available := make(map[string]bool, len(built))
	for _, out := range built {
		if tag, _ := out["tag"].(string); strings.HasPrefix(tag, "node-") {
			available[tag] = true
		}
	}
	targets := map[string]string{}
	var groups []map[string]any
	actions := make([]string, 0, len(cfg.CustomRules)+len(sets))
	for _, rule := range cfg.CustomRules {
		actions = append(actions, rule.Action)
	}
	for _, set := range sets {
		actions = append(actions, set.Action)
	}
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if _, done := targets[action]; done || config.IsBasicAction(action) {
			continue
		}
		var tags []string
		for _, n := range cfg.TargetNodes(action) {
			if tag := nodeTag(n.ID); available[tag] {
				tags = append(tags, tag)
			}
		}
		switch len(tags) {
		case 0:
			targets[action] = ""
		case 1:
			targets[action] = tags[0]
		default:
			tag := "auto-" + action
			targets[action] = tag
			groups = append(groups, map[string]any{
				"type": "urltest", "tag": tag, "outbounds": tags,
				"url": "https://www.gstatic.com/generate_204", "interval": "5m", "tolerance": 50,
			})
		}
	}
	return targets, groups
}
