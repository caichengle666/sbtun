//go:build cli

package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/caichengle666/sbtun/app"
)

func validateConnsArgs(args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "test":
		if len(args) == 2 {
			return nil
		}
	case "close":
		if len(args) == 2 {
			if _, err := strconv.Atoi(args[1]); err == nil || args[1] == "all" {
				return nil
			}
		}
	default:
		if len(args) == 1 {
			return nil // 关键词过滤
		}
	}
	return usageError("conns", args)
}

// runConnsCommand 查看当前连接走了哪条规则、哪个出口，或测试某个网站会怎么走。
func runConnsCommand(application *app.App, args []string) error {
	if len(args) == 2 && args[0] == "test" {
		c, err := application.TestRoute(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("网站: %s\n命中: %s\n出口: %s\n", c.Host, c.Rule, connTarget(c))
		return nil
	}
	conns, err := application.GetConnections()
	if err != nil {
		return err
	}
	if len(args) == 2 && args[0] == "close" {
		if args[1] == "all" {
			return application.CloseConnection("")
		}
		index, _ := strconv.Atoi(args[1])
		if index < 1 || index > len(conns) {
			return fmt.Errorf("连接编号无效: %s", args[1])
		}
		return application.CloseConnection(conns[index-1].ID)
	}
	keyword := ""
	if len(args) == 1 {
		keyword = strings.ToLower(args[0])
	}
	shown := 0
	for i, c := range conns {
		if keyword != "" && !strings.Contains(strings.ToLower(c.Host+" "+c.Rule+" "+connTarget(c)), keyword) {
			continue
		}
		shown++
		fmt.Printf("%d\t%s:%s\t%s\t%s\t↑%s ↓%s\n", i+1, c.Host, c.Port, c.Rule, connTarget(c), cliBytes(c.Upload), cliBytes(c.Download))
	}
	if shown == 0 {
		fmt.Println("没有连接")
	}
	return nil
}

func connTarget(c app.ConnectionDTO) string {
	if c.Via != "" {
		return c.Target + "（" + c.Via + "）"
	}
	return c.Target
}

func cliBytes(n uint64) string {
	units := []string{"B", "KB", "MB", "GB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
