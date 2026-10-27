//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

func TestShareWithInvokerSetsModeAndOwner(t *testing.T) {
	t.Setenv("SUDO_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("SUDO_GID", strconv.Itoa(os.Getgid()))
	path := filepath.Join(t.TempDir(), "sets.json")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ShareWithInvoker(path, 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o644 || int(info.Sys().(*syscall.Stat_t).Uid) != os.Getuid() {
		t.Fatalf("mode=%v", info.Mode())
	}
}

// root 下把旧版本留下的 root 文件交还调用者，私钥除外。非 root 环境跳过。
func TestShareTreeWithInvokerSkipsPrivateKeys(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("需要 root")
	}
	t.Setenv("SUDO_UID", "12345")
	t.Setenv("SUDO_GID", "12345")
	dir := t.TempDir()
	for _, name := range []string{"config.json", "ca.key"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ShareTreeWithInvoker(dir)
	uid := func(name string) uint32 {
		info, _ := os.Stat(filepath.Join(dir, name))
		return info.Sys().(*syscall.Stat_t).Uid
	}
	if uid("config.json") != 12345 || uid("ca.key") != 0 {
		t.Fatalf("config=%d key=%d", uid("config.json"), uid("ca.key"))
	}
}
