//go:build linux && cli

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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
	if command == "help" || command == "-h" || command == "--help" {
		printHelp()
		return
	}
	if command == "version" || command == "-v" || command == "--version" {
		fmt.Println("sbtun 0.1.10")
		return
	}
	release, err := app.AcquireSingleInstance()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer release()
	if command == "run" || command == "start" {
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
			fmt.Printf("已添加 %d 个节点\n", count)
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

func printHelp() {
	fmt.Println(`sbtun Linux 命令行用法:
  sbtun run                         前台启动 TUN 和代理
  sbtun start                       同 run
  sbtun status                      查看配置状态
  sbtun add-node <链接>             添加节点或订阅
  sbtun route <模式>                设置路由: smart/global/direct/custom
  sbtun rules list                  列出规则集
  sbtun rules update <id>           更新规则集
  sbtun rules update-all            更新全部规则集
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
