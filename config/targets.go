package config

import (
	"fmt"
	"regexp"
	"strings"
)

// NodeGroup 按条件挑出一组节点：同一项里任选其一，不同项要同时满足，空项不限。
// 规则走节点组时，组内自动用延迟最低的节点；订阅更新后成员跟着变。
type NodeGroup struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Regions       []string `json:"regions,omitempty"`
	Subscriptions []string `json:"subscriptions,omitempty"` // 空字符串表示手动添加的节点
	Protocols     []string `json:"protocols,omitempty"`
	Keyword       string   `json:"keyword,omitempty"` // 名称包含其中任一词，空格或逗号分隔
	Exclude       string   `json:"exclude,omitempty"` // 名称包含其中任一词就排除
}

var regionCode = regexp.MustCompile(`^[A-Z]{2}$`)

func groupWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return r == ' ' || r == ',' || r == '，' || r == '\t' })
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func (g NodeGroup) Matches(n Node) bool {
	if len(g.Regions) > 0 && !contains(g.Regions, n.Country) {
		return false
	}
	if len(g.Subscriptions) > 0 && !contains(g.Subscriptions, n.Subscription) {
		return false
	}
	if len(g.Protocols) > 0 && !contains(g.Protocols, n.Protocol) {
		return false
	}
	name := strings.ToLower(n.Name)
	if words := groupWords(g.Keyword); len(words) > 0 {
		hit := false
		for _, w := range words {
			hit = hit || strings.Contains(name, w)
		}
		if !hit {
			return false
		}
	}
	for _, w := range groupWords(g.Exclude) {
		if strings.Contains(name, w) {
			return false
		}
	}
	return true
}

// 规则去向：proxy / direct / block，或带参数的 node:<节点ID>、group:<节点组ID>、
// sub:<订阅ID>、region:<地区代码>、proto:<协议>。
const (
	TargetNode   = "node"
	TargetGroup  = "group"
	TargetSub    = "sub"
	TargetRegion = "region"
	TargetProto  = "proto"
)

// SplitAction 拆出去向类型和参数；ID 区分大小写，参数原样保留。
func SplitAction(action string) (kind, arg string) {
	action = strings.TrimSpace(action)
	if i := strings.IndexByte(action, ':'); i > 0 {
		return strings.ToLower(action[:i]), strings.TrimSpace(action[i+1:])
	}
	return strings.ToLower(action), ""
}

// IsBasicAction 说明去向是不是 proxy/direct/block 这类不指向节点的动作。
func IsBasicAction(action string) bool {
	kind, arg := SplitAction(action)
	return arg == "" && (kind == "proxy" || kind == "direct" || kind == "block")
}

// IsBlockAction 说明去向是不是屏蔽；直连模式下只有屏蔽类规则生效。
func IsBlockAction(action string) bool {
	kind, arg := SplitAction(action)
	return kind == "block" && arg == ""
}

func ValidateAction(action string) error {
	kind, arg := SplitAction(action)
	switch kind {
	case "proxy", "direct", "block":
		if arg == "" {
			return nil
		}
	case TargetNode, TargetGroup, TargetSub, TargetProto:
		if arg != "" {
			return nil
		}
	case TargetRegion:
		if regionCode.MatchString(arg) {
			return nil
		}
		return fmt.Errorf("地区代码无效 %q（应为两位大写字母，如 US）", arg)
	}
	return fmt.Errorf("不支持动作 %q", action)
}

// TargetNodes 返回去向对应的节点；目标已被删除或没有匹配节点时返回空，调用方退回 proxy。
func (cfg Config) TargetNodes(action string) []Node {
	kind, arg := SplitAction(action)
	var match func(Node) bool
	switch kind {
	case TargetNode:
		match = func(n Node) bool { return n.ID == arg }
	case TargetSub:
		match = func(n Node) bool { return n.Subscription == arg }
	case TargetRegion:
		match = func(n Node) bool { return n.Country == arg }
	case TargetProto:
		match = func(n Node) bool { return n.Protocol == arg }
	case TargetGroup:
		for _, g := range cfg.NodeGroups {
			if g.ID == arg {
				match = g.Matches
			}
		}
	}
	if match == nil {
		return nil
	}
	var out []Node
	for _, n := range cfg.Nodes {
		if match(n) {
			out = append(out, n)
		}
	}
	return out
}

func validateGroups(groups []NodeGroup) error {
	ids := map[string]bool{}
	names := map[string]bool{}
	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		switch {
		case g.ID == "" || strings.ContainsAny(g.ID, " :"):
			return fmt.Errorf("节点组 %q 的 ID 无效", g.Name)
		case name == "":
			return fmt.Errorf("节点组名称不能为空")
		case ids[g.ID]:
			return fmt.Errorf("节点组 ID 重复: %s", g.ID)
		case names[name]:
			return fmt.Errorf("节点组名称重复: %s", name)
		}
		for _, r := range g.Regions {
			if !regionCode.MatchString(r) {
				return fmt.Errorf("节点组 %q 的地区代码无效: %q", name, r)
			}
		}
		ids[g.ID], names[name] = true, true
	}
	return nil
}
