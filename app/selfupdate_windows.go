//go:build windows

package app

import (
	"os"
	"os/exec"
	"syscall"
)

func detachProcess(cmd *exec.Cmd) {
	// DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP：新进程不受旧进程退出时的 Ctrl 事件影响。
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200}
}

func matchOwner(string, os.FileInfo) {}
