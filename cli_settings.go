//go:build cli

package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/caichengle666/sbtun/app"
	"github.com/caichengle666/sbtun/config"
)

var ruleMatchLabels = map[string]string{
	"domain_suffix":  "域名后缀",
	"domain_keyword": "域名关键词",
	"domain":         "完整域名",
	"ip_cidr":        "IP 网段",
	"port":           "端口",
}

var ruleActionLabels = map[string]string{"proxy": "代理", "direct": "直连", "block": "阻断"}

func onOff(enabled bool) string {
	if enabled {
		return "开启"
	}
	return "关闭"
}

// mutateConfig saves a changed copy of the config through the same path the
// GUI uses, so a running instance reloads it.
func mutateConfig(application *app.App, mutate func(cfg *config.Config) error) error {
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	if err := mutate(&cfg); err != nil {
		return err
	}
	return application.SaveConfig(cfg)
}

func validateRuleListArgs(command string, args []string, withAction bool) error {
	switch {
	case len(args) == 0, len(args) == 1 && args[0] == "list":
		return nil
	case len(args) == 2 && args[0] == "delete":
		if index, err := strconv.Atoi(args[1]); err != nil || index < 1 {
			return fmt.Errorf("规则编号无效: %s", args[1])
		}
		return nil
	case args[0] == "add" && (withAction && len(args) == 4 || !withAction && len(args) == 3):
		if _, ok := ruleMatchLabels[args[1]]; !ok {
			return fmt.Errorf("匹配类型无效: %s（可选 domain_suffix、domain_keyword、domain、ip_cidr、port）", args[1])
		}
		if strings.TrimSpace(args[2]) == "" {
			return errors.New("匹配内容不能为空")
		}
		if withAction {
			if _, ok := ruleActionLabels[args[3]]; !ok {
				return fmt.Errorf("处理方式无效: %s（可选 proxy、direct、block）", args[3])
			}
		}
		return nil
	}
	return usageError(command, args)
}

func validateSetArgs(args []string) error {
	if len(args) != 2 || (args[1] != "on" && args[1] != "off") {
		return usageError("set", args)
	}
	switch args[0] {
	case "ipv6", "webrtc", "diagnostics":
		return nil
	}
	return fmt.Errorf("未知设置: %s（可选 ipv6、webrtc、diagnostics）", args[0])
}

// runRuleListCommand handles custom routing rules (custom) and DNS filter
// rules (filter); filter rules always block.
func runRuleListCommand(application *app.App, args []string, filter bool) error {
	pick := func(cfg *config.Config) *[]config.Rule {
		if filter {
			return &cfg.DNSFilterRules
		}
		return &cfg.CustomRules
	}
	if len(args) == 0 || args[0] == "list" {
		cfg, err := application.LoadConfig()
		if err != nil {
			return err
		}
		rules := *pick(&cfg)
		if len(rules) == 0 {
			fmt.Println("暂无规则")
			return nil
		}
		for i, rule := range rules {
			fmt.Printf("%d\t%s\t%s\t%s\n", i+1, ruleMatchLabels[rule.MatchType], rule.Value, ruleActionLabels[rule.Action])
		}
		return nil
	}
	switch args[0] {
	case "add":
		rule := config.Rule{MatchType: args[1], Value: strings.TrimSpace(args[2]), Action: "block"}
		if !filter {
			rule.Action = args[3]
		}
		err := mutateConfig(application, func(cfg *config.Config) error {
			rules := pick(cfg)
			for _, existing := range *rules {
				if existing.MatchType == rule.MatchType && strings.EqualFold(existing.Value, rule.Value) && existing.Action == rule.Action {
					return errors.New("这条规则已经存在")
				}
			}
			*rules = append(*rules, rule)
			return nil
		})
		if err == nil {
			fmt.Println("规则已添加")
		}
		return err
	case "delete":
		index, _ := strconv.Atoi(args[1])
		err := mutateConfig(application, func(cfg *config.Config) error {
			rules := pick(cfg)
			if index > len(*rules) {
				return fmt.Errorf("规则编号超出范围: %d", index)
			}
			*rules = append((*rules)[:index-1], (*rules)[index:]...)
			return nil
		})
		if err == nil {
			fmt.Println("规则已删除")
		}
		return err
	}
	return nil
}

