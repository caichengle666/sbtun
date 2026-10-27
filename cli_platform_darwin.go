//go:build darwin && cli

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func cliSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }

// redirectElevatedOutput 只有 Windows 需要：提权后的进程开在新控制台里，输出交给父进程回显。
func redirectElevatedOutput() {}

func relaunchElevated(string, []string) error {
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
