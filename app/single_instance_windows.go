//go:build windows

package app

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const instanceMutexName = "Global\\sbtun-single-instance"

// AcquireSingleInstance prevents multiple sbtun processes from sharing TUN state.
func AcquireSingleInstance() (func(), error) {
	name, err := windows.UTF16PtrFromString(instanceMutexName)
	if err != nil {
		return nil, err
	}
	// 互斥体已存在时 CreateMutex 返回有效句柄，同时 err 为 ERROR_ALREADY_EXISTS。
	handle, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return nil, fmt.Errorf("sbtun 已经在运行")
	}
	if err != nil {
		return nil, fmt.Errorf("创建单实例锁失败: %w", err)
	}
	return func() { windows.CloseHandle(handle) }, nil
}

// WindowClassName is the Win32 class of the GUI window, so a second launch
// (e.g. clicking the pinned taskbar icon while sbtun sits in the tray) can find
// the running window and bring it back instead of complaining.
const WindowClassName = "sbtunMainWindow"

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

const (
	swShow    = 5
	swRestore = 9
)

// ActivateRunningInstance shows the running GUI's window. It returns false
// when no window exists, e.g. the lock is held by the command-line version.
func ActivateRunningInstance() bool {
	class, _ := windows.UTF16PtrFromString(WindowClassName)
	// The running instance may still be creating its window; wait briefly.
	for i := 0; i < 20; i++ {
		hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(class)), 0)
		if hwnd != 0 {
			procShowWindow.Call(hwnd, swShow)
			if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
				procShowWindow.Call(hwnd, swRestore)
			}
			procSetForegroundWindow.Call(hwnd)
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func ShowSingleInstanceMessage() {
	text, _ := windows.UTF16PtrFromString("sbtun 已经在运行（可能是命令行版本），请勿重复启动。")
	caption, _ := windows.UTF16PtrFromString("sbtun")
	_, _ = windows.MessageBox(0, text, caption, 0x00000010)
}
