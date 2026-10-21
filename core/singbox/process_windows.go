//go:build windows

package singbox

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// requestGracefulStop is not available on Windows: sing-box runs without a
// console, so there is no console control event to deliver. The process is
// killed instead; Wintun removes the adapter when its handle closes, and the
// TUN manager cleans up the remaining routes and DNS settings.
func requestGracefulStop(cmd *exec.Cmd) error {
	return errors.New("graceful stop unsupported on windows")
}

// killOnCloseJob 是带 KILL_ON_JOB_CLOSE 的 Job 对象，句柄随 sbtun 进程存活；
// sbtun 被强杀或崩溃时系统关闭句柄，Job 里的 sing-box 随之结束。创建失败返回 0。
var killOnCloseJob = sync.OnceValue(func() windows.Handle {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0
	}
	return job
})

// startProcess 启动后把 sing-box 放进 Job 对象，绑定到 sbtun 的生命周期。
func startProcess(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	if job := killOnCloseJob(); job != 0 {
		if h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)); err == nil {
			_ = windows.AssignProcessToJobObject(job, h)
			windows.CloseHandle(h)
		}
	}
	return nil
}

func processExecutable(pid int) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err == nil && code != 259 { // 259 = STILL_ACTIVE
		return "", os.ErrProcessDone
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:size]), nil
}

// stopStalePID 在 Windows 上没有优雅退出的手段，直接结束进程。
func stopStalePID(pid int, _ bool) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	defer p.Release()
	return p.Kill()
}
