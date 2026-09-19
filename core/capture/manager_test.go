package capture

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/elazarl/goproxy"
)

func TestManagerForwardsThroughConfiguredUpstream(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("through-upstream"))
	}))
	defer target.Close()

	var upstreamHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		upstreamHits.Add(1)
		outbound := req.Clone(req.Context())
		outbound.RequestURI = ""
		resp, err := http.DefaultTransport.RoundTrip(outbound)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for name, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer upstream.Close()

	manager := NewManager(t.TempDir(), "127.0.0.1:0", strings.TrimPrefix(upstream.URL, "http://"))
	if err := manager.Start(context.Background(), []string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	proxyURL, _ := url.Parse("http://" + manager.Status(true).Address)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if upstreamHits.Load() != 1 || !bytes.Equal(body, []byte("through-upstream")) {
		t.Fatalf("upstream hits=%d body=%q", upstreamHits.Load(), body)
	}
}

func TestManagerForwardsConnectThroughConfiguredUpstream(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secure-through-upstream"))
	}))
	defer target.Close()

	var upstreamHits atomic.Int32
	upstreamProxy := goproxy.NewProxyHttpServer()
	upstreamProxy.OnRequest().HandleConnectFunc(func(host string, _ *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		upstreamHits.Add(1)
		return goproxy.OkConnect, host
	})
	upstream := httptest.NewServer(upstreamProxy)
	defer upstream.Close()

	manager := NewManager(t.TempDir(), "127.0.0.1:0", strings.TrimPrefix(upstream.URL, "http://"))
	if err := manager.Start(context.Background(), []string{"unmatched.invalid"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	proxyURL, _ := url.Parse("http://" + manager.Status(true).Address)
	client := &http.Client{Transport: &http.Transport{
		Proxy:           http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if upstreamHits.Load() != 1 || !bytes.Equal(body, []byte("secure-through-upstream")) {
		t.Fatalf("CONNECT upstream hits=%d body=%q", upstreamHits.Load(), body)
	}
}

func TestManagerCapturesHTTPHeaders(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Response", "ok")
		w.WriteHeader(http.StatusCreated)
	}))
	defer target.Close()

	manager := NewManager(t.TempDir(), "127.0.0.1:0", "")
	if err := manager.Start(context.Background(), []string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	proxyURL, err := url.Parse("http://" + manager.Status(true).Address)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	req, err := http.NewRequest(http.MethodGet, target.URL+"/capture", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "secret")
	req.Header.Set("X-Test", "visible")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	flows, err := manager.Flows()
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 1 {
		t.Fatalf("flows=%d want=1", len(flows))
	}
	flow := flows[0]
	if flow.StatusCode != http.StatusCreated || http.Header(flow.RequestHeaders).Get("Authorization") != "secret" {
		t.Fatalf("unexpected flow: %+v", flow)
	}
	if http.Header(flow.RequestHeaders).Get("X-Test") != "visible" || http.Header(flow.ResponseHeaders).Get("X-Response") != "ok" {
		t.Fatalf("headers not captured: %+v", flow)
	}
}

func TestDomainMatchingIncludesSubdomains(t *testing.T) {
	if !matchesDomain("api.example.com:443", []string{"example.com"}) {
		t.Fatal("subdomain should match")
	}
	if matchesDomain("notexample.com:443", []string{"example.com"}) {
		t.Fatal("unrelated suffix should not match")
	}
	if !matchesDomain("www.google.com:443", []string{"google"}) {
		t.Fatal("keyword should match anywhere in hostname")
	}
	if matchesDomain("www.example.com:443", []string{"google"}) {
		t.Fatal("unrelated keyword should not match")
	}
}

func TestManagerCapturesHTTPSHeadersWithGeneratedCA(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-TLS-Response", "ok")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	targetRoots := x509.NewCertPool()
	targetRoots.AddCert(target.Certificate())

	manager := NewManager(t.TempDir(), "127.0.0.1:0", "")
	manager.transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: targetRoots}}
	if err := manager.Start(context.Background(), []string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	proxyURL, _ := url.Parse("http://" + manager.Status(true).Address)
	caPEM, err := os.ReadFile(manager.certPath())
	if err != nil {
		t.Fatal(err)
	}
	clientRoots := x509.NewCertPool()
	if !clientRoots.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to trust generated capture CA")
	}
	client := &http.Client{Transport: &http.Transport{
		Proxy:           http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{RootCAs: clientRoots},
	}}
	req, _ := http.NewRequest(http.MethodGet, target.URL+"/secure", nil)
	req.Header.Set("Cookie", "session=secret")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	flows, err := manager.Flows()
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 1 {
		t.Fatalf("flows=%d want=1", len(flows))
	}
	flow := flows[0]
	if flow.StatusCode != http.StatusNoContent || http.Header(flow.RequestHeaders).Get("Cookie") != "session=secret" {
		t.Fatalf("unexpected HTTPS flow: %+v", flow)
	}
	if http.Header(flow.ResponseHeaders).Get("X-TLS-Response") != "ok" {
		t.Fatalf("HTTPS response headers missing: %+v", flow.ResponseHeaders)
	}
}

func TestManagerSavesBodiesToJSONAndDeletesCapture(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if string(body) != `{"name":"sbtun"}` {
			t.Errorf("request body=%q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
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
	resp, err := client.Post(target.URL+"/body", "application/json", strings.NewReader(`{"name":"sbtun"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	flows, err := manager.Flows()
	if err != nil || len(flows) != 1 {
		t.Fatalf("flows=%d err=%v", len(flows), err)
	}
	detail, err := manager.Flow(flows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.RequestBody != `{"name":"sbtun"}` || detail.ResponseBody != `{"ok":true}` {
		t.Fatalf("bodies not captured: request=%q response=%q", detail.RequestBody, detail.ResponseBody)
	}
	if err := manager.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manager.storagePath()); err != nil {
		t.Fatal(err)
	}

	reloaded := NewManager(workDir, "127.0.0.1:0", "")
	reloadedFlows, err := reloaded.Flows()
	if err != nil || len(reloadedFlows) != 1 {
		t.Fatalf("reloaded flows=%d err=%v", len(reloadedFlows), err)
	}
	if err := reloaded.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manager.storagePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("capture file should be deleted, err=%v", err)
	}
}
