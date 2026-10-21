//go:build linux

package singbox

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

var (
	spawnOnce sync.Once
	spawnReqs chan func()
)

// startProcess 让 sbtun 意外退出时内核收到 SIGTERM 自行清理路由，不留孤儿进程。
// Pdeathsig 绑定的是创建子进程的 OS 线程：线程退出子进程也会收到信号，而 Go 会在
// 锁定线程的 goroutine 结束时销毁线程。所以统一在一个永不退出、锁定线程的 goroutine 里创建。
func startProcess(cmd *exec.Cmd) error {
	spawnOnce.Do(func() {
		spawnReqs = make(chan func())
		go func() {
			runtime.LockOSThread()
			for req := range spawnReqs {
				req()
			}
		}()
	})
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM
	errc := make(chan error, 1)
	spawnReqs <- func() { errc <- cmd.Start() }
	return <-errc
}

// processExecutable 返回进程的可执行文件路径；内核被升级替换后路径带 " (deleted)" 后缀。
func processExecutable(pid int) (string, error) {
	path, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	return strings.TrimSuffix(path, " (deleted)"), err
}
