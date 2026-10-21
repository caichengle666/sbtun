//go:build darwin

package singbox

import (
	"bytes"
	"errors"
	"os/exec"
	"strconv"
)

// macOS 没有 Pdeathsig，靠 pid 文件在下次启动时清理残留进程。
func startProcess(cmd *exec.Cmd) error { return cmd.Start() }

// processExecutable 用 ps 读取进程的可执行文件完整路径（macOS 的 comm 是完整路径）。
func processExecutable(pid int) (string, error) {
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", err
	}
	path := string(bytes.TrimSpace(out))
	if path == "" {
		return "", errors.New("进程不存在")
	}
	return path, nil
}
