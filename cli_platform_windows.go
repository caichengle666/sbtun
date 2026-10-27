//go:build windows && cli

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func cliSignals() []os.Signal { return []os.Signal{os.Interrupt} }

// 提权后的进程开在新控制台窗口里，执行完就关；父进程用这个前置参数给它一个文件收集输出，结束后回显。
// 用参数不用环境变量：经 UAC 提权的进程不一定继承父进程设置的环境变量。
const elevatedLogFlag = "--sbtun-elevated-log="

// errorCancelled 是父进程约定的退出码：用户在 UAC 里点了“否”或提权失败（sbtun 自己只用 0、1、2）。
const errorCancelled = 1223

func redirectElevatedOutput() {
	if len(os.Args) < 2 || !strings.HasPrefix(os.Args[1], elevatedLogFlag) {
		return
	}
	path := strings.TrimPrefix(os.Args[1], elevatedLogFlag)
	os.Args = append(os.Args[:1], os.Args[2:]...)
	if file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0); err == nil {
		os.Stdout, os.Stderr = file, file
	}
}

// elevatedArguments 把参数拼成 Windows 命令行（Start-Process 的 -ArgumentList 数组不给元素加引号，
// 带空格的参数会被拆开、空参数会丢），再写成 PowerShell 单引号字符串。
func elevatedArguments(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = syscall.EscapeArg(arg)
	}
	return psQuote(strings.Join(quoted, " "))
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func relaunchElevated(command string, args []string) error {
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
	// 前台运行和交互式编辑要在新窗口里看输出、输入，不收集。
	childArgs := os.Args[1:]
	logPath := ""
	if !interactiveCommand(command, args) {
		log, err := os.CreateTemp("", "sbtun-elevated-*.log")
		if err != nil {
			return fmt.Errorf("创建输出文件失败: %w", err)
		}
		log.Close()
		logPath = log.Name()
		childArgs = append([]string{elevatedLogFlag + logPath}, childArgs...)
	}
	script := "$env:SBTUN_ELEVATED='1'; try { $p = Start-Process -FilePath " + psQuote(exe) + " -ArgumentList " + elevatedArguments(childArgs) +
		" -Verb RunAs -Wait -PassThru -ErrorAction Stop } catch { [Console]::Error.WriteLine($_.Exception.Message); exit " +
		fmt.Sprint(errorCancelled) + " }; exit $p.ExitCode"
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		code = exitErr.ExitCode()
	} else if runErr != nil {
		os.Remove(logPath)
		return fmt.Errorf("请求管理员权限失败: %w", runErr)
	}
	if logPath != "" {
		output, _ := os.ReadFile(logPath)
		os.Remove(logPath)
		if code == 0 {
			os.Stdout.Write(output)
		} else {
			os.Stderr.Write(output)
		}
	}
	if code == errorCancelled {
		return fmt.Errorf("请求管理员权限失败: %s", strings.TrimSpace(stderr.String()))
	}
	os.Exit(code)
	return nil
}
