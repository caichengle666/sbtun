package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/caichengle666/sbtun/config"
)

func TestParseSubscriptionUsage(t *testing.T) {
	u := parseSubscriptionUsage("upload=1073741824; download=2147483648; total=107374182400; expire=1798675200")
	if u == nil || u.Upload != 1<<30 || u.Download != 2<<30 || u.Total != 100<<30 || u.Expire != 1798675200 {
		t.Fatalf("got %+v", u)
	}
	if u := parseSubscriptionUsage("upload=1.5e3;download=0;total=1E10"); u == nil || u.Upload != 1500 || u.Total != 1e10 {
		t.Fatalf("float values: %+v", u)
	}
	for _, h := range []string{"", "upload=10; download=20", "total=abc", "total=-1"} {
		if u := parseSubscriptionUsage(h); u != nil {
			t.Fatalf("%q: want nil, got %+v", h, u)
		}
	}
}

func TestSubscriptionUsageText(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		u    config.SubscriptionUsage
		text string
		warn bool
	}{
		{config.SubscriptionUsage{Upload: 5 << 30, Download: 10 << 30, Total: 100 << 30}, "剩余 85.0 GB / 共 100 GB", false},
		{config.SubscriptionUsage{Download: 95 << 30, Total: 100 << 30}, "剩余 5.0 GB / 共 100 GB", true},
		{config.SubscriptionUsage{Download: 100 << 30, Total: 100 << 30}, "流量已用完", true},
		{config.SubscriptionUsage{Expire: now.Add(-time.Hour).Unix()}, "已过期", true},
		{config.SubscriptionUsage{Expire: now.Add(50 * time.Hour).Unix()}, "3 天后到期", true},
	}
	for _, c := range cases {
		text, warn := SubscriptionUsageText(&c.u, now)
		if text != c.text || warn != c.warn {
			t.Fatalf("%+v: got %q %v, want %q %v", c.u, text, warn, c.text, c.warn)
		}
	}
	if text, _ := SubscriptionUsageText(nil, now); text != "" {
		t.Fatalf("nil usage: %q", text)
	}
}

func TestFetchReturnsUsageHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("subscription-userinfo", "upload=1; download=2; total=300")
		w.Write([]byte("trojan://pw@example.com:443#t"))
	}))
	defer server.Close()
	f := &proxiedFetcher{app: &App{}}
	nodes, err := parseSubscriptionWith(server.URL+"/sub", f.fetch)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("nodes=%v err=%v", nodes, err)
	}
	if f.usage == nil || f.usage.Total != 300 || f.usage.Download != 2 {
		t.Fatalf("usage %+v", f.usage)
	}
}
