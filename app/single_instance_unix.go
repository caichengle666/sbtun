//go:build !windows

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
)

func AcquireSingleInstance() (func(), error) {
	path := filepath.Join(singleInstanceDir, "sbtun-single-instance-"+strconv.Itoa(os.Getuid())+".lock")
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

// singleInstanceDir 是固定的锁目录。不用 os.TempDir()：macOS 的 TMPDIR 每个用户不同，
// pkexec/sudo 又可能重置环境，两个 root 实例会拿到不同的锁而同时运行。
// 文件名带 uid：之前 sudo 留下的 root 锁文件不会挡住普通用户。安卓没有 /tmp，用应用私有的临时目录。
var singleInstanceDir = func() string {
	if runtime.GOOS == "android" {
		return os.TempDir()
	}
	return "/tmp"
}()
