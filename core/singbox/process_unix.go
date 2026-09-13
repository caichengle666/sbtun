//go:build !windows

package singbox

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}
