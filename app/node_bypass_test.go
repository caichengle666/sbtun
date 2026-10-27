package app

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core"
)

func TestNodeResolverCachesAndKeepsStaleAnswers(t *testing.T) {
	calls := 0
	fail := false
	r := &nodeResolver{lookup: func(_ context.Context, host string) ([]netip.Addr, error) {
		calls++
		if fail {
			return nil, errors.New("dns down")
		}
		return []netip.Addr{netip.MustParseAddr("198.51.100.9")}, nil
	}}
	nodes := []config.Node{{Server: "node.example.com"}, {Server: "203.0.113.7"}}
	if got := r.resolve(nodes); len(got) != 1 || got[0].String() != "198.51.100.9" {
		t.Fatalf("resolved=%v", got)
	}
	r.resolve(nodes)
	if calls != 1 {
		t.Fatalf("cached lookup repeated: calls=%d", calls)
	}
	// After the TTL a failed lookup keeps the previous answer.
	r.cache["node.example.com"] = nodeResolveEntry{addrs: r.cache["node.example.com"].addrs, at: time.Now().Add(-nodeResolveCacheTTL - time.Second)}
	fail = true
	if got := r.resolve(nodes); len(got) != 1 || calls != 2 {
		t.Fatalf("stale answer lost: resolved=%v calls=%d", got, calls)
	}
}

// 节点域名解析出的 IP 变了（DNS 轮换、缓存过期），但配置本身没变时，不能判定为“配置变化”而重启。
func TestSyncIfUnchangedIgnoresNodeIPRotation(t *testing.T) {
	r := NewRuntimeCoordinator(t.TempDir(), "sing-box")
	ip := "198.51.100.1"
	r.resolver.lookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr(ip)}, nil
	}
	cfg := config.Default()
	cfg.Nodes = []config.Node{{ID: "a", Name: "a", Protocol: "socks", Server: "node.example.com", Port: 1080}}
	cfg.CurrentNodeID = "a"
	built := builtConfig{resolved: r.resolver.resolve(cfg.Nodes)}
	var err error
	if built.data, err = r.buildConfig(cfg, built.resolved); err != nil {
		t.Fatal(err)
	}
	r.State.Set(core.StateRunning, "")
	r.activeConfig, r.activeConfigSet, r.activeData, r.activeResolved = cfg, true, built.data, built.resolved

	ip = "198.51.100.2"
	r.resolver.cache = nil // 缓存过期
	if same, err := r.SyncIfUnchanged(cfg); err != nil || !same {
		t.Fatalf("IP 轮换被当成配置变化: same=%v err=%v", same, err)
	}
	changed := cfg
	changed.Nodes = []config.Node{{ID: "a", Name: "a", Protocol: "socks", Server: "other.example.com", Port: 1080}}
	if same, _ := r.SyncIfUnchanged(changed); same {
		t.Fatal("节点地址变了应判定为配置变化")
	}
}
