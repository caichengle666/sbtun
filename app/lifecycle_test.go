package app

import (
	"os"
	"testing"
	"time"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core"
	"github.com/caichengle666/sbtun/core/singbox"
	"github.com/caichengle666/sbtun/core/tun"
)

func TestWatchSingBoxExitSynchronizesState(t *testing.T) {
	r := &RuntimeCoordinator{
		State:   core.NewStateStore(),
		SingBox: singbox.NewManager(),
		TUN:     tun.NewManager(),
	}
	r.State.Set(core.StateRunning, "")
	r.TUN.MarkRunning()

	done := make(chan struct{})
	go func() {
		r.watchSingBoxExit()
		close(done)
	}()

	// Exited 通道由 sing-box Manager 暴露；这里通过真实 Manager 的事件通道注入
	// 一个异常退出事件，验证协调器不会继续保持 Running。
	r.SingBox.InjectExit(os.ErrProcessDone)

	deadline := time.After(time.Second)
	for {
		state, message := r.State.Get()
		if state == core.StateError {
			if message == "" {
				t.Fatal("异常退出后错误状态缺少错误信息")
			}
			if r.TUN.Running() {
				t.Fatal("异常退出后 TUN 仍为运行状态")
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("异常退出后状态未同步，当前状态=%s", state)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

}

func TestCaptureConfigChangedIgnoresOrderAndFormatting(t *testing.T) {
	before := config.Config{CaptureEnabled: true, CaptureDomains: []string{" example.com", ".API.Example.com"}}
	after := config.Config{CaptureEnabled: true, CaptureDomains: []string{"api.example.com", "example.com"}}
	if captureConfigChanged(before, after) {
		t.Fatal("equivalent capture domains should not require restarting capture")
	}
}

func TestCaptureConfigChangedDetectsRuleChanges(t *testing.T) {
	before := config.Config{CaptureEnabled: true, CaptureDomains: []string{"example.com"}}
	after := config.Config{CaptureEnabled: true, CaptureDomains: []string{"example.com", "api.example.com"}}
	if !captureConfigChanged(before, after) {
		t.Fatal("changed capture domains should restart capture")
	}
}

func TestWatchSingBoxExitDoesNotOverrideStopped(t *testing.T) {
	r := &RuntimeCoordinator{
		State:   core.NewStateStore(),
		SingBox: singbox.NewManager(),
		TUN:     tun.NewManager(),
	}
	r.State.Set(core.StateStopped, "")

	go r.watchSingBoxExit()
	r.SingBox.InjectExit(os.ErrProcessDone)
	time.Sleep(50 * time.Millisecond)

	state, message := r.State.Get()
	if state != core.StateStopped || message != "" {
		t.Fatalf("主动停止状态被错误覆盖: state=%s message=%q", state, message)
	}
}

func TestAtomicWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/runtime.json"
	want := []byte(`{"test":true}`)
	if err := atomicWrite(path, want); err != nil {
		t.Fatalf("atomicWrite 失败: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 runtime.json 失败: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("内容不一致: got=%q want=%q", got, want)
	}
}

func TestWatchSingBoxExitIgnoresReplacedProcess(t *testing.T) {
	r := &RuntimeCoordinator{
		State:   core.NewStateStore(),
		SingBox: singbox.NewManager(),
		TUN:     tun.NewManager(),
	}
	r.State.Set(core.StateRunning, "")
	r.TUN.MarkRunning()
	// Generation 2 is the process started by a reload; generation 1 crashed
	// while the reload held the coordinator lock.
	r.SingBox.SetGenerationForTest(2)
	go r.watchSingBoxExit()
	r.SingBox.InjectExitEvent(singbox.ExitEvent{Err: os.ErrProcessDone, Generation: 1})

	time.Sleep(200 * time.Millisecond)
	if state, message := r.State.Get(); state != core.StateRunning {
		t.Fatalf("stale exit changed state to %s: %s", state, message)
	}
	if !r.TUN.Running() {
		t.Fatal("stale exit must not mark the TUN stopped")
	}
}
