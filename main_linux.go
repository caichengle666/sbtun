//go:build linux && cli

package main

import (
	"bufio"
	"context"
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
	if requiresElevation(command) {
		if err := relaunchElevated(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if command == "run" || command == "start" {
		application.StartMonitoring()
	}
	if command == "run" || command == "start" {
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
		fmt.Printf("路由模式: %s\nDNS 模式: %s\n当前节点: %s\n节点数量: %d\n", cfg.RoutingMode, cfg.DNSMode, cfg.CurrentNodeID, len(cfg.Nodes))
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
			err = interactiveEdit(application)
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
			err = fmt.Errorf("usage: %s switch <index>", os.Args[0])
			break
		}
		err = switchNodeByIndex(application, os.Args[2])
		if err == nil {
			fmt.Println("node switched")
		}
	case "add-rule":
		if len(os.Args) < 3 {
			err = fmt.Errorf("usage: %s add-rule <链接|文件|文本>", os.Args[0])
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
	default:
		err = fmt.Errorf("未知命令: %s", command)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func requiresElevation(command string) bool {
	switch command {
	case "run", "start", "stop", "route", "add-node", "add-rule", "switch", "del", "delete", "edit":
		return true
	default:
		return false
	}
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
	fmt.Println(`sbtun Linux 命令行用法:
  sbtun run                         前台启动 TUN 和代理
  sbtun start                       同 run
  sbtun status                      查看配置状态
  sbtun add-node <链接>             添加节点或订阅
  sbtun route <模式>                设置路由: smart/global/direct/custom
  sbtun add-rule <链接|文件|文本>  添加自定义规则
  sbtun rules list                  列出规则集
  sbtun rules update <id>           更新规则集
  sbtun nodes                       列出节点编号
  sbtun del <编号...>                批量删除节点，例如 del 1 2 3
  sbtun test <编号...>               按编号顺序测试节点健康
  sbtun edit                         列出节点并进入交互式编辑
  sbtun edit <编号> <节点链接>       用完整链接覆盖全部参数
  sbtun edit <编号> <名称> <服务器> <端口> 仅修改基本信息
  sbtun switch <编号>                按编号切换节点
  sbtun rules update-all            更新全部规则集
  sbtun stop                        停止运行中的实例
  sbtun help                        显示帮助
  sbtun version                     显示版本

示例:
  sbtun nodes                       先查看节点编号
  sbtun del 1 2 3                   删除第 1、2、3 个节点
  sbtun test 1 2 3                  顺序测试第 1、2、3 个节点
  sbtun edit 2 韩国节点 1.2.3.4 443 修改第 2 个节点
  sbtun edit 2 "vless://UUID@1.2.3.4:443?..." 修改 UUID、TLS、Reality 等全部参数
  交互编辑中直接按回车保留当前值，输入 - 清空当前参数。
  名称包含空格时请使用引号，例如: sbtun edit 2 "韩国 高速" 1.2.3.4 443

批量测试按顺序执行，不会同时启动大量检测进程；删除当前节点后会自动选择剩余节点。
 运行中修改 route 或 rules 会自动重载 sing-box。停止前台运行请按 Ctrl+C。`)
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

// listNodes prints the current node list with 1-based indices.
func listNodes(application *app.App) error {
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	if len(cfg.Nodes) == 0 {
		fmt.Println("no nodes yet; use add-node first")
		return nil
	}
	for i, node := range cfg.Nodes {
		marker := " "
		if node.ID == cfg.CurrentNodeID {
			marker = "*"
		}
		fmt.Printf("%s %d\t%s\t%s:%d\n", marker, i+1, node.Name, node.Server, node.Port)
	}
	fmt.Printf("current: %s\n", cfg.CurrentNodeID)
	return nil
}

// switchNodeByIndex selects a node using a 1-based index.
func switchNodeByIndex(application *app.App, arg string) error {
	index, err := strconv.Atoi(arg)
	if err != nil || index < 1 {
		return fmt.Errorf("invalid node index: %s", arg)
	}
	cfg, err := application.LoadConfig()
	if err != nil {
		return err
	}
	if index > len(cfg.Nodes) {
		return fmt.Errorf("node index out of range: %d (max %d)", index, len(cfg.Nodes))
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

func interactiveEdit(application *app.App) error {
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
	indexText, err := promptLine(reader, "请输入要编辑的节点编号")
	if err != nil {
		return err
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
