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
