//go:build !windows

package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core"
	"github.com/caichengle666/sbtun/core/singbox"
	"github.com/caichengle666/sbtun/core/tun"
)

// 假内核：check 拒绝含 node-bad 的配置；run 每次启动记一行，收到 SIGTERM 正常退出。
const fakeReloadSingBox = `#!/bin/sh
if [ "$1" = check ]; then
  if grep -q node-bad "$3"; then echo rejected >&2; exit 1; fi
  exit 0
fi
echo run >> "$(dirname "$0")/runs"
trap 'exit 0' TERM
while :; do sleep 0.05; done
`

func newReloadTestRuntime(t *testing.T) (*RuntimeCoordinator, string) {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "sing-box")
	if err := os.WriteFile(binary, []byte(fakeReloadSingBox), 0o755); err != nil {
		t.Fatal(err)
	}
	auth, err := newLocalAuth()
	if err != nil {
		t.Fatal(err)
	}
	r := &RuntimeCoordinator{
		State: core.NewStateStore(), SingBox: singbox.NewManager(), TUN: tun.NewManager(),
		WorkDir: filepath.Join(dir, "work"), Binary: binary, LocalAuth: auth,
	}
	t.Cleanup(func() { _ = r.Stop() })
	return r, dir
}

func reloadTestConfig(id string, port uint16) config.Config {
	cfg := config.Default()
	cfg.Nodes = []config.Node{{ID: id, Name: id, Protocol: "socks", Server: "127.0.0.1", Port: port}}
	cfg.CurrentNodeID = id
	return cfg
}

// countRuns 返回假内核的启动次数；进程启动后写文件有延迟，最多等到 want 次或 1 秒。
func countRuns(t *testing.T, dir string, want int) int {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		data, _ := os.ReadFile(filepath.Join(dir, "runs"))
		n := strings.Count(string(data), "run")
		if n >= want || time.Now().After(deadline) {
			return n
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// 校验失败时 runtime.json 保持旧内容，旧进程继续运行。
func TestReloadKeepsRuntimeConfigWhenInvalid(t *testing.T) {
	r, dir := newReloadTestRuntime(t)
	if err := r.Start(context.Background(), reloadTestConfig("good", 1080)); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(r.configPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Reload(context.Background(), reloadTestConfig("bad", 1080)); err == nil || !strings.Contains(err.Error(), ErrConfigInvalid) {
		t.Fatalf("Reload err=%v", err)
	}
	after, _ := os.ReadFile(r.configPath())
	if !bytes.Equal(before, after) {
		t.Fatal("校验失败后 runtime.json 被覆盖")
	}
	if state, _ := r.State.Get(); state != core.StateRunning || !r.SingBox.Running() {
		t.Fatalf("校验失败不应影响运行中的进程: state=%s", state)
	}
	if n := countRuns(t, dir, 1); n != 1 {
		t.Fatalf("sing-box 启动次数=%d want 1", n)
	}
	matches, _ := filepath.Glob(filepath.Join(r.WorkDir, ".runtime-*"))
	if len(matches) != 0 {
		t.Fatalf("临时文件未清理: %v", matches)
	}
}

// 已停止时 Reload 只写配置，不再自行启动 sing-box。
func TestReloadOnStoppedRuntimeDoesNotStart(t *testing.T) {
	r, dir := newReloadTestRuntime(t)
	if err := r.Reload(context.Background(), reloadTestConfig("good", 1080)); err != nil {
		t.Fatal(err)
	}
	if state, _ := r.State.Get(); state != core.StateStopped || r.SingBox.Running() {
		t.Fatalf("state=%s running=%v", state, r.SingBox.Running())
	}
	if _, err := os.Stat(r.configPath()); err != nil {
		t.Fatalf("应写入 runtime.json: %v", err)
	}
	if n := countRuns(t, dir, 0); n != 0 {
		t.Fatalf("sing-box 不应启动, runs=%d", n)
	}
}

// 只改了不影响 sing-box 配置的字段（订阅 UpdatedAt）时不重启；节点参数变了才需要重载。
func TestSyncIfUnchangedSkipsIdenticalConfig(t *testing.T) {
	r, dir := newReloadTestRuntime(t)
	cfg := reloadTestConfig("good", 1080)
	cfg.Subscriptions = []config.Subscription{{ID: "s1", Name: "s1", URL: "https://example.com/sub", UpdatedAt: "2026-01-01T00:00:00Z"}}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	same := cfg
	same.Subscriptions = []config.Subscription{{ID: "s1", Name: "s1", URL: "https://example.com/sub", UpdatedAt: "2026-10-21T00:00:00Z"}}
	if ok, err := r.SyncIfUnchanged(same); err != nil || !ok {
		t.Fatalf("配置等价时应跳过重启: ok=%v err=%v", ok, err)
	}
	if ok, _ := r.SyncIfUnchanged(reloadTestConfig("good", 1081)); ok {
		t.Fatal("节点端口变了必须重载")
	}
	if n := countRuns(t, dir, 1); n != 1 {
		t.Fatalf("sing-box 启动次数=%d want 1", n)
	}
	_ = r.Stop()
	if ok, _ := r.SyncIfUnchanged(same); ok {
		t.Fatal("已停止时不应报告无需重载")
	}
}

// 运行中只切换当前节点（Clash API 已无缝切换）时，配置文件变化不应触发重启 sing-box。
func TestSyncIfUnchangedIgnoresNodeSelection(t *testing.T) {
	r, dir := newReloadTestRuntime(t)
	cfg := reloadTestConfig("a", 1080)
	cfg.Nodes = append(cfg.Nodes, config.Node{ID: "b", Name: "b", Protocol: "socks", Server: "127.0.0.1", Port: 1081})
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	switched := cfg
	switched.CurrentNodeID = "b"
	if ok, err := r.SyncIfUnchanged(switched); err != nil || !ok {
		t.Fatalf("只切节点时应跳过重启: ok=%v err=%v", ok, err)
	}
	data, _ := os.ReadFile(r.configPath())
	if !bytes.Contains(data, []byte(`"default": "node-b"`)) && !bytes.Contains(data, []byte(`"default":"node-b"`)) {
		t.Fatal("runtime.json 应写入新的当前节点，下次启动用它")
	}
	if ok, _ := r.SyncIfUnchanged(switched); !ok {
		t.Fatal("再次同步相同配置也应跳过")
	}
	changed := switched
	changed.Nodes = []config.Node{cfg.Nodes[0], {ID: "b", Name: "b", Protocol: "socks", Server: "127.0.0.1", Port: 1082}}
	if ok, _ := r.SyncIfUnchanged(changed); ok {
		t.Fatal("节点参数变了必须重载")
	}
	if n := countRuns(t, dir, 1); n != 1 {
		t.Fatalf("sing-box 启动次数=%d want 1", n)
	}
}
