//go:build !windows

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func withConfigLock(path string, action func() error) error {
	if path == "" {
		return fmt.Errorf("配置文件路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	// sudo/pkexec 下新建的目录和锁文件交还调用者，否则普通用户以后无法在里面写文件、打开锁。
	_ = applyInvokerOwnership(filepath.Dir(path))
	lockPath := path + ".lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDONLY, 0o666)
	if err != nil {
		return fmt.Errorf("打开配置锁失败: %w", err)
	}
	defer file.Close()
	_ = applyInvokerOwnership(lockPath)
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("锁定配置失败: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	return action()
}
