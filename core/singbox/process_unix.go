//go:build !windows

package singbox

import (
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) {}

// requestGracefulStop sends SIGTERM, which sing-box handles by closing its
// inbounds and undoing auto_route before exiting.
func requestGracefulStop(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGTERM)
}

// stopStalePID 先让残留进程优雅退出（SIGTERM），force 时直接 SIGKILL。
func stopStalePID(pid int, force bool) error {
	if force {
		return syscall.Kill(pid, syscall.SIGKILL)
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}
