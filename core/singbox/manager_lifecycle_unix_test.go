//go:build !windows

package singbox

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeScript 写一个假内核脚本，返回脚本路径和运行目录下的 runtime.json 路径。
func writeScript(t *testing.T, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "sing-box")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "runtime.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	return binary, configPath
}

func waitFile(path string, timeout time.Duration) bool {
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// ctx 取消时先发 SIGTERM 让内核优雅退出，并按预期退出处理。
func TestStartContextCancelStopsGracefully(t *testing.T) {
	binary, configPath := writeScript(t, `trap 'touch "$(dirname "$0")/graceful"; exit 0' TERM
touch "$(dirname "$0")/ready"
while :; do sleep 0.05; done
`)
	dir := filepath.Dir(binary)
	ctx, cancel := context.WithCancel(context.Background())
	m := NewManager()
	if err := m.Start(ctx, binary, configPath); err != nil {
		t.Fatal(err)
	}
	if !waitFile(filepath.Join(dir, "ready"), 2*time.Second) {
		t.Fatal("假内核未就绪")
	}
	pidPath := filepath.Join(dir, pidFileName)
	data, err := os.ReadFile(pidPath)
	if err != nil || strings.TrimSpace(string(data)) == "" {
		t.Fatalf("启动后应写 pid 文件: %v", err)
	}
	cancel()
	select {
	case event := <-m.Exited():
		if !event.Expected {
			t.Fatalf("ctx 取消导致的退出应为预期退出: %+v", event)
		}
	case <-time.After(gracefulStopTimeout + 2*time.Second):
		t.Fatal("等待退出超时")
	}
	if _, err := os.Stat(filepath.Join(dir, "graceful")); err != nil {
		t.Fatal("应先收到 SIGTERM 而不是被直接 SIGKILL")
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("退出后应删除 pid 文件: %v", err)
	}
}

// 残留 pid 文件指向的进程已不存在或不是 sing-box 时，只删文件，不动别的进程。
func TestCleanupStaleIgnoresOtherProcess(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, pidFileName)
	if err := writePIDFile(pidPath, os.Getpid()+1_000_000); err != nil {
		t.Fatal(err)
	}
	cleanupStalePID(pidPath, filepath.Join(dir, "sing-box"))
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatal("无效 pid 文件应被删除")
	}
	// 指向测试进程自身：路径不符，不能被杀。
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getppid())), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanupStalePID(pidPath, filepath.Join(dir, "sing-box"))
}
