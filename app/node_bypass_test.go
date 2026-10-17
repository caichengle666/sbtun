package app

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/caichengle666/sbtun/config"
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
