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

var (
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole     = kernel32.NewProc("AttachConsole")
	procFreeConsole       = kernel32.NewProc("FreeConsole")
	procGetConsoleWindow  = kernel32.NewProc("GetConsoleWindow")
	procSetCtrlHandler    = kernel32.NewProc("SetConsoleCtrlHandler")
	procGenerateCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
	consoleMu             sync.Mutex
	// swallowCtrl 吞掉转发 Ctrl+C 时自己也会收到的那一份。用回调而不是 SetConsoleCtrlHandler(NULL, TRUE)：
	// 后者的“忽略 Ctrl+C”会被之后启动的 sing-box 继承，下次就关不掉了。
	swallowCtrl = sync.OnceFunc(func() {
		procSetCtrlHandler.Call(windows.NewCallback(func(uint32) uintptr { return 1 }), 1)
	})
)

// requestGracefulStop 给 sing-box 发 Ctrl+C，让它自己撤掉 TUN 地址、路由和 DNS，
// 不然被强杀后地址可能残留，下次启动报 “set ipv4 address: The object already exists”。
// sing-box 以 CREATE_NO_WINDOW 启动，有自己的隐藏控制台：GUI 版（本身没有控制台）
// 临时挂到那个控制台上发送 Ctrl+C 再脱离。CLI 版自己占着终端，不能脱离，仍走强杀。
func requestGracefulStop(cmd *exec.Cmd) error {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	if hwnd, _, _ := procGetConsoleWindow.Call(); hwnd != 0 {
		return errors.New("当前进程有控制台，无法转发 Ctrl+C")
	}
	if ok, _, err := procAttachConsole.Call(uintptr(cmd.Process.Pid)); ok == 0 {
		return err
	}
	defer procFreeConsole.Call()
	swallowCtrl()
	if ok, _, err := procGenerateCtrlEvent.Call(windows.CTRL_C_EVENT, 0); ok == 0 {
		return err
	}
	return nil
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
