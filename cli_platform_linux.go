//go:build linux && cli

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/caichengle666/sbtun/app"
)

func cliSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }

func relaunchElevated() error {
	if os.Geteuid() == 0 || os.Getenv("SBTUN_ELEVATED") == "1" {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取 CLI 路径失败: %w", err)
	}
	env := append(os.Environ(), "SBTUN_ELEVATED=1")
	for _, tool := range []string{"pkexec", "sudo"} {
		path, lookErr := exec.LookPath(tool)
		if lookErr != nil {
			continue
		}
		if tool == "pkexec" {
			args := append([]string{"pkexec", "env", "SBTUN_ELEVATED=1", exe}, os.Args[1:]...)
			return syscall.Exec(path, args, env)
		}
		args := append([]string{"sudo", "-E", exe}, os.Args[1:]...)
		return syscall.Exec(path, args, env)
	}
	return fmt.Errorf("需要管理员权限，请安装 pkexec 或 sudo 后重试")
}

func stopRunningInstance() error {
	myPID := os.Getpid()
	myExe, _ := os.Readlink("/proc/self/exe")
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return fmt.Errorf("读取 /proc 失败: %w", err)
	}
	for _, entry := range entries {
		pid, convErr := strconv.Atoi(entry.Name())
		if convErr != nil || pid == myPID {
			continue
		}
		comm, readErr := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if readErr != nil {
			continue
		}
		name := strings.TrimSpace(string(comm))
		if name != "sbtun-cli" && name != "sbtun" {
			continue
		}
		if myExe != "" {
			exe, readErr := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
			if readErr != nil || filepath.Dir(exe) != filepath.Dir(myExe) {
				continue
			}
		}
		if _, findErr := os.FindProcess(pid); findErr == nil {
			if err := app.RequestRunningInstanceStop(); err != nil {
				return fmt.Errorf("发送停止请求失败: %w", err)
			}
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); os.IsNotExist(err) {
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}
			return fmt.Errorf("等待 sbtun 正常退出超时")
		}
	}
	return fmt.Errorf("没有运行中的 sbtun 实例")
}
