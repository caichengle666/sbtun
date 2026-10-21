//go:build linux

package singbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// processGone 判断进程已结束；被 init 收养后可能短暂处于僵尸状态，也算结束。
func processGone(pid int) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	fields := strings.Fields(string(data[strings.LastIndexByte(string(data), ')')+1:]))
	return len(fields) > 0 && (fields[0] == "Z" || fields[0] == "X")
}

func copySleep(t *testing.T) string {
	t.Helper()
	src, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("没有 sleep")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "sing-box")
	if err := os.WriteFile(binary, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return binary
}

// 启动时发现上次残留、且可执行路径就是 sing-box 的进程，先把它结束。
func TestCleanupStaleKillsMatchingProcess(t *testing.T) {
	binary := copySleep(t)
	stale := exec.Command(binary, "60")
	if err := stale.Start(); err != nil {
		t.Fatal(err)
	}
	other := exec.Command("sleep", "60") // 路径不同的进程不能被误杀
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer other.Process.Kill()
	pidPath := filepath.Join(t.TempDir(), pidFileName)

	_ = writePIDFile(pidPath, other.Process.Pid)
	cleanupStalePID(pidPath, binary)
	if processGone(other.Process.Pid) {
		t.Fatal("可执行路径不符的进程被误杀")
	}

	_ = writePIDFile(pidPath, stale.Process.Pid)
	exited := make(chan struct{})
	go func() { _ = stale.Wait(); close(exited) }()
	cleanupStalePID(pidPath, binary)
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		_ = stale.Process.Kill()
		t.Fatal("残留 sing-box 未被清理")
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatal("清理后应删除 pid 文件")
	}
}

// sbtun 被 SIGKILL 后，Pdeathsig 让 sing-box 随之退出，不留孤儿进程。
func TestChildDiesWithParent(t *testing.T) {
	binary, configPath := writeScript(t, "exec sleep 60\n")
	helper := exec.Command(os.Args[0], "-test.run=TestOrphanHelperProcess")
	helper.Env = append(os.Environ(), "SBTUN_ORPHAN_BINARY="+binary, "SBTUN_ORPHAN_CONFIG="+configPath)
	out, err := helper.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	n, _ := out.Read(buf)
	childPID, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		_ = helper.Process.Kill()
		t.Fatalf("helper 输出无效: %q", buf[:n])
	}
	_ = helper.Process.Signal(syscall.SIGKILL)
	_ = helper.Wait()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if processGone(childPID) {
			return
		}
	}
	_ = syscall.Kill(childPID, syscall.SIGKILL)
	t.Fatal("父进程被强杀后 sing-box 仍在运行")
}

func TestOrphanHelperProcess(t *testing.T) {
	binary := os.Getenv("SBTUN_ORPHAN_BINARY")
	if binary == "" {
		return
	}
	m := NewManager()
	if err := m.Start(context.Background(), binary, os.Getenv("SBTUN_ORPHAN_CONFIG")); err != nil {
		os.Exit(1)
	}
	os.Stdout.WriteString(strconv.Itoa(m.cmd.Process.Pid) + "\n")
	time.Sleep(time.Minute)
}
