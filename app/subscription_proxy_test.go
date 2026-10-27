package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core/rules"
)

func TestProxyCandidatesPrefersCurrentNode(t *testing.T) {
	cfg := config.Config{CurrentNodeID: "c", Nodes: []config.Node{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}}
	got := proxyCandidates(cfg)
	if len(got) != subscriptionProxyTries || got[0].ID != "c" || got[1].ID != "a" || got[2].ID != "b" {
		t.Fatalf("候选节点顺序不对: %+v", got)
	}
}

func TestProxiedFetcherKeepsHTTPStatusFromDirect(t *testing.T) {
	f := &proxiedFetcher{app: &App{}}
	if _, err := f.fetch("http://127.0.0.1:1/sub", "x"); err == nil {
		t.Fatal("连不上时应该返回错误")
	}
	if f.direct == nil {
		t.Fatal("直连失败后应记录原因，后续不再直连")
	}
}

// 只有网络不通才换节点重试；HTTP 状态、内容错误、主动取消都不重试。
func TestNeedsProxy(t *testing.T) {
	_, dialErr := http.Get("http://127.0.0.1:1/")
	for err, want := range map[error]bool{
		nil:                    false,
		dialErr:                true,
		rules.StatusError(404): false,
		httpStatusError{502}:   false,
		errors.New("规则集格式错误"):  false,
		fmt.Errorf("下载失败: %w", &url.Error{Op: "Get", URL: "x", Err: context.Canceled}): false,
	} {
		if got := needsProxy(err); got != want {
			t.Errorf("needsProxy(%v) = %v, want %v", err, got, want)
		}
	}
}

// 规则集下载和订阅一样：直连不通时记下原因，HTTP 错误原样返回、不经节点重试。
func TestRuleFetchKeepsHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	a := &App{}
	if _, err := a.fetchRule(server.URL); !errors.As(err, new(rules.StatusError)) {
		t.Fatalf("err = %v", err)
	}
	if _, err := a.fetchRule("http://127.0.0.1:1/x.srs"); err == nil || !needsProxy(err) {
		t.Fatalf("连不上时应返回网络错误: %v", err)
	}
}
