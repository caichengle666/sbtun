//go:build !windows

package app

import (
	"os"
	"os/exec"
	"syscall"
)

func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// matchOwner gives a file written by the elevated GUI (root) the owner of the
// file it replaces, so the portable folder stays usable without sudo.
func matchOwner(path string, like os.FileInfo) {
	stat, ok := like.Sys().(*syscall.Stat_t)
	if !ok || os.Geteuid() != 0 {
		return
	}
	_ = os.Lchown(path, int(stat.Uid), int(stat.Gid))
}
