//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

// 释放锁后锁文件保留，避免 flock 与 unlink 之间的竞态出现两个实例。
func TestSingleInstanceKeepsLockFile(t *testing.T) {
	old := singleInstanceDir
	singleInstanceDir = t.TempDir()
	defer func() { singleInstanceDir = old }()
	release, err := AcquireSingleInstance()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireSingleInstance(); err == nil {
		t.Fatal("第二个实例应当拿不到锁")
	}
	release()
	path := filepath.Join(singleInstanceDir, "sbtun-single-instance-"+strconv.Itoa(os.Getuid())+".lock")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("释放后锁文件不应删除: %v", err)
	}
	release, err = AcquireSingleInstance()
	if err != nil {
		t.Fatalf("释放后应能重新加锁: %v", err)
	}
	release()
}

// 锁目录固定，不随 TMPDIR 变化（macOS 每个用户的 TMPDIR 不同，sudo/pkexec 会重置环境）。
func TestSingleInstanceDirIgnoresTMPDIR(t *testing.T) {
	if runtime.GOOS != "android" && singleInstanceDir != "/tmp" {
		t.Fatalf("singleInstanceDir = %q", singleInstanceDir)
	}
}
