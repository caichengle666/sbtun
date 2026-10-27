//go:build cli

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/caichengle666/sbtun/app"
	"github.com/caichengle666/sbtun/config"
)

func main() {
	command, commandArgs, ok := parseCLIArgs(os.Args)
	if !ok {
		printHelp()
		return
	}
	if command == "help" || command == "-h" || command == "--help" {
		printHelp()
		return
	}
	if command == "version" || command == "-v" || command == "--version" {
		fmt.Println("sbtun " + app.Version())
		printAppUpdate()
		return
	}
	if hasHelpArg(commandArgs) && printCommandHelp(command, commandArgs) {
		return
	}
	if err := validateCLIArgs(command, commandArgs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if command == "edit" && len(commandArgs) <= 1 && !isTerminalInput(os.Stdin) {
		fmt.Fprintln(os.Stderr, "edit 交互模式需要交互式终端；请直接在终端运行，或使用完整参数形式")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), cliSignals()...)
	defer stop()
	application := app.New()
	application.StartupCLI(ctx)
	var err error
	captureRun := command == "capture" && len(commandArgs) > 0 && commandArgs[0] == "run"
	if requiresElevation(command, commandArgs) {
		if err := relaunchElevated(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if captureRun {
		if err := enableCapture(application, commandArgs[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	if command == "run" || command == "start" || captureRun {
		application.StartMonitoring()
	}
	if command == "run" || command == "start" || captureRun {
		release, err := app.AcquireSingleInstance()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer release()
		if err := application.ClearStopRequest(); err != nil {
			fmt.Fprintln(os.Stderr, "清理旧停止请求失败:", err)
			os.Exit(1)
		}
		if err := application.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("sbtun 已启动，按 Ctrl+C 停止")
		if captureRun {
			fmt.Println("抓包分析器已启动，等待流量...")
		}
		stopRequested := make(chan struct{})
		go func() {
			if application.WaitForStopRequest(ctx) {
				close(stopRequested)
			}
		}()
		select {
		case <-ctx.Done():
		case <-stopRequested:
		}
		if application.GetConfig().CaptureEnabled {
			if saveErr := application.SaveCaptureFlows(); saveErr != nil {
				fmt.Fprintln(os.Stderr, "保存抓包失败:", saveErr)
			}
		}
		if err := application.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	switch command {
	case "status":
		cfg, loadErr := application.LoadConfig()
		if loadErr != nil {
			err = fmt.Errorf("读取配置失败: %w", loadErr)
			break
		}
		status := application.GetStatus()
		currentID := cfg.CurrentNodeID
		if status.CurrentNodeID != "" {
			currentID = status.CurrentNodeID
		}
		running := "未运行"
		if status.Running {
			running = "运行中"
		}
		fmt.Printf("状态: %s\n路由模式: %s\nDNS 模式: %s\n当前节点: %s\n节点数量: %d\n", running, cfg.RoutingMode, cfg.DNSMode, nodeLabel(cfg.Nodes, currentID), len(cfg.Nodes))
		if status.Selector != "" {
			fmt.Printf("运行时 selector: %s\n", status.Selector)
		}
		autoSwitch := "开启"
		if cfg.AutoSwitchDisabled {
			autoSwitch = "关闭"
		}
		fmt.Printf("自动切换: %s\nIPv6: %s\nWebRTC 防泄漏: %s\n诊断信息: %s\n", autoSwitch, onOff(cfg.IPv6Enabled), onOff(cfg.WebRTCProtectionEnabled), onOff(cfg.DiagnosticsEnabled))
		if cfg.WebDAV != nil {
			fmt.Printf("WebDAV 备份: %s（自动备份%s）\n", cfg.WebDAV.URL, onOff(cfg.WebDAV.AutoBackup))
		}
	case "auto-switch":
		enabled := commandArgs[0] == "on"
		err = application.SetAutoSwitch(enabled)
		if err == nil && enabled {
			fmt.Println("已开启自动切换：当前节点失效时切换到延迟最低的可用节点")
		} else if err == nil {
			fmt.Println("已关闭自动切换")
		}
	case "route":
		err = application.SetRoutingMode(config.RoutingMode(commandArgs[0]))
		if err == nil {
			fmt.Println("路由模式已更新")
		}
	case "add-node":
		var count int
		count, err = application.ImportSubscription(commandArgs[0])
		if err == nil {
			if count == 0 {
				fmt.Println("未添加新节点：节点已存在")
			} else {
				fmt.Printf("已添加 %d 个节点\n", count)
			}
		}
	case "nodes":
		err = listNodes(application)
	case "del", "delete":
		ids, parseErr := nodeIDsFromArgs(application, commandArgs)
		if parseErr != nil {
			err = parseErr
			break
		}
		err = application.RemoveNodes(ids)
		if err == nil {
			fmt.Printf("已删除 %d 个节点\n", len(ids))
		}
	case "test":
		err = runTestCommand(application, commandArgs)
	case "export":
		err = runExportCommand(application, commandArgs)
	case "qr":
		err = runQRCommand(application, commandArgs)
	case "custom":
		err = runRuleListCommand(application, commandArgs, false)
	case "filter":
		err = runRuleListCommand(application, commandArgs, true)
	case "groups":
		err = runGroupsCommand(application, commandArgs)
	case "set":
		err = runSetCommand(application, commandArgs)
	case "backup":
		err = runBackupCommand(application, commandArgs)
	case "diag":
		err = runDiagCommand(application)
	case "info":
		err = showNodeInfo(application, commandArgs[0])
	case "edit":
		if len(commandArgs) == 0 {
			err = interactiveEdit(application, "")
			break
		}
		if len(commandArgs) == 1 {
			err = interactiveEdit(application, commandArgs[0])
			break
		}
		if len(commandArgs) == 2 {
			index, _ := strconv.Atoi(commandArgs[0])
			cfg, loadErr := application.LoadConfig()
			if loadErr != nil {
				err = loadErr
				break
			}
			if index > len(cfg.Nodes) {
				err = fmt.Errorf("节点编号超出范围: %d", index)
				break
			}
			err = application.UpdateNodeFromLink(cfg.Nodes[index-1].ID, commandArgs[1])
			if err == nil {
				fmt.Println("节点全部参数已更新")
			}
			break
		}
		index, _ := strconv.Atoi(commandArgs[0])
		port, _ := strconv.Atoi(commandArgs[3])
		cfg, loadErr := application.LoadConfig()
		if loadErr != nil {
			err = loadErr
			break
		}
		if index > len(cfg.Nodes) {
			err = fmt.Errorf("节点编号或节点信息无效")
			break
		}
		node := cfg.Nodes[index-1]
		node.Name, node.Server, node.Port = strings.TrimSpace(commandArgs[1]), strings.TrimSpace(commandArgs[2]), uint16(port)
		err = application.UpdateNode(node.ID, node)
		if err == nil {
			fmt.Println("节点信息已更新")
		}
	case "switch":
		err = switchNodeByIndex(application, commandArgs[0])
		if err == nil {
			fmt.Println("节点已切换")
		}
	case "add-rule":
		var result app.RuleImportResult
		result, err = application.ImportCustomRules(commandArgs[0])
		if err == nil {
			fmt.Println(describeRuleImport(result))
		}
	case "stop":
		err = stopRunningInstance()
		if err == nil {
			fmt.Println("sbtun 已停止")
		}
	case "rules":
		err = runRulesCommand(application, commandArgs)
	case "subs":
		err = runSubsCommand(application, commandArgs)
	case "capture":
		err = runCaptureCommand(application, commandArgs)
	case "kernel":
		err = runKernelCommand(application, commandArgs)
	case "upgrade":
		err = runUpgradeCommand(application)
	case "conns":
		err = runConnsCommand(application, commandArgs)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func parseCLIArgs(args []string) (string, []string, bool) {
	if len(args) < 2 {
		return "", nil, false
	}
	return args[1], args[2:], true
}

func hasHelpArg(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

// cliUsage is the single source of every command's usage line, shared by
// `<command> -h` and argument validation errors.
func cliUsage(command string, args []string) string {
	switch command {
	case "run", "start", "stop", "status", "nodes", "diag", "upgrade":
		return "sbtun " + command
	case "route":
		return "sbtun route <smart|global|direct>"
	case "auto-switch":
		return "sbtun auto-switch <on|off>"
	case "add-node":
		return "sbtun add-node <节点链接或订阅链接>"
	case "switch", "info", "qr":
		return "sbtun " + command + " <编号>"
	case "del", "delete":
		return "sbtun " + command + " <编号...>"
	case "test", "export":
		return "sbtun " + command + " [编号...]"
	case "custom":
		return "sbtun custom [list|add <类型> <内容> <去向>|edit <编号> <类型> <内容> <去向>|delete <编号>]"
	case "groups":
		return "sbtun groups [list|add <名称> [region=US,JP] [sub=订阅] [proto=hy2,vless] [keyword=词] [exclude=词]|delete <编号>]"
	case "filter":
		return "sbtun filter [list|add <类型> <内容>|edit <编号> <类型> <内容>|delete <编号>]"
	case "set":
		return "sbtun set <ipv6|webrtc|diagnostics> <on|off>"
	case "edit":
		return "sbtun edit [编号]\n  sbtun edit <编号> <节点链接>\n  sbtun edit <编号> <名称> <服务器> <端口>"
	case "add-rule":
		return "sbtun add-rule <链接|文件|文本>"
	case "rules":
		return "sbtun rules [list|add <名称> <链接> [去向]|edit <ID> <名称> <链接> [去向]|action <ID> <去向>|delete <ID>|enable <ID>|disable <ID>|restore|update <ID>|update-all]"
	case "subs":
		return "sbtun subs [list|update [编号]|delete <编号>|auto <小时|off>]"
	case "conns":
		return "sbtun conns [关键词|test <域名>|close <编号|all>]"
	case "capture":
		if len(args) > 0 {
			switch args[0] {
			case "cert":
				return certificateCLIUsage()
			case "enable":
				return "sbtun capture enable <域名、关键词或 *>"
			case "show":
				return "sbtun capture show <ID>"
			case "disable", "status", "list", "clear":
				return "sbtun capture " + args[0]
			}
		}
		return "sbtun capture <run|enable|disable|status|list|show|clear|cert>"
	case "kernel":
		return "sbtun kernel <version|update>"
	case "backup":
		return "sbtun backup [list|restore <编号>|delete <编号>|set <WebDAV地址> <用户名> <密码> [加密密码]|auto <on|off>|clear]"
	}
	return ""
}

func usageError(command string, args []string) error {
	return fmt.Errorf("用法: %s", cliUsage(command, args))
}

func printCommandHelp(command string, args []string) bool {
	usage := cliUsage(command, args)
	if usage == "" {
		return false
	}
	fmt.Println("用法:", usage)
	return true
}

func validateCLIArgs(command string, args []string) error {
	requireNoArgs := func() error {
		if len(args) != 0 {
			return usageError(command, args)
		}
		return nil
	}
	requireNodeIndices := func() error {
		if len(args) == 0 {
			return usageError(command, args)
		}
		for _, arg := range args {
			index, err := strconv.Atoi(arg)
			if err != nil || index < 1 {
				return fmt.Errorf("节点编号无效: %s", arg)
			}
		}
		return nil
	}

	switch command {
	case "run", "start", "stop", "status", "nodes", "diag", "upgrade":
		return requireNoArgs()
	case "auto-switch":
		if len(args) != 1 || (args[0] != "on" && args[0] != "off") {
			return usageError(command, args)
		}
		return nil
	case "route":
		if len(args) != 1 {
			return usageError(command, args)
		}
		switch config.RoutingMode(args[0]) {
		case config.RoutingSmart, config.RoutingGlobal, config.RoutingDirect:
			return nil
		case "custom":
			return errors.New("自定义模式已取消：自己的分流规则和规则集在 smart、global 下都会优先生效，请用 sbtun route global")
		default:
			return fmt.Errorf("无效路由模式: %s", args[0])
		}
	case "add-node":
		if len(args) != 1 {
			return usageError(command, args)
		}
		return app.ValidateNodeSource(args[0])
	case "switch", "info", "qr":
		if len(args) != 1 {
			return usageError(command, args)
		}
		return requireNodeIndices()
	case "del", "delete":
		return requireNodeIndices()
	case "test", "export":
		if len(args) == 0 {
			return nil
		}
		return requireNodeIndices()
	case "custom":
		return validateRuleListArgs(command, args, true)
	case "groups":
		return validateGroupsArgs(args)
	case "filter":
		return validateRuleListArgs(command, args, false)
	case "set":
		return validateSetArgs(args)
	case "edit":
		if len(args) != 0 && len(args) != 1 && len(args) != 2 && len(args) != 4 {
			return usageError(command, args)
		}
		if len(args) > 0 {
			index, err := strconv.Atoi(args[0])
			if err != nil || index < 1 {
				return fmt.Errorf("节点编号无效: %s", args[0])
			}
		}
		if len(args) == 2 && strings.TrimSpace(args[1]) == "" {
			return errors.New("节点链接不能为空")
		}
		if len(args) == 4 {
			port, err := strconv.Atoi(args[3])
			if strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[2]) == "" || err != nil || port < 1 || port > 65535 {
				return errors.New("节点名称、服务器或端口无效")
			}
		}
		return nil
	case "add-rule":
		if len(args) != 1 {
			return usageError(command, args)
		}
		return nil
	case "rules":
		switch {
		case len(args) == 0, len(args) == 1 && (args[0] == "list" || args[0] == "update-all"):
			return nil
		case len(args) == 1 && args[0] == "restore":
			return nil
		case len(args) == 2 && (args[0] == "update" || args[0] == "enable" || args[0] == "disable") && strings.TrimSpace(args[1]) != "":
			return nil
		case (len(args) == 3 || len(args) == 4) && args[0] == "add" && strings.TrimSpace(args[1]) != "" && strings.TrimSpace(args[2]) != "":
			return nil
		case (len(args) == 4 || len(args) == 5) && args[0] == "edit" && strings.TrimSpace(args[1]) != "" && strings.TrimSpace(args[2]) != "" && strings.TrimSpace(args[3]) != "":
			return nil
		case len(args) == 3 && args[0] == "action" && strings.TrimSpace(args[1]) != "" && strings.TrimSpace(args[2]) != "":
			return nil
		case len(args) == 2 && args[0] == "delete" && strings.TrimSpace(args[1]) != "":
			return nil
		default:
			return usageError(command, args)
		}
	case "subs":
		switch {
		case len(args) == 0, len(args) == 1 && (args[0] == "list" || args[0] == "update"):
			return nil
		case len(args) == 2 && (args[0] == "update" || args[0] == "delete"):
			if index, err := strconv.Atoi(args[1]); err != nil || index < 1 {
				return fmt.Errorf("订阅编号无效: %s", args[1])
			}
			return nil
		case len(args) == 2 && args[0] == "auto":
			if args[1] == "off" {
				return nil
			}
			if hours, err := strconv.Atoi(args[1]); err != nil || hours < 1 || hours > 720 {
				return fmt.Errorf("自动更新间隔应为 1 到 720 小时，或 off: %s", args[1])
			}
			return nil
		default:
			return usageError(command, args)
		}
	case "capture":
		return validateCaptureArgs(args)
	case "conns":
		return validateConnsArgs(args)
	case "kernel":
		return validateKernelArgs(args)
	case "backup":
		return validateBackupArgs(args)
	default:
		return fmt.Errorf("未知命令: %s", command)
	}
}

func validateKernelArgs(args []string) error {
	if len(args) != 1 {
		return usageError("kernel", args)
	}
	switch args[0] {
	case "version", "update":
		return nil
	default:
		return fmt.Errorf("未知内核命令: %s", args[0])
	}
}

func validateCaptureArgs(args []string) error {
	if len(args) == 0 {
		return usageError("capture", args)
	}
	switch args[0] {
	case "run":
		return nil
	case "enable":
		if len(args) < 2 {
			return usageError("capture", args)
		}
		return nil
	case "disable", "status", "list", "clear":
		if len(args) != 1 {
			return usageError("capture", args)
		}
		return nil
	case "show":
		if len(args) != 2 {
			return usageError("capture", args)
		}
		id, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil || id == 0 {
			return fmt.Errorf("请求 ID 无效: %s", args[1])
		}
		return nil
	case "cert":
		if len(args) < 2 || len(args) > 3 || args[1] != "install" && args[1] != "uninstall" {
			return usageError("capture", args)
		}
		if len(args) == 3 {
			if runtime.GOOS != "windows" {
				return errors.New("当前系统只支持系统级证书安装，不需要指定 user")
			}
			if args[2] != "user" && args[2] != "system" {
				return errors.New("证书级别必须是 user 或 system")
			}
		}
		return nil
	default:
		return fmt.Errorf("未知抓包命令: %s", args[0])
	}
}

func requiresElevation(command string, args []string) bool {
	switch command {
	case "run", "start", "stop", "route", "auto-switch", "add-node", "add-rule", "switch", "del", "delete", "edit", "set", "upgrade":
		return true
	case "custom", "filter", "groups":
		return len(args) > 0 && (args[0] == "add" || args[0] == "delete")
	case "capture":
		if len(args) == 0 {
			return false
		}
		switch args[0] {
		case "run", "enable", "disable", "clear", "cert":
			return true
		}
	default:
		return false
	}
	return false
}

func printHelp() {
	fmt.Println("sbtun 命令行\n\n用法:\n  sbtun <命令> [参数]")
	printHelpGroup("运行", []helpEntry{
		{"run", "前台启动 TUN 和代理"},
		{"start", "同 run"},
		{"stop", "停止运行中的实例"},
		{"status", "查看配置状态"},
	})
	printHelpGroup("节点", []helpEntry{
		{"nodes", "列出节点编号，* 表示当前节点"},
		{"info <编号>", "查看节点协议和完整参数"},
		{"add-node <链接>", "添加单节点链接或订阅链接"},
		{"switch <编号>", "按编号切换节点"},
		{"test [编号...]", "测试节点 Ping、端口和 URL 延迟，省略编号则测试全部，最多 4 个并行"},
		{"export [编号...]", "导出节点分享链接，省略编号则导出全部"},
		{"qr <编号>", "在终端显示节点分享链接二维码"},
		{"del <编号...>", "批量删除节点，例如 del 1 2 3"},
		{"edit [编号]", "进入交互式编辑，可直接指定节点"},
		{"edit <编号> <节点链接>", "用完整链接覆盖全部参数"},
		{"edit <编号> <名称> <服务器> <端口>", "仅修改基本信息"},
		{"subs", "列出订阅编号"},
		{"subs update [编号]", "更新指定订阅，省略编号则更新全部"},
		{"subs delete <编号>", "删除订阅及其全部节点"},
		{"subs auto <小时|off>", "设置订阅自动更新间隔（默认 24 小时），off 关闭"},
	})
	printHelpGroup("路由与规则", []helpEntry{
		{"route <模式>", "设置路由: smart（国内直连）/global（全部代理）/direct（全部直连）；自己的规则在 smart、global 下优先，direct 下只保留屏蔽"},
		{"auto-switch <on|off>", "开启或关闭节点失效自动切换"},
		{"conns [关键词]", "查看当前连接：网站、命中的规则、实际出口"},
		{"conns test <域名>", "实际连一次，看这个网站命中哪条规则、走哪个出口"},
		{"conns close <编号|all>", "断开指定连接或全部连接"},
		{"set ipv6 <on|off>", "开启或关闭 IPv6"},
		{"set webrtc <on|off>", "开启或关闭 WebRTC 防泄漏"},
		{"set diagnostics <on|off>", "开启或关闭诊断信息"},
		{"diag", "查看运行诊断（需先开启诊断信息）"},
		{"add-rule <链接|文件|文本>", "批量导入分流规则；引用的 RULE-SET 列表各自成为规则集"},
		{"custom", "列出分流规则"},
		{"custom add <类型> <内容> <去向>", "添加分流规则，类型 domain_suffix/domain_keyword/domain/ip_cidr/port；去向 proxy/direct/block，或节点组名、节点名、订阅名、协议(hy2)、地区(美国/US)"},
		{"custom edit <编号> <类型> <内容> <去向>", "修改分流规则，参数同 add"},
		{"custom delete <编号>", "删除分流规则"},
		{"groups", "列出节点组（规则可以直接走一个组，组内自动用最快的）"},
		{"groups add <名称> [条件...]", "新建节点组，条件 region=US,JP sub=订阅名 proto=hy2,vless keyword=IPLC exclude=过期，不写的不限"},
		{"groups delete <编号>", "删除节点组"},
		{"filter", "列出 DNS 过滤规则"},
		{"filter add <类型> <内容>", "添加 DNS 过滤（阻断）规则"},
		{"filter edit <编号> <类型> <内容>", "修改 DNS 过滤规则"},
		{"filter delete <编号>", "删除 DNS 过滤规则"},
		{"rules list", "列出规则集和去向"},
		{"rules add <名称> <链接> [去向]", "添加规则集：.srs、Clash/Surge 规则列表或域名列表；去向默认 block（屏蔽广告），也可写 proxy/direct、节点组名、地区等"},
		{"rules edit <ID> <名称> <链接> [去向]", "编辑规则集，不写去向则不变"},
		{"rules action <ID> <去向>", "只修改规则集的去向"},
		{"rules delete <ID>", "删除规则集"},
		{"rules enable|disable <ID>", "启用或停用规则集"},
		{"rules restore", "恢复默认规则集"},
		{"rules update <id>", "更新指定规则集"},
		{"rules update-all", "更新全部规则集"},
	})
	printHelpGroup("流量分析", []helpEntry{
		{"capture run [规则...]", "启用抓包并前台启动 TUN，* 表示全部 HTTP/HTTPS"},
		{"capture enable <规则...>", "保存抓包规则，* 表示全部 HTTP/HTTPS"},
		{"capture disable", "关闭抓包，下次启动生效"},
		{"capture status", "查看抓包配置和运行状态"},
		{"capture list", "列出已保存请求"},
		{"capture show <ID>", "查看一条已保存请求的完整内容"},
		{"capture clear", "删除已保存请求"},
		{"capture cert <install|uninstall> [user|system]", "安装或卸载抓包根证书，默认系统级"},
	})
	printHelpGroup("云端备份（WebDAV）", []helpEntry{
		{"backup set <地址> <用户名> <密码> [加密密码]", "设置 WebDAV，例如坚果云 https://dav.jianguoyun.com/dav/sbtun/"},
		{"backup", "立即备份手动节点、订阅地址、规则和设置（订阅节点不备份）"},
		{"backup list", "列出云端备份，保留最近 10 份"},
		{"backup restore <编号>", "用云端备份整体覆盖本机，订阅节点重新拉取"},
		{"backup delete <编号>", "删除一份云端备份"},
		{"backup auto <on|off>", "开启后内容有变化时自动备份"},
		{"backup clear", "清除 WebDAV 设置"},
	})
	printHelpGroup("其他", []helpEntry{
		{"kernel version", "查看当前 sing-box 内核版本"},
		{"kernel update", "更新到 sing-box 官方最新稳定版"},
		{"upgrade", "更新 sbtun 程序本体到最新版"},
		{"help", "显示帮助"},
		{"version", "显示版本并检查是否有新版本"},
	})
	fmt.Println(`
示例:
  sbtun nodes
  sbtun switch 2
  sbtun del 1 2 3
  sbtun test
  sbtun custom add domain_suffix example.com direct
  sbtun groups add "美国 hy2" region=US proto=hy2
  sbtun custom add domain_suffix openai.com "美国 hy2"
  sbtun conns test openai.com
  sbtun edit 2
  sbtun edit 2 "vless://UUID@1.2.3.4:443?..."

说明:
  交互编辑中直接按回车保留当前值，输入 - 清空当前参数。
  名称包含空格时请使用引号，例如: sbtun edit 2 "韩国 高速" 1.2.3.4 443
  批量测试最多 4 个并行，不会同时启动大量检测进程。
  删除当前节点后会自动选择剩余节点。
  运行中修改 route 或 rules 会自动重载 sing-box。
  匹配顺序：DNS 过滤（屏蔽）→ 分流规则 → 自己添加的规则集 → 内置国内/国外规则（仅 smart）→ 兜底。
  停止前台运行请按 Ctrl+C。`)
}

type helpEntry struct {
	command     string
	description string
}

func printHelpGroup(title string, entries []helpEntry) {
	maxWidth := 0
	for _, entry := range entries {
		if width := terminalTextWidth(entry.command); width > maxWidth {
			maxWidth = width
		}
	}
	fmt.Printf("\n%s:\n", title)
	for _, entry := range entries {
		padding := strings.Repeat(" ", maxWidth-terminalTextWidth(entry.command))
		fmt.Printf("  sbtun %s%s  %s\n", entry.command, padding, entry.description)
	}
}

func terminalTextWidth(value string) int {
	width := 0
	for _, char := range value {
		if char <= 0x7f {
			width++
		} else {
			width += 2
		}
	}
	return width
}

func runSubsCommand(application *app.App, args []string) error {
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	subs := cfg.Subscriptions
	pick := func(arg string) (config.Subscription, error) {
		index, _ := strconv.Atoi(arg)
		if index < 1 || index > len(subs) {
			return config.Subscription{}, fmt.Errorf("订阅编号超出范围: %s", arg)
		}
		return subs[index-1], nil
	}
	printResult := func(r app.SubscriptionUpdateDTO) {
		fmt.Printf("%s: 共 %d 个节点，新增 %d，移除 %d\n", r.Name, r.Total, r.Added, r.Removed)
	}
	if len(args) == 0 || args[0] == "list" {
		if len(subs) == 0 {
			fmt.Println("暂无订阅，使用 sbtun add-node <订阅地址> 导入")
			return nil
		}
		if hours := app.SubscriptionUpdateHours(cfg); hours > 0 {
			fmt.Printf("自动更新: 每 %d 小时（sbtun run 运行期间）\n", hours)
		} else {
			fmt.Println("自动更新: 关闭")
		}
		for i, sub := range subs {
			count := 0
			for _, n := range cfg.Nodes {
				if n.Subscription == sub.ID {
					count++
				}
			}
			updated := "尚未更新"
			if t, err := time.Parse(time.RFC3339, sub.UpdatedAt); err == nil {
				updated = "更新于 " + t.Local().Format("2006-01-02 15:04")
			}
			line := fmt.Sprintf("%d\t%s\t%d 个节点\t%s", i+1, sub.Name, count, updated)
			if usage, warn := app.SubscriptionUsageText(sub.Usage, time.Now()); usage != "" {
				if warn {
					usage = "⚠ " + usage
				}
				line += "\t" + usage
			}
			fmt.Println(line)
		}
		return nil
	}
	switch args[0] {
	case "update":
		if len(args) == 2 {
			sub, err := pick(args[1])
			if err != nil {
				return err
			}
			result, err := application.UpdateSubscription(sub.ID)
			if err != nil {
				return err
			}
			printResult(result)
			return nil
		}
		if len(subs) == 0 {
			return errors.New("暂无订阅")
		}
		failed := 0
		for _, result := range application.UpdateAllSubscriptions() {
			if result.Error != "" {
				failed++
				fmt.Fprintf(os.Stderr, "%s: %s\n", result.Name, result.Error)
				continue
			}
			printResult(result)
		}
		if failed > 0 {
			return fmt.Errorf("%d 个订阅更新失败", failed)
		}
		return nil
	case "auto":
		hours := 0
		if args[1] != "off" {
			hours, _ = strconv.Atoi(args[1])
		}
		if err := application.SetSubscriptionAutoUpdate(hours); err != nil {
			return err
		}
		if hours > 0 {
			fmt.Printf("订阅将每 %d 小时自动更新\n", hours)
		} else {
			fmt.Println("已关闭订阅自动更新")
		}
		return nil
	case "delete":
		sub, err := pick(args[1])
		if err != nil {
			return err
		}
		if err := application.RemoveSubscription(sub.ID); err != nil {
			return err
		}
		fmt.Println("订阅已删除")
		return nil
	}
	return usageError("subs", args)
}

func runRulesCommand(application *app.App, args []string) error {
	cfg := application.GetConfig()
	if len(args) == 0 || args[0] == "list" {
		for _, rule := range application.ListRules() {
			state := "已启用"
			if !rule.Enabled {
				state = "已停用"
			}
			if !rule.Exists {
				state += "，文件缺失"
			}
			target := "内置"
			if rule.Source != "default" {
				target = app.DescribeAction(cfg, rule.Action)
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", rule.ID, rule.Name, target, state)
		}
		return nil
	}
	target := func(i int) (string, error) {
		if len(args) <= i {
			return "", nil
		}
		return cliRuleTarget(cfg, args[i])
	}
	switch args[0] {
	case "add":
		action, err := target(3)
		if err != nil {
			return err
		}
		if err := application.AddRuleSet(args[1], args[2], action); err != nil {
			return err
		}
		fmt.Println("规则集已添加")
		return nil
	case "edit":
		action, err := target(4)
		if err != nil {
			return err
		}
		if action == "" {
			for _, rule := range application.ListRules() {
				if rule.ID == args[1] {
					action = rule.Action
				}
			}
		}
		if err := application.EditRuleSet(args[1], args[2], args[3], action); err != nil {
			return err
		}
		fmt.Println("规则集已修改")
		return nil
	case "action":
		action, err := target(2)
		if err != nil {
			return err
		}
		if err := application.SetRuleSetAction(args[1], action); err != nil {
			return err
		}
		fmt.Println("规则集去向已改为", app.DescribeAction(cfg, action))
		return nil
	case "delete":
		if err := application.DeleteRuleSet(args[1]); err != nil {
			return err
		}
		fmt.Println("规则集已删除")
		return nil
	case "update":
		if err := application.UpdateRule(args[1]); err != nil {
			return err
		}
		fmt.Println("规则集更新成功")
		return nil
	case "enable", "disable":
		if err := application.SetRuleEnabled(args[1], args[0] == "enable"); err != nil {
			return err
		}
		fmt.Printf("规则集已%s\n", map[string]string{"enable": "启用", "disable": "停用"}[args[0]])
		return nil
	case "restore":
		if err := application.RestoreDefaultRuleSets(); err != nil {
			return err
		}
		fmt.Println("已恢复默认规则集")
		return nil
	case "update-all":
		errs := application.UpdateAllRules()
		if len(errs) > 0 {
			return fmt.Errorf("规则更新失败: %v", errs)
		}
		fmt.Println("全部规则集更新成功")
		return nil
	default:
		return usageError("rules", args)
	}
}

func runKernelCommand(application *app.App, args []string) error {
	switch args[0] {
	case "version":
		version, err := application.SingBoxVersion()
		if err != nil {
			return err
		}
		fmt.Println(version)
		return nil
	case "update":
		if status := application.GetStatus(); status.Running {
			return errors.New("检测到 sing-box 正在运行，请先执行 sbtun stop 再更新内核")
		}
		result, err := application.UpdateSingBox()
		if err != nil {
			return err
		}
		fmt.Println(result.Message)
		return nil
	default:
		return usageError("kernel", args)
	}
}

func runUpgradeCommand(application *app.App) error {
	fmt.Println("正在下载新版本…")
	result, err := application.UpdateApp()
	if err != nil {
		return err
	}
	if !result.Updated {
		fmt.Printf("已是最新版本 v%s\n", result.CurrentVersion)
		return nil
	}
	fmt.Printf("%s。正在运行的 sbtun（界面或 sbtun run）重新打开后生效\n", result.Message)
	return nil
}

func runCaptureCommand(application *app.App, args []string) error {
	switch args[0] {
	case "run":
		return nil
	case "enable":
		if err := enableCapture(application, args[1:]); err != nil {
			return err
		}
		fmt.Println("抓包已启用；运行 sbtun capture run 可立即启动")
		return nil
	case "disable":
		cfg, err := application.LoadConfig()
		if err != nil {
			return err
		}
		cfg.CaptureEnabled = false
		if err := application.SaveConfig(cfg); err != nil {
			return err
		}
		fmt.Println("抓包已关闭，下次启动生效")
		return nil
	case "status":
		cfg, err := application.LoadConfig()
		if err != nil {
			return err
		}
		status := application.GetCaptureStatus()
		rules := strings.Join(cfg.CaptureDomains, ", ")
		for _, rule := range cfg.CaptureDomains {
			if rule == "*" {
				rules = "*（全部 HTTP/HTTPS）"
				break
			}
		}
		certificateLevel := status.CertificateLevel
		if certificateLevel == "" {
			certificateLevel = "未安装"
		}
		fmt.Printf("启用: %t\n运行: %t\n抓包规则: %s\n已保存请求: %d\n证书级别: %s\n存储文件: %s\n",
			cfg.CaptureEnabled, status.Running, rules, status.FlowCount,
			certificateLevel, status.StoragePath)
		if status.Message != "" {
			fmt.Printf("状态信息: %s\n", status.Message)
		}
		return nil
	case "list":
		flows, err := application.GetCaptureFlows()
		if err != nil {
			return err
		}
		if len(flows) == 0 {
			fmt.Println("暂无已保存请求")
			return nil
		}
		for _, flow := range flows {
			fmt.Printf("%d\t%s\t%d\t%s\n", flow.ID, flow.Method, flow.StatusCode, flow.URL)
		}
		return nil
	case "show":
		id, _ := strconv.ParseUint(args[1], 10, 64)
		flow, err := application.GetCaptureFlow(id)
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(flow, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	case "clear":
		if err := application.ClearCaptureFlows(); err != nil {
			return err
		}
		fmt.Println("已删除保存的抓包记录")
		return nil
	case "cert":
		level := "system"
		if len(args) >= 3 {
			level = args[2]
		}
		switch args[1] {
		case "install":
			if err := application.InstallCaptureCertificate(level); err != nil {
				return err
			}
			fmt.Printf("抓包根证书已安装（%s）\n", level)
			return nil
		case "uninstall":
			if err := application.UninstallCaptureCertificate(level); err != nil {
				return err
			}
			fmt.Printf("抓包根证书已卸载（%s）\n", level)
			return nil
		default:
			return usageError("capture", args)
		}
	default:
		return fmt.Errorf("未知抓包命令: %s", args[0])
	}
}

func enableCapture(application *app.App, patterns []string) error {
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	if normalized := normalizeCapturePatterns(patterns); len(normalized) > 0 {
		cfg.CaptureDomains = normalized
	}
	if len(cfg.CaptureDomains) == 0 {
		return fmt.Errorf("至少需要一个抓包域名或关键词")
	}
	cfg.CaptureEnabled = true
	return application.SaveConfig(cfg)
}

func normalizeCapturePatterns(values []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(part)), ".")
			if part == "" {
				continue
			}
			if _, exists := seen[part]; exists {
				continue
			}
			seen[part] = struct{}{}
			result = append(result, part)
		}
	}
	return result
}

// listNodes prints the current node list with 1-based indices.
func listNodes(application *app.App) error {
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	if len(cfg.Nodes) == 0 {
		fmt.Println("暂无节点，请先使用 add-node 添加")
		return nil
	}
	currentID := cfg.CurrentNodeID
	if status := application.GetStatus(); status.CurrentNodeID != "" {
		currentID = status.CurrentNodeID
	}
	subNames := make(map[string]string, len(cfg.Subscriptions))
	for _, sub := range cfg.Subscriptions {
		subNames[sub.ID] = sub.Name
	}
	for i, node := range cfg.Nodes {
		marker := " "
		if node.ID == currentID {
			marker = "*"
		}
		source := "手动添加"
		if name, ok := subNames[node.Subscription]; ok {
			source = name
		}
		region := node.Country
		if region == "" {
			region = "--"
		} else if node.ExitIP == "" {
			region += "?" // 按名字猜的，测速后会查落地
		}
		fmt.Printf("%s %d\t%s\t%s\t%s\t%s:%d\t%s\n", marker, i+1, node.Name, region, node.Protocol, node.Server, node.Port, source)
	}
	fmt.Printf("当前节点: %s\n", nodeLabel(cfg.Nodes, currentID))
	return nil
}

// nodeLabel 把节点 ID 显示成“编号 名称”，找不到时原样返回。
func nodeLabel(nodes []config.Node, id string) string {
	for i, node := range nodes {
		if node.ID == id {
			return fmt.Sprintf("%d %s", i+1, node.Name)
		}
	}
	if id == "" {
		return "--"
	}
	return id
}

func certificateCLIUsage() string {
	if runtime.GOOS == "windows" {
		return "sbtun capture cert <install|uninstall> [user|system]"
	}
	return "sbtun capture cert <install|uninstall>"
}

func showNodeInfo(application *app.App, indexArg string) error {
	index, err := strconv.Atoi(indexArg)
	if err != nil || index < 1 {
		return fmt.Errorf("节点编号无效: %s", indexArg)
	}
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	if index > len(cfg.Nodes) {
		return fmt.Errorf("节点编号超出范围: %d（最大 %d）", index, len(cfg.Nodes))
	}
	node := cfg.Nodes[index-1]
	currentID := cfg.CurrentNodeID
	if status := application.GetStatus(); status.CurrentNodeID != "" {
		currentID = status.CurrentNodeID
	}
	fmt.Printf("编号: %d\n当前节点: %t\n名称: %s\n协议: %s\n服务器: %s\n端口: %d\n", index, node.ID == currentID, node.Name, node.Protocol, node.Server, node.Port)
	keys := sortedSettingKeys(node.Settings)
	if len(keys) == 0 {
		fmt.Println("协议参数: 无")
		return nil
	}
	fmt.Println("协议参数:")
	for _, key := range keys {
		fmt.Printf("  %s: %s\n", key, node.Settings[key])
	}
	return nil
}

// switchNodeByIndex selects a node using a 1-based index.
func switchNodeByIndex(application *app.App, arg string) error {
	index, err := strconv.Atoi(arg)
	if err != nil || index < 1 {
		return fmt.Errorf("节点编号无效: %s", arg)
	}
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	if index > len(cfg.Nodes) {
		return fmt.Errorf("节点编号超出范围: %d（最大 %d）", index, len(cfg.Nodes))
	}
	return application.SelectNode(cfg.Nodes[index-1].ID)
}

func nodeIDsFromArgs(application *app.App, args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("请提供节点编号，例如: sbtun del 1 2 3")
	}
	cfg, err := application.LoadConfig()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(args))
	ids := make([]string, 0, len(args))
	for _, arg := range args {
		index, convErr := strconv.Atoi(arg)
		if convErr != nil || index < 1 || index > len(cfg.Nodes) {
			return nil, fmt.Errorf("节点编号无效: %s", arg)
		}
		id := cfg.Nodes[index-1].ID
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func interactiveEdit(application *app.App, indexArg string) error {
	if !isTerminalInput(os.Stdin) {
		return errors.New("edit 交互模式需要交互式终端；请直接在终端运行，或使用完整参数形式")
	}
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	if len(cfg.Nodes) == 0 {
		return fmt.Errorf("暂无节点，请先使用 add-node 添加")
	}
	if err := listNodes(application); err != nil {
		return err
	}
	reader := bufio.NewReader(os.Stdin)
	indexText := indexArg
	if indexText == "" {
		indexText, err = promptLine(reader, "请输入要编辑的节点编号")
		if err != nil {
			return err
		}
	}
	index, err := strconv.Atoi(indexText)
	if err != nil || index < 1 || index > len(cfg.Nodes) {
		return fmt.Errorf("节点编号无效: %s", indexText)
	}
	node := cfg.Nodes[index-1]
	node.Name = promptKeep(reader, "名称", node.Name)
	node.Server = promptKeep(reader, "服务器", node.Server)
	portText := promptKeep(reader, "端口", strconv.Itoa(int(node.Port)))
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("端口无效: %s", portText)
	}
	for _, key := range sortedSettingKeys(node.Settings) {
		value := promptKeep(reader, key, node.Settings[key])
		if value == "" {
			delete(node.Settings, key)
		} else {
			node.Settings[key] = value
		}
	}
	node.Port = uint16(port)
	if err := application.UpdateNode(node.ID, node); err != nil {
		return err
	}
	fmt.Println("节点信息已保存")
	return nil
}

func isTerminalInput(input *os.File) bool {
	info, err := input.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func promptLine(reader *bufio.Reader, label string) (string, error) {
	fmt.Printf("%s: ", label)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptKeep(reader *bufio.Reader, label, current string) string {
	value, err := promptLine(reader, fmt.Sprintf("%s [%s]", label, current))
	if err != nil || value == "" {
		return current
	}
	if value == "-" {
		return ""
	}
	return value
}

func sortedSettingKeys(settings map[string]string) []string {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func printAppUpdate() {
	result, err := app.New().CheckAppUpdate()
	switch {
	case err != nil:
		fmt.Fprintln(os.Stderr, err)
	case result.HasUpdate:
		fmt.Printf("发现新版本 v%s，运行 sbtun upgrade 更新: %s\n", result.LatestVersion, result.ReleaseURL)
	default:
		fmt.Println("已是最新版本")
	}
}
