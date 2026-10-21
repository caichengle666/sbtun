//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// 释放锁后锁文件保留，避免 flock 与 unlink 之间的竞态出现两个实例。
func TestSingleInstanceKeepsLockFile(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	release, err := AcquireSingleInstance()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireSingleInstance(); err == nil {
		t.Fatal("第二个实例应当拿不到锁")
	}
	release()
	path := filepath.Join(os.TempDir(), "sbtun-single-instance-"+strconv.Itoa(os.Getuid())+".lock")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("释放后锁文件不应删除: %v", err)
	}
	release, err = AcquireSingleInstance()
	if err != nil {
		t.Fatalf("释放后应能重新加锁: %v", err)
	}
	release()
}
