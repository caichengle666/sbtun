package singbox

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// pidFileName 记录正在运行的 sing-box 的 pid，放在运行目录（runtime.json 同目录）。
const pidFileName = "sing-box.pid"

// CleanupStale 结束上次运行残留的 sing-box（sbtun 被强杀、崩溃时可能留下）。
// 只在持有单实例锁时调用；pid 文件里的进程仍在、且可执行文件就是 binary 才会动手，避免误杀 pid 被复用的进程。
func CleanupStale(workDir, binary string) {
	cleanupStalePID(filepath.Join(workDir, pidFileName), binary)
}

func cleanupStalePID(pidPath, binary string) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}
	defer os.Remove(pidPath)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 || pid == os.Getpid() || !isProcessOf(pid, binary) {
		return
	}
	// 先请求优雅退出让内核撤掉路由，超时再强杀。
	_ = stopStalePID(pid, false)
	for deadline := time.Now().Add(gracefulStopTimeout); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if !isProcessOf(pid, binary) {
			return
		}
	}
	_ = stopStalePID(pid, true)
}

func isProcessOf(pid int, binary string) bool {
	exe, err := processExecutable(pid)
	return err == nil && samePath(exe, binary)
}

func samePath(a, b string) bool {
	a, b = normalizePath(a), normalizePath(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func normalizePath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	return filepath.Clean(path)
}

func writePIDFile(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600)
}

// removePIDFile 只删除记录着该 pid 的文件，不误删新进程写下的记录。
func removePIDFile(path string, pid int) {
	if data, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(data)) == strconv.Itoa(pid) {
		_ = os.Remove(path)
	}
}
