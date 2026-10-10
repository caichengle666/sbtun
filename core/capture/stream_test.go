package capture

import (
	"bufio"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elazarl/goproxy"
)

func waitForFlows(t *testing.T, manager *Manager, want int) []Flow {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		flows, err := manager.Flows()
		if err != nil {
			t.Fatal(err)
		}
		if len(flows) == want || time.Now().After(deadline) {
			if len(flows) != want {
				t.Fatalf("flows=%d want=%d", len(flows), want)
			}
			return flows
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestManagerDoesNotHoldStreamingResponses(t *testing.T) {
	release := make(chan struct{})
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: first\n\n"))
		w.(http.Flusher).Flush()
		<-release
		_, _ = w.Write([]byte("data: second\n\n"))
	}))
	defer target.Close()
	defer close(release)

	manager := NewManager(t.TempDir(), "127.0.0.1:0", "")
	if err := manager.Start(context.Background(), []string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	proxyURL, _ := url.Parse("http://" + manager.Status(true).Address)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}

	type result struct {
		line string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := client.Get(target.URL + "/events")
		if err != nil {
			got <- result{err: err}
			return
		}
		defer resp.Body.Close()
		line, err := bufio.NewReader(resp.Body).ReadString('\n')
		got <- result{line: line, err: err}
	}()
	select {
	case r := <-got:
		if r.err != nil || r.line != "data: first\n" {
			t.Fatalf("first event line=%q err=%v", r.line, r.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("streaming response was held back by capture")
	}
}

func TestManagerBatchesWritesAndFlushesOnStop(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()

	workDir := t.TempDir()
	manager := NewManager(workDir, "127.0.0.1:0", "")
	if err := manager.Start(context.Background(), []string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	proxyURL, _ := url.Parse("http://" + manager.Status(true).Address)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	for i := 0; i < 5; i++ {
		resp, err := client.Get(target.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	waitForFlows(t, manager, 5)
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	if manager.isDirty() {
		t.Fatal("stop should flush captured flows")
	}
	reader := NewManager(workDir, "127.0.0.1:0", "")
	if flows, err := reader.Flows(); err != nil || len(flows) != 5 {
		t.Fatalf("persisted flows=%d err=%v", len(flows), err)
	}
}

func TestManagerHonorsExternalClearWhileRunning(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()

	workDir := t.TempDir()
	manager := NewManager(workDir, "127.0.0.1:0", "")
	if err := manager.Start(context.Background(), []string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	proxyURL, _ := url.Parse("http://" + manager.Status(true).Address)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	get := func() {
		resp, err := client.Get(target.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	get()
	waitForFlows(t, manager, 1)
	if err := manager.Save(); err != nil {
		t.Fatal(err)
	}

	other := NewManager(workDir, "127.0.0.1:0", "")
	if err := other.Clear(); err != nil {
		t.Fatal(err)
	}
	get()
	waitForFlows(t, manager, 2)
	if err := manager.Save(); err != nil {
		t.Fatal(err)
	}
	reader := NewManager(workDir, "127.0.0.1:0", "")
	flows, err := reader.Flows()
	if err != nil || len(flows) != 1 {
		t.Fatalf("flows after external clear=%d err=%v", len(flows), err)
	}
}

func TestManagerRequiresProxyAuth(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()

	manager := NewManager(t.TempDir(), "127.0.0.1:0", "")
	manager.SetAuth(&ProxyAuth{Username: "sbtun", Password: "secret"})
	if err := manager.Start(context.Background(), []string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	address := manager.Status(true).Address

	anonymous, _ := url.Parse("http://" + address)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(anonymous)}}
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("anonymous status=%d want 407", resp.StatusCode)
	}

	withAuth, _ := url.Parse("http://sbtun:secret@" + address)
	client = &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(withAuth)}}
	resp, err = client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("authorized status=%d body=%q", resp.StatusCode, body)
	}
	flows := waitForFlows(t, manager, 1)
	if http.Header(flows[0].RequestHeaders).Get("Proxy-Authorization") != "" {
		t.Fatal("proxy credential must not be recorded")
	}
}

func TestManagerAuthenticatesToUpstream(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("sbtun:secret"))
	var authorizedHits atomic.Int32
	upstreamProxy := goproxy.NewProxyHttpServer()
	upstreamProxy.OnRequest().DoFunc(func(req *http.Request, _ *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if req.Header.Get("Proxy-Authorization") != want {
			return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusProxyAuthRequired, "auth")
		}
		authorizedHits.Add(1)
		return req, nil
	})
	upstream := httptest.NewServer(upstreamProxy)
	defer upstream.Close()

	manager := NewManager(t.TempDir(), "127.0.0.1:0", strings.TrimPrefix(upstream.URL, "http://"))
	manager.SetAuth(&ProxyAuth{Username: "sbtun", Password: "secret"})
	if err := manager.Start(context.Background(), []string{"unmatched.invalid"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	proxyURL, _ := url.Parse("http://sbtun:secret@" + manager.Status(true).Address)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || authorizedHits.Load() != 1 {
		t.Fatalf("status=%d authorized upstream hits=%d", resp.StatusCode, authorizedHits.Load())
	}
}

func TestBodyRecorderTruncatesWithoutBuffering(t *testing.T) {
	var done atomic.Int32
	recorder := newBodyRecorder(io.NopCloser(strings.NewReader("abcdefgh")), 4, func(*bodyRecorder) { done.Add(1) })
	data, err := io.ReadAll(recorder)
	if err != nil || string(data) != "abcdefgh" {
		t.Fatalf("relayed=%q err=%v", data, err)
	}
	_ = recorder.Close()
	body, total, truncated, _ := recorder.snapshot()
	if string(body) != "abcd" || total != 8 || !truncated || done.Load() != 1 {
		t.Fatalf("body=%q total=%d truncated=%v done=%d", body, total, truncated, done.Load())
	}
}
