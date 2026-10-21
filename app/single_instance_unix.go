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
	// 释放时不删锁文件：解锁和删除之间别的进程可能已锁住旧 inode，
	// 删掉路径后第三个进程又能在新文件上加锁，出现两个实例。
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
