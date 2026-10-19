//go:build darwin && cli

package main

import (
	"fmt"
	"os"
	"os/exec"
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
	path, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("需要管理员权限，请使用 sudo 运行")
	}
	args := append([]string{"sudo", "SBTUN_ELEVATED=1", exe}, os.Args[1:]...)
	return syscall.Exec(path, args, os.Environ())
}

// stopRunningInstance finds the GUI or CLI by process name, asks it to stop
// through the shared stop request file and waits until the process exits.
func stopRunningInstance() error {
	out, _ := exec.Command("pgrep", "^sbtun(-cli)?$").Output()
	myPID := os.Getpid()
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(field); err == nil && pid != myPID {
			pids = append(pids, pid)
		}
	}
	if len(pids) == 0 {
		return fmt.Errorf("没有运行中的 sbtun 实例")
	}
	if err := app.RequestRunningInstanceStop(); err != nil {
		return fmt.Errorf("发送停止请求失败: %w", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		alive := false
		for _, pid := range pids {
			if syscall.Kill(pid, 0) == nil {
				alive = true
			}
		}
		if !alive {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("等待 sbtun 正常退出超时")
}
