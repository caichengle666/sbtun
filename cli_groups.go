//go:build cli

package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/caichengle666/sbtun/app"
	"github.com/caichengle666/sbtun/config"
)

// cliRuleTarget 解析命令行里的去向：proxy/direct/block、node:/group:/sub:/region:/proto: 写法，
// 或直接写节点组名、节点名、订阅名、协议、地区（如 美国、US）。
func cliRuleTarget(cfg config.Config, text string) (string, error) {
	if err := config.ValidateAction(text); err == nil {
		kind, arg := config.SplitAction(text)
		if arg == "" {
			return kind, nil
		}
		return text, nil
	}
	action := app.ResolveRuleTarget(text, cfg)
	if action == "proxy" {
		return "", fmt.Errorf("找不到去向 %q：可写 proxy/direct/block，或节点组名、节点名、订阅名、协议、地区", text)
	}
	return action, config.ValidateAction(action)
}

// describeAction 把去向写成人能看懂的样子，目标没了时提示暂走代理。
func describeAction(cfg config.Config, action string) string {
	if config.IsBasicAction(action) {
		kind, _ := config.SplitAction(action)
		return ruleActionLabels[kind]
	}
	kind, arg := config.SplitAction(action)
	name := ""
	switch kind {
	case config.TargetNode:
		for _, n := range cfg.Nodes {
			if n.ID == arg {
				name = "节点 " + n.Name
			}
		}
	case config.TargetGroup:
		for _, g := range cfg.NodeGroups {
			if g.ID == arg {
				name = "节点组 " + g.Name
			}
		}
	case config.TargetSub:
		for _, s := range cfg.Subscriptions {
			if s.ID == arg {
				name = "订阅 " + s.Name
			}
		}
	case config.TargetRegion:
		name = "地区 " + arg
	case config.TargetProto:
		name = "协议 " + arg
	}
	if name == "" {
		return action + "（目标已删除，暂走代理）"
	}
	if count := len(cfg.TargetNodes(action)); count == 0 {
		return name + "（没有匹配节点，暂走代理）"
	} else if count > 1 {
		return fmt.Sprintf("%s（%d 个节点，自动选最快）", name, count)
	}
	return name
}

func validateGroupsArgs(args []string) error {
	switch {
	case len(args) == 0, len(args) == 1 && args[0] == "list":
		return nil
	case len(args) == 2 && args[0] == "delete":
		if index, err := strconv.Atoi(args[1]); err != nil || index < 1 {
			return fmt.Errorf("节点组编号无效: %s", args[1])
		}
		return nil
	case len(args) >= 2 && args[0] == "add":
		for _, cond := range args[2:] {
			key, _, ok := strings.Cut(cond, "=")
			if !ok || !strings.Contains(" region sub proto keyword exclude ", " "+key+" ") {
				return fmt.Errorf("条件无效: %s（可用 region= sub= proto= keyword= exclude=）", cond)
			}
		}
		return nil
	}
	return usageError("groups", args)
}

