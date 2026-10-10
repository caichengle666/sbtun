//go:build windows

package singbox

import (
	"errors"
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// requestGracefulStop is not available on Windows: sing-box runs without a
// console, so there is no console control event to deliver. The process is
// killed instead; Wintun removes the adapter when its handle closes, and the
// TUN manager cleans up the remaining routes and DNS settings.
func requestGracefulStop(cmd *exec.Cmd) error {
	return errors.New("graceful stop unsupported on windows")
}
