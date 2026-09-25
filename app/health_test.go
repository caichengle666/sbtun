package app

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core"
)

func TestTestNodeOnRunningInstanceUsesClashDelay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxies/node-n1/delay" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("timeout"); got != "5000" {
			t.Errorf("timeout=%q", got)
		}
		if got := r.URL.Query().Get("url"); got != "https://www.gstatic.com/generate_204" {
			t.Errorf("url=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"delay": 123})
	}))
	defer server.Close()

	previousURL := clashAPIBaseURL
	clashAPIBaseURL = server.URL
	defer func() { clashAPIBaseURL = previousURL }()

	result := probeURLViaClash(context.Background(), config.Node{ID: "n1"})
	if !result.OK || result.Latency != 123 || result.Message != "URL 访问成功" {
		t.Fatalf("unexpected health result: %+v", result)
	}
}

func TestTestNodeOnRunningInstanceReportsClashFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "An error occurred in the delay test"})
	}))
	defer server.Close()

	previousURL := clashAPIBaseURL
	clashAPIBaseURL = server.URL
	defer func() { clashAPIBaseURL = previousURL }()

	result := probeURLViaClash(context.Background(), config.Node{ID: "n1"})
	if result.OK || !strings.Contains(result.Message, "delay test") {
		t.Fatalf("unexpected health result: %+v", result)
	}
}

func TestRunningNodeUsesMainInstanceProbe(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(map[string]int{"delay": 42})
	}))
	defer server.Close()

	previousURL := clashAPIBaseURL
	clashAPIBaseURL = server.URL
	defer func() { clashAPIBaseURL = previousURL }()

	a := New()
	a.ctx = context.Background()
	a.runtime = &RuntimeCoordinator{State: core.NewStateStore()}
	a.runtime.State.Set(core.StateRunning, "")

	result := a.testNode(context.Background(), config.Node{ID: "n1", Server: "127.0.0.1", Port: 1})
	if !called || !result.URL.OK || result.URL.Latency != 42 {
		t.Fatalf("unexpected running node result: %+v called=%v", result, called)
	}
}

func TestStoppedCLIUsesExternalClashAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/proxies/proxy":
			_ = json.NewEncoder(w).Encode(map[string]string{"now": "node-n1"})
		case "/proxies/node-n1/delay":
			_ = json.NewEncoder(w).Encode(map[string]int{"delay": 77})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	previousURL := clashAPIBaseURL
	clashAPIBaseURL = server.URL
	defer func() { clashAPIBaseURL = previousURL }()

	a := New()
	a.ctx = context.Background()
	a.runtime = &RuntimeCoordinator{State: core.NewStateStore()}

	result := a.testNode(context.Background(), config.Node{ID: "n1", Server: "127.0.0.1", Port: 1})
	if !result.URL.OK || result.URL.Latency != 77 {
		t.Fatalf("unexpected external clash result: %+v", result)
	}
}

func TestTestNodeReportsReachableTCPPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	address := listener.Addr().(*net.TCPAddr)
	result := testNode(context.Background(), config.Node{ID: "n1", Server: address.IP.String(), Port: uint16(address.Port)})
	if !result.Port.OK || result.Port.Latency < 0 {
		t.Fatalf("unexpected health result: %+v", result)
	}
}

func TestTestNodeReportsUnavailablePort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().(*net.TCPAddr)
	listener.Close()

	result := testNode(context.Background(), config.Node{ID: "n1", Server: address.IP.String(), Port: uint16(address.Port)})
	if result.Port.OK || result.Message == "" {
		t.Fatalf("unexpected health result: %+v", result)
	}
}

func TestTestNodeUsesUDPForHysteria2(t *testing.T) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	address := listener.LocalAddr().(*net.UDPAddr)
	result := testNode(context.Background(), config.Node{ID: "hy2", Protocol: "hysteria2", Server: address.IP.String(), Port: uint16(address.Port)})
	if !result.Port.OK || result.Transport != "udp" || result.Port.Message != "UDP 可发送" {
		t.Fatalf("unexpected hysteria2 health result: %+v", result)
	}
}