func splitValues(v string) []string {
	var out []string
	for _, item := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '，' }) {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// parseGroup 把 region=US,JP sub=机场B proto=hy2,vless keyword=IPLC exclude=过期 换成节点组条件。
func parseGroup(cfg config.Config, name string, conds []string) (config.NodeGroup, error) {
	g := config.NodeGroup{Name: strings.TrimSpace(name)}
	for _, cond := range conds {
		key, value, _ := strings.Cut(cond, "=")
		switch key {
		case "region":
			for _, v := range splitValues(value) {
				code := strings.ToUpper(v)
				if len(code) != 2 {
					code = config.GuessRegion(v)
				}
				if code == "" {
					return g, fmt.Errorf("认不出地区: %s", v)
				}
				g.Regions = append(g.Regions, code)
			}
		case "sub":
			for _, v := range splitValues(value) {
				id, err := subscriptionIDByName(cfg, v)
				if err != nil {
					return g, err
				}
				g.Subscriptions = append(g.Subscriptions, id)
			}
		case "proto":
			for _, v := range splitValues(value) {
				action := app.ResolveRuleTarget(v, config.Config{})
				kind, proto := config.SplitAction(action)
				if kind != config.TargetProto {
					return g, fmt.Errorf("认不出协议: %s", v)
				}
				g.Protocols = append(g.Protocols, proto)
			}
		case "keyword":
			g.Keyword = value
		case "exclude":
			g.Exclude = value
		}
	}
	return g, nil
}

func subscriptionIDByName(cfg config.Config, v string) (string, error) {
	if v == "manual" || v == "手动" {
		return "", nil
	}
	if index, err := strconv.Atoi(v); err == nil && index >= 1 && index <= len(cfg.Subscriptions) {
		return cfg.Subscriptions[index-1].ID, nil
	}
	for _, s := range cfg.Subscriptions {
		if s.Name == v || s.ID == v {
			return s.ID, nil
		}
	}
	return "", fmt.Errorf("找不到订阅: %s（可写订阅名、subs 里的编号或 manual）", v)
}

func describeGroup(cfg config.Config, g config.NodeGroup) string {
	var parts []string
	if len(g.Subscriptions) > 0 {
		names := make([]string, 0, len(g.Subscriptions))
		for _, id := range g.Subscriptions {
			name := "手动添加"
			for _, s := range cfg.Subscriptions {
				if s.ID == id {
					name = s.Name
				}
			}
			names = append(names, name)
		}
		parts = append(parts, "订阅 "+strings.Join(names, "/"))
	}
	if len(g.Regions) > 0 {
		parts = append(parts, "地区 "+strings.Join(g.Regions, "/"))
	}
	if len(g.Protocols) > 0 {
		parts = append(parts, "协议 "+strings.Join(g.Protocols, "/"))
	}
	if g.Keyword != "" {
		parts = append(parts, "名称含 "+g.Keyword)
	}
	if g.Exclude != "" {
		parts = append(parts, "排除 "+g.Exclude)
	}
	if len(parts) == 0 {
		return "全部节点"
	}
	return strings.Join(parts, "，")
}

func runGroupsCommand(application *app.App, args []string) error {
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "list" {
		if len(cfg.NodeGroups) == 0 {
			fmt.Println("暂无节点组，用 groups add <名称> [条件...] 新建")
			return nil
		}
		for i, g := range cfg.NodeGroups {
			fmt.Printf("%d\t%s\t%d 个节点\t%s\n", i+1, g.Name, len(cfg.TargetNodes(config.TargetGroup+":"+g.ID)), describeGroup(cfg, g))
		}
		return nil
	}
	switch args[0] {
	case "add":
		g, err := parseGroup(cfg, args[1], args[2:])
		if err != nil {
			return err
		}
		err = mutateConfig(application, func(cfg *config.Config) error {
			for _, existing := range cfg.NodeGroups {
				if existing.Name == g.Name {
					return errors.New("已有同名节点组")
				}
			}
			g.ID = fmt.Sprintf("g%d", len(cfg.NodeGroups)+1)
			for taken := true; taken; {
				taken = false
				for _, existing := range cfg.NodeGroups {
					if existing.ID == g.ID {
						g.ID, taken = g.ID+"x", true
					}
				}
			}
			cfg.NodeGroups = append(cfg.NodeGroups, g)
			fmt.Printf("节点组已添加，匹配 %d 个节点；规则里用 group:%s 或直接写组名\n", len(cfg.TargetNodes(config.TargetGroup+":"+g.ID)), g.ID)
			return nil
		})
		return err
	case "delete":
		index, _ := strconv.Atoi(args[1])
		err := mutateConfig(application, func(cfg *config.Config) error {
			if index > len(cfg.NodeGroups) {
				return fmt.Errorf("节点组编号超出范围: %d", index)
			}
			cfg.NodeGroups = append(cfg.NodeGroups[:index-1], cfg.NodeGroups[index:]...)
			return nil
		})
		if err == nil {
			fmt.Println("节点组已删除，用它的规则暂走代理")
		}
		return err
	}
	return nil
}
