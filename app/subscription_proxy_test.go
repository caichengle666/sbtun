package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

// fakeNodeProxy 冒充已启动的节点代理：一个 HTTP 正向代理，把请求交给 handler 处理并计数。
func fakeNodeProxy(t *testing.T, f *proxiedFetcher, handler http.Handler) *atomic.Int32 {
	t.Helper()
	hits := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	proxyURL, _ := url.Parse(server.URL)
	f.proxy = &nodeProxy{client: &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}}
	t.Cleanup(func() { f.proxy = nil }) // 假代理没有进程，不能走 close
	return hits
}

// WebDAV 直连不通时经节点重试：PUT/PROPFIND 的方法、认证头和请求体都原样重放。
func TestWebDAVFallsBackToNodeProxy(t *testing.T) {
	dav, puts := newWebDAVTestServer(t)
	client, err := newWebDAVClient(nil, config.WebDAVSettings{URL: "http://127.0.0.1:1/dav/", Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	hits := fakeNodeProxy(t, client.fetcher, dav.Config.Handler)
	if err := client.ensureDir(); err != nil {
		t.Fatal(err)
	}
	if client.fetcher.direct == nil {
		t.Fatal("直连失败应被记下")
	}
	name := backupPrefix + "20260101-0000a.json"
	if err := client.put(name, []byte(`{"x":1}`)); err != nil {
		t.Fatal(err)
	}
	list, err := client.list()
	if err != nil || len(list) != 1 || list[0].Name != name || list[0].Size != 7 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if data, err := client.get(name); err != nil || string(data) != `{"x":1}` {
		t.Fatalf("get=%q err=%v", data, err)
	}
	if puts.Load() != 1 || hits.Load() < 4 {
		t.Fatalf("puts=%d hits=%d", puts.Load(), hits.Load())
	}
}

// 直连拿到的 HTTP 错误码原样返回，不经节点重试；外层 client 的 CheckRedirect 照常生效。
func TestRoundTripKeepsStatusAndRedirectPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "/releases/tag/v9.9.9", http.StatusFound)
			return
		}
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()
	f := &proxiedFetcher{timeout: 5 * time.Second}
	hits := fakeNodeProxy(t, f, http.NotFoundHandler())
	result, err := checkAppUpdate(context.Background(), f, server.URL+"/releases/latest", "0.1.0")
	if err != nil || !result.HasUpdate || result.LatestVersion != "9.9.9" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	response, err := (&http.Client{Transport: f}).Get(server.URL + "/x")
	if err != nil || response.StatusCode != http.StatusBadGateway {
		t.Fatalf("response=%v err=%v", response, err)
	}
	response.Body.Close()
	if hits.Load() != 0 || f.direct != nil {
		t.Fatalf("HTTP 状态不该触发经节点重试: hits=%d direct=%v", hits.Load(), f.direct)
	}
}

// 没有 GetBody 的请求体也能重放；调用方 ctx 已结束时不再经节点重试。
func TestRoundTripReplaysBodyAndRespectsContext(t *testing.T) {
	f := &proxiedFetcher{timeout: 5 * time.Second}
	var got string
	hits := fakeNodeProxy(t, f, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		got = r.Method + " " + string(data)
	}))
	request, _ := http.NewRequest("REPORT", "http://127.0.0.1:1/x", io.NopCloser(strings.NewReader("body")))
	response, err := f.RoundTrip(request)
	if err != nil || got != "REPORT body" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	response.Body.Close()

	f = &proxiedFetcher{timeout: 5 * time.Second}
	hits = fakeNodeProxy(t, f, http.NotFoundHandler())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request, _ = http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:1/x", nil)
	if _, err := f.RoundTrip(request); err == nil || hits.Load() != 0 {
		t.Fatalf("ctx 已取消仍经节点重试: hits=%d err=%v", hits.Load(), err)
	}
}
