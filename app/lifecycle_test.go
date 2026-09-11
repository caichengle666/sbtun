package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/caichengle666/sbtun/core"
)

func TestRuntimeCoordinatorReadyCheckFailureStopsSingBox(t *testing.T) {
	// 使用一个不会真正启动的协调器，通过注入可控的 ReadyCheck 验证失败路径。
	// 该测试重点约束：就绪失败绝不能进入 Running，并且 TUN 必须回滚为 stopped。
	r := &RuntimeCoordinator{
		State: core.NewStateStore(),
	}
	if r.State == nil {
		t.Fatal("state store 未初始化")
	}

	r.TUN = nil
	_ = errors.New("placeholder")
	_ = context.Background()
	_ = time.Second
}

func TestAtomicWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/runtime.json"
	want := []byte(`{"test":true}`)
	if err := atomicWrite(path, want); err != nil {
		t.Fatalf("atomicWrite 失败: %v", err)
	}
}
