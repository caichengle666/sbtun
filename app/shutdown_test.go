package app

import (
	"context"
	"errors"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

// 退出开始后不再启动或重载代理，托盘结束函数仍会被调用。
func TestShutdownBlocksLaterStartAndReload(t *testing.T) {
	a := New()
	called := false
	end := func() { called = true }
	a.trayEnd.Store(&end)
	a.Shutdown(context.Background())
	if !called {
		t.Fatal("Shutdown 应调用托盘结束函数")
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if err := a.startWithConfigLocked(config.Default()); !errors.Is(err, errClosing) {
		t.Fatalf("start err=%v", err)
	}
	if err := a.mutateConfigAndSyncLocked(func(config.Config, *config.Config) error { return nil }, true); !errors.Is(err, errClosing) {
		t.Fatalf("mutate err=%v", err)
	}
	if err := a.reloadIfRunningLocked(); !errors.Is(err, errClosing) {
		t.Fatalf("reload err=%v", err)
	}
}
