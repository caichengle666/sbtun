package mobile

import (
	"strings"
	"testing"

	"github.com/caichengle666/sbtun/app"
)

func TestCallDispatchesToApp(t *testing.T) {
	instance = app.New()
	defer func() { instance = nil }()
	if got := Call("Ping", "[]"); !strings.Contains(got, `"result":"sbtun 运行正常"`) {
		t.Fatalf("Ping=%s", got)
	}
	if got := Call("Ping", `["x"]`); !strings.Contains(got, "需要 0 个参数") {
		t.Fatalf("arg count=%s", got)
	}
	if got := Call("Shutdown", `[null]`); !strings.Contains(got, "不支持") {
		t.Fatalf("blocked=%s", got)
	}
	if got := Call("NoSuch", "[]"); !strings.Contains(got, "不支持") {
		t.Fatalf("missing=%s", got)
	}
	// 返回 error 的方法：未初始化时 Start 报错而不是崩溃
	if got := Call("Start", "[]"); !strings.Contains(got, `"error"`) {
		t.Fatalf("Start=%s", got)
	}
}
