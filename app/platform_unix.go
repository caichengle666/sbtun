//go:build !windows

package app

import (
	"context"
	"os/exec"
)

func singBoxBinaryName() string { return "sing-box" }

func configureProcess(cmd *exec.Cmd) {}

func pingCommand(ctx context.Context, host string) *exec.Cmd {
	return exec.CommandContext(ctx, "ping", "-c", "1", "-W", "3", host)
}
