//go:build linux && cli

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	case "run", "start", "stop", "route", "add-node", "add-rule", "switch":
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
  sbtun switch <编号>                按编号切换节点
  sbtun rules update-all            更新全部规则集
  sbtun stop                        停止运行中的实例
  sbtun help                        显示帮助
  sbtun version                     显示版本

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
