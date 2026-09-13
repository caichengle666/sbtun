//go:build windows

package app

import (
	"context"
	"os/exec"
	"syscall"
)

func singBoxBinaryName() string { return "sing-box.exe" }

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func pingCommand(ctx context.Context, host string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "ping.exe", "-n", "1", "-w", "3000", host)
	configureProcess(cmd)
	return cmd
}