func runSetCommand(application *app.App, args []string) error {
	enabled := args[1] == "on"
	names := map[string]string{"ipv6": "IPv6", "webrtc": "WebRTC 防泄漏", "diagnostics": "诊断信息"}
	err := mutateConfig(application, func(cfg *config.Config) error {
		switch args[0] {
		case "ipv6":
			cfg.IPv6Enabled = enabled
		case "webrtc":
			cfg.WebRTCProtectionEnabled = enabled
		case "diagnostics":
			cfg.DiagnosticsEnabled = enabled
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("%s已%s\n", names[args[0]], onOff(enabled))
	if args[0] == "ipv6" && enabled {
		if status := application.GetIPv6Status(); !status.Available {
			fmt.Println("提示:", status.Message)
		}
	}
	return nil
}

func runDiagCommand(application *app.App) error {
	result := application.GetDiagnostics()
	if !result.Enabled {
		return errors.New("诊断信息未开启，先运行 sbtun set diagnostics on")
	}
	selector := result.Selector
	if selector == "" {
		selector = "未读取"
	}
	fmt.Printf("当前配置节点: %s\n实际 selector: %s\n", result.CurrentNodeID, selector)
	if result.Message != "" {
		fmt.Printf("状态: %s\n", result.Message)
	}
	return nil
}

// nodeIDsOrAll resolves node numbers to IDs; no numbers means every node.
func nodeIDsOrAll(application *app.App, cfg config.Config, args []string) ([]string, error) {
	if len(args) > 0 {
		return nodeIDsFromArgs(application, args)
	}
	if len(cfg.Nodes) == 0 {
		return nil, errors.New("暂无节点，请先使用 add-node 添加")
	}
	ids := make([]string, 0, len(cfg.Nodes))
	for _, node := range cfg.Nodes {
		ids = append(ids, node.ID)
	}
	return ids, nil
}

func runExportCommand(application *app.App, args []string) error {
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	ids, err := nodeIDsOrAll(application, cfg, args)
	if err != nil {
		return err
	}
	links, err := application.ExportNodes(ids)
	if err != nil {
		return err
	}
	for _, link := range links {
		fmt.Println(link)
	}
	return nil
}

func formatCheck(label string, check app.HealthCheckDTO) string {
	if check.Message == "" && !check.OK {
		return label + " -"
	}
	if !check.OK {
		return label + " 失败"
	}
	return fmt.Sprintf("%s %dms", label, check.Latency)
}

func runTestCommand(application *app.App, args []string) error {
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	ids, err := nodeIDsOrAll(application, cfg, args)
	if err != nil {
		return err
	}
	index := make(map[string]int, len(cfg.Nodes))
	for i, node := range cfg.Nodes {
		index[node.ID] = i
	}
	failed := 0
	for _, result := range application.TestNodes(ids) {
		i := index[result.NodeID]
		mark := "✓"
		if !result.Healthy {
			mark = "✗"
			failed++
		}
		fmt.Printf("%s %d\t%s\t%s\t%s\t%s\t%s\n", mark, i+1, cfg.Nodes[i].Name,
			formatCheck("Ping", result.Ping), formatCheck("端口", result.Port), formatCheck("URL", result.URL), shortMessage(result.Message))
	}
	fmt.Printf("共 %d 个，可用 %d 个\n", len(ids), len(ids)-failed)
	return nil
}

var singBoxLogStamp = regexp.MustCompile(`\s[+-]\d{4} \d{4}-\d{2}-\d{2} `)

// shortMessage keeps the first line of a test message, capped at 60 runes;
// sing-box log lines appended to failures are dropped.
func shortMessage(message string) string {
	message, _, _ = strings.Cut(strings.TrimSpace(message), "\n")
	if loc := singBoxLogStamp.FindStringIndex(message); loc != nil {
		message = strings.TrimSpace(message[:loc[0]])
	}
	if runes := []rune(message); len(runes) > 60 {
		message = string(runes[:60]) + "…"
	}
	return message
}
