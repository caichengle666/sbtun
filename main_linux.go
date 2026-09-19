//go:build linux && cli

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/caichengle666/sbtun/app"
	"github.com/caichengle666/sbtun/config"
)

func main() {
	command := "run"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	application := app.New()
	application.StartupCLI(ctx)
	var err error
	if command == "help" || command == "-h" || command == "--help" {
		printHelp()
		return
	}
	if command == "version" || command == "-v" || command == "--version" {
		fmt.Println("sbtun " + app.Version())
		return
	}
	captureRun := command == "capture" && len(os.Args) > 2 && os.Args[2] == "run"
	if requiresElevation(command, os.Args[2:]) {
		if err := relaunchElevated(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if captureRun {
		if err := enableCapture(application, os.Args[3:]); err != nil {
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
		if err := application.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("sbtun 已启动，按 Ctrl+C 停止")
		<-ctx.Done()
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
		fmt.Printf("路由模式: %s\nDNS 模式: %s\n当前节点: %s\n节点数量: %d\n", cfg.RoutingMode, cfg.DNSMode, currentID, len(cfg.Nodes))
		if status.Selector != "" {
			fmt.Printf("运行时 selector: %s\n", status.Selector)
		}
	case "route":
		if len(os.Args) < 3 {
			err = fmt.Errorf("用法: %s route <smart|global|direct|custom>", os.Args[0])
			break
		}
		err = application.SetRoutingMode(config.RoutingMode(os.Args[2]))
		if err == nil {
			fmt.Println("路由模式已更新")
		}
	case "add-node":
		if len(os.Args) < 3 {
			err = fmt.Errorf("用法: %s add-node <节点链接或订阅链接>", os.Args[0])
			break
		}
		if os.Args[2] == "-h" || os.Args[2] == "--help" {
			fmt.Printf("用法: %s add-node <节点链接或订阅链接>\n", os.Args[0])
			break
		}
		var count int
		count, err = application.ImportSubscription(os.Args[2])
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
		ids, parseErr := nodeIDsFromArgs(application, os.Args[2:])
		if parseErr != nil {
			err = parseErr
			break
		}
		err = application.RemoveNodes(ids)
		if err == nil {
			fmt.Printf("已删除 %d 个节点\n", len(ids))
		}
	case "test":
		ids, parseErr := nodeIDsFromArgs(application, os.Args[2:])
		if parseErr != nil {
			err = parseErr
			break
		}
		for _, result := range application.TestNodes(ids) {
			fmt.Printf("%s\t健康=%t\t%s\n", result.NodeID, result.Healthy, result.Message)
		}
	case "edit":
		if len(os.Args) == 2 {
			err = interactiveEdit(application, "")
			break
		}
		if len(os.Args) == 3 {
			err = interactiveEdit(application, os.Args[2])
			break
		}
		if len(os.Args) == 4 {
			index, convErr := strconv.Atoi(os.Args[2])
			if convErr != nil || index < 1 {
				err = fmt.Errorf("节点编号无效: %s", os.Args[2])
				break
			}
			cfg, loadErr := application.LoadConfig()
			if loadErr != nil {
				err = loadErr
				break
			}
			if index > len(cfg.Nodes) {
				err = fmt.Errorf("节点编号超出范围: %d", index)
				break
			}
			err = application.UpdateNodeFromLink(cfg.Nodes[index-1].ID, os.Args[3])
			if err == nil {
				fmt.Println("节点全部参数已更新")
			}
			break
		}
		if len(os.Args) < 6 {
			err = fmt.Errorf("用法: %s edit <编号> <节点链接> 或 edit <编号> <名称> <服务器> <端口>", os.Args[0])
			break
		}
		index, convErr := strconv.Atoi(os.Args[2])
		port, portErr := strconv.Atoi(os.Args[5])
		if convErr != nil || index < 1 || portErr != nil || port < 1 || port > 65535 {
			err = fmt.Errorf("节点编号或端口无效")
			break
		}
		cfg, loadErr := application.LoadConfig()
		if loadErr != nil {
			err = loadErr
			break
		}
		if index > len(cfg.Nodes) || strings.TrimSpace(os.Args[3]) == "" || strings.TrimSpace(os.Args[4]) == "" {
			err = fmt.Errorf("节点编号或节点信息无效")
			break
		}
		node := cfg.Nodes[index-1]
		node.Name, node.Server, node.Port = strings.TrimSpace(os.Args[3]), strings.TrimSpace(os.Args[4]), uint16(port)
		err = application.UpdateNode(node.ID, node)
		if err == nil {
			fmt.Println("节点信息已更新")
		}
	case "switch":
		if len(os.Args) < 3 {
			err = fmt.Errorf("用法: %s switch <编号>", os.Args[0])
			break
		}
		err = switchNodeByIndex(application, os.Args[2])
		if err == nil {
			fmt.Println("节点已切换")
		}
	case "add-rule":
		if len(os.Args) < 3 {
			err = fmt.Errorf("用法: %s add-rule <链接|文件|文本>", os.Args[0])
			break
		}
		var ruleCount int
		ruleCount, err = application.ImportCustomRules(os.Args[2])
		if err == nil {
			fmt.Printf("已导入 %d 条规则\n", ruleCount)
		}
	case "stop":
		err = stopRunningInstance()
		if err == nil {
			fmt.Println("sbtun 已停止")
		}
	case "rules":
		err = runRulesCommand(application, os.Args[2:])
	case "capture":
		err = runCaptureCommand(application, os.Args[2:])
	default:
		err = fmt.Errorf("未知命令: %s", command)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func requiresElevation(command string, args []string) bool {
	switch command {
	case "run", "start", "stop", "route", "add-node", "add-rule", "switch", "del", "delete", "edit":
		return true
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

// relaunchElevated keeps the CLI convenient for TUN and root-owned runtime
// data. The marker prevents sudo/pkexec recursion after elevation.
func relaunchElevated() error {
	if os.Geteuid() == 0 || os.Getenv("SBTUN_ELEVATED") == "1" {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取 CLI 路径失败: %w", err)
	}
	args := append([]string{exe}, os.Args[1:]...)
	env := append(os.Environ(), "SBTUN_ELEVATED=1")
	for _, tool := range []string{"pkexec", "sudo"} {
		path, lookErr := exec.LookPath(tool)
		if lookErr != nil {
			continue
		}
		if tool == "pkexec" {
			args = append([]string{"pkexec", "env", "SBTUN_ELEVATED=1", exe}, os.Args[1:]...)
			return syscall.Exec(path, args, env)
		}
		args = append([]string{"sudo", "-E", exe}, os.Args[1:]...)
		return syscall.Exec(path, args, env)
	}
	return fmt.Errorf("需要管理员权限，请安装 pkexec 或 sudo 后重试")
}

func printHelp() {
	fmt.Println("sbtun Linux 命令行\n\n用法:\n  sbtun <命令> [参数]")
	printHelpGroup("运行", []helpEntry{
		{"run", "前台启动 TUN 和代理"},
		{"start", "同 run"},
		{"stop", "停止运行中的实例"},
		{"status", "查看配置状态"},
	})
	printHelpGroup("节点", []helpEntry{
		{"nodes", "列出节点编号，* 表示当前节点"},
		{"add-node <链接>", "添加单节点链接或订阅链接"},
		{"switch <编号>", "按编号切换节点"},
		{"test <编号...>", "按编号顺序测试节点健康"},
		{"del <编号...>", "批量删除节点，例如 del 1 2 3"},
		{"edit [编号]", "进入交互式编辑，可直接指定节点"},
		{"edit <编号> <节点链接>", "用完整链接覆盖全部参数"},
		{"edit <编号> <名称> <服务器> <端口>", "仅修改基本信息"},
	})
	printHelpGroup("路由与规则", []helpEntry{
		{"route <模式>", "设置路由: smart/global/direct/custom"},
		{"add-rule <链接|文件|文本>", "添加自定义规则"},
		{"rules list", "列出规则集"},
		{"rules update <id>", "更新指定规则集"},
		{"rules update-all", "更新全部规则集"},
	})
	printHelpGroup("流量分析", []helpEntry{
		{"capture run [关键词...]", "启用抓包并前台启动 TUN"},
		{"capture enable <关键词...>", "保存抓包关键词，下次启动生效"},
		{"capture disable", "关闭抓包，下次启动生效"},
		{"capture status", "查看抓包配置和运行状态"},
		{"capture list", "列出已保存请求"},
		{"capture show <ID>", "查看一条已保存请求的完整内容"},
		{"capture clear", "删除已保存请求"},
		{"capture cert <install|uninstall>", "安装或卸载抓包根证书"},
	})
	printHelpGroup("其他", []helpEntry{
		{"help", "显示帮助"},
		{"version", "显示版本"},
	})
	fmt.Println(`
示例:
  sbtun nodes
  sbtun switch 2
  sbtun del 1 2 3
  sbtun test 1 2 3
  sbtun edit 2
  sbtun edit 2 "vless://UUID@1.2.3.4:443?..."

说明:
  交互编辑中直接按回车保留当前值，输入 - 清空当前参数。
  名称包含空格时请使用引号，例如: sbtun edit 2 "韩国 高速" 1.2.3.4 443
  批量测试按顺序执行，不会同时启动大量检测进程。
  删除当前节点后会自动选择剩余节点。
  运行中修改 route 或 rules 会自动重载 sing-box。
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

func runRulesCommand(application *app.App, args []string) error {
	if len(args) == 0 || args[0] == "list" {
		for _, rule := range application.ListRules() {
			fmt.Printf("%s\t%s\t存在=%t\n", rule.ID, rule.Name, rule.Exists)
		}
		return nil
	}
	switch args[0] {
	case "update":
		if len(args) < 2 {
			return fmt.Errorf("用法: sbtun rules update <id>")
		}
		return application.UpdateRule(args[1])
	case "update-all":
		errs := application.UpdateAllRules()
		if len(errs) > 0 {
			return fmt.Errorf("规则更新失败: %v", errs)
		}
		return nil
	default:
		return fmt.Errorf("用法: sbtun rules [list|update <id>|update-all]")
	}
}

func runCaptureCommand(application *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: sbtun capture <run|enable|disable|status|list|show|clear|cert>")
	}
	switch args[0] {
	case "run":
		return nil
	case "enable":
		if len(args) < 2 {
			return fmt.Errorf("用法: sbtun capture enable <域名或关键词...>")
		}
		if err := enableCapture(application, args[1:]); err != nil {
			return err
		}
		fmt.Println("抓包已启用，下次启动生效")
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
		fmt.Printf("启用: %t\n运行: %t\n关键词: %s\n已保存请求: %d\n证书已安装: %t\n存储文件: %s\n",
			cfg.CaptureEnabled, status.Running, strings.Join(cfg.CaptureDomains, ", "), status.FlowCount,
			status.CertificateInstalled, status.StoragePath)
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
		if len(args) < 2 {
			return fmt.Errorf("用法: sbtun capture show <ID>")
		}
		id, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil || id == 0 {
			return fmt.Errorf("请求 ID 无效: %s", args[1])
		}
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
		if len(args) < 2 {
			return fmt.Errorf("用法: sbtun capture cert <install|uninstall>")
		}
		switch args[1] {
		case "install":
			if err := application.InstallCaptureCertificate(); err != nil {
				return err
			}
			fmt.Println("抓包根证书已安装")
			return nil
		case "uninstall":
			if err := application.UninstallCaptureCertificate(); err != nil {
				return err
			}
			fmt.Println("抓包根证书已卸载")
			return nil
		default:
			return fmt.Errorf("用法: sbtun capture cert <install|uninstall>")
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
	for i, node := range cfg.Nodes {
		marker := " "
		if node.ID == currentID {
			marker = "*"
		}
		fmt.Printf("%s %d\t%s\t%s:%d\n", marker, i+1, node.Name, node.Server, node.Port)
	}
	fmt.Printf("当前节点: %s\n", currentID)
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

// stopRunningInstance scans /proc for running sbtun-cli or sbtun processes
// and sends SIGTERM. It avoids matching its own PID.
func stopRunningInstance() error {
	myPid := os.Getpid()
	myExe, _ := os.Readlink("/proc/self/exe")
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return fmt.Errorf("读取 /proc 失败: %w", err)
	}
	var targets []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, convErr := strconv.Atoi(entry.Name())
		if convErr != nil || pid == myPid {
			continue
		}
		commPath := filepath.Join("/proc", entry.Name(), "comm")
		comm, readErr := os.ReadFile(commPath)
		if readErr != nil {
			continue
		}
		name := strings.TrimSpace(string(comm))
		if name == "sbtun-cli" || name == "sbtun" {
			if myExe != "" {
				exe, readErr := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
				if readErr != nil || exe != myExe {
					continue
				}
			}
			targets = append(targets, pid)
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("没有运行中的 sbtun 实例")
	}
	for _, pid := range targets {
		proc, findErr := os.FindProcess(pid)
		if findErr != nil {
			continue
		}
		if sigErr := proc.Signal(syscall.SIGTERM); sigErr == nil {
			return nil
		}
	}
	return fmt.Errorf("无法停止 sbtun 实例")
}
