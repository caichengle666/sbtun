//go:build !windows

package app

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caichengle666/sbtun/config"
)

// 内核秒退时测速应立即报错，而不是等满超时。
func TestNodeProxyDetectsImmediateExit(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "sing-box")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho bad config >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	node := config.Node{ID: "n1", Name: "n1", Protocol: "socks", Server: "127.0.0.1", Port: 1080}
	proxy, err := startNodeProxy(context.Background(), binary, node, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	started := time.Now()
	err = proxy.waitReady(5 * time.Second)
	if err == nil || !strings.Contains(err.Error(), "已退出") || !strings.Contains(err.Error(), "bad config") {
		t.Fatalf("waitReady err=%v", err)
	}
	request, _ := http.NewRequest(http.MethodGet, "http://"+proxy.addr, nil)
	if _, err := waitForURL(context.Background(), proxy.client, request, proxy.exited, 5*time.Second); err == nil || !strings.Contains(err.Error(), "已退出") {
		t.Fatalf("waitForURL err=%v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("退出检测太慢: %v", elapsed)
	}
}
