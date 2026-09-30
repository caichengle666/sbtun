//go:build windows && cli

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/caichengle666/sbtun/app"
	"golang.org/x/sys/windows"
)

func cliSignals() []os.Signal { return []os.Signal{os.Interrupt} }

func relaunchElevated() error {
	if windows.GetCurrentProcessToken().IsElevated() {
		return nil
	}
	if os.Getenv("SBTUN_ELEVATED") == "1" {
		return fmt.Errorf("需要管理员权限，请使用管理员身份运行")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取 CLI 路径失败: %w", err)
	}
	quotedArgs := make([]string, 0, len(os.Args))
	quotedArgs = append(quotedArgs, "'"+strings.ReplaceAll(exe, "'", "''")+"'")
	for _, arg := range os.Args[1:] {
		quotedArgs = append(quotedArgs, "'"+strings.ReplaceAll(arg, "'", "''")+"'")
	}
	script := "$env:SBTUN_ELEVATED='1'; Start-Process -FilePath " + quotedArgs[0] + " -ArgumentList @(" + strings.Join(quotedArgs[1:], ",") + ") -Verb RunAs -Wait"
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("请求管理员权限失败: %w: %s", err, strings.TrimSpace(string(output)))
	}
	os.Exit(0)
	return nil
}

func stopRunningInstance() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := strings.ReplaceAll(filepath.Dir(exe), "'", "''")
	myPID := strconv.Itoa(os.Getpid())
	script := "$p=Get-CimInstance Win32_Process | Where-Object { ($_.Name -eq 'sbtun.exe' -or $_.Name -eq 'sbtun-cli.exe') -and $_.ExecutablePath -and (Split-Path $_.ExecutablePath -Parent) -eq '" + dir + "' -and $_.ProcessId -ne " + myPID + " }; if(-not $p){exit 3}; $p.ProcessId"
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("停止 sbtun 失败: %w: %s", err, strings.TrimSpace(string(output)))
	}
	pidText := strings.TrimSpace(string(output))
	pidFields := strings.Fields(pidText)
	if len(pidFields) == 0 {
		return fmt.Errorf("停止 sbtun 失败: 未找到运行实例 PID")
	}
	pid, err := strconv.ParseUint(pidFields[0], 10, 32)
	if err != nil {
		return fmt.Errorf("读取运行实例 PID 失败: %w", err)
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("等待 sbtun 退出失败: %w", err)
	}
	defer windows.CloseHandle(handle)
	if err := app.RequestRunningInstanceStop(); err != nil {
		return fmt.Errorf("发送停止请求失败: %w", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, waitErr := windows.WaitForSingleObject(handle, 100)
		if waitErr != nil {
			return fmt.Errorf("等待 sbtun 退出失败: %w", waitErr)
		}
		if status == windows.WAIT_OBJECT_0 {
			return nil
		}
	}
	return fmt.Errorf("等待 sbtun 正常退出超时")
}
