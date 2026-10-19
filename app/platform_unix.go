//go:build !windows

package app

import (
	"context"
	"os/exec"
	"runtime"
)

func singBoxBinaryName() string { return "sing-box" }

func configureProcess(cmd *exec.Cmd) {}

func pingCommand(ctx context.Context, host string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		// macOS ping's -W is in milliseconds; -t is the overall timeout in seconds.
		return exec.CommandContext(ctx, "ping", "-c", "1", "-t", "3", host)
	}
	return exec.CommandContext(ctx, "ping", "-c", "1", "-W", "3", host)
}
