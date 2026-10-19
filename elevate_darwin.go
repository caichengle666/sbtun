//go:build darwin && !cli && !bindings

package main

import (
	"os"
	"os/exec"
	"strings"
)

// ensureElevated relaunches the app as root through the macOS administrator
// prompt, because creating utun devices, routes and DNS settings needs root.
func ensureElevated() {
	if os.Geteuid() == 0 {
		return
	}
	if os.Getenv("SBTUN_ELEVATED") == "1" {
		showDarwinAlert("sbtun 需要管理员权限才能运行。")
		os.Exit(1)
	}
	exe, err := os.Executable()
	if err != nil {
		showDarwinAlert("获取 sbtun 路径失败: " + err.Error())
		os.Exit(1)
	}
	command := "SBTUN_ELEVATED=1 /usr/bin/nohup " + shellQuote(exe) + " >/dev/null 2>&1 &"
	script := `do shell script "` + appleScriptEscape(command) + `" with prompt "sbtun 需要管理员权限来创建 TUN 网卡和修改系统 DNS。" with administrator privileges`
	// Cancelling the prompt also lands here; there is nothing to show then.
	_ = exec.Command("osascript", "-e", script).Run()
	os.Exit(0)
}

func showDarwinAlert(message string) {
	script := `display alert "sbtun" message "` + appleScriptEscape(message) + `" as critical`
	_ = exec.Command("osascript", "-e", script).Run()
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func appleScriptEscape(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
}
