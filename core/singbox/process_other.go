//go:build !linux && !darwin && !windows

package singbox

import (
	"errors"
	"os/exec"
)

func startProcess(cmd *exec.Cmd) error { return cmd.Start() }

// 其他平台无法可靠核对可执行路径，不清理残留进程，避免误杀。
func processExecutable(int) (string, error) { return "", errors.ErrUnsupported }
