//go:build windows

package app

import (
	"fmt"
	"golang.org/x/sys/windows"
)

const instanceMutexName = "Global\\sbtun-single-instance"

// AcquireSingleInstance prevents multiple sbtun processes from sharing TUN state.
func AcquireSingleInstance() (func(), error) {
	name, err := windows.UTF16PtrFromString(instanceMutexName)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		return nil, fmt.Errorf("创建单实例锁失败: %w", err)
	}
	if windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(handle)
		return nil, fmt.Errorf("sbtun 已经在运行")
	}
	return func() { windows.CloseHandle(handle) }, nil
}
