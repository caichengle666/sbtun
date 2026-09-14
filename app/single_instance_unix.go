//go:build !windows

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

func AcquireSingleInstance() (func(), error) {
	// A previous sudo launch may leave the shared /tmp lock owned by root.
	// Use a per-UID fallback so an unprivileged CLI can recover cleanly while
	// still preventing duplicate processes for that user.
	path := filepath.Join(os.TempDir(), "sbtun-single-instance-"+strconv.Itoa(os.Getuid())+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("创建单实例锁失败: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("sbtun 已经在运行")
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		_ = os.Remove(path)
	}, nil
}
