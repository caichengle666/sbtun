package app

import (
	"testing"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core/capture"
)

func TestCaptureRouteLabels(t *testing.T) {
	flow := capture.Flow{Host: "api.example.com", URL: "https://api.example.com/v1"}
	if got := captureRoute(flow, config.Config{RoutingMode: config.RoutingDirect}); got != "direct" {
		t.Fatalf("direct route=%q", got)
	}
	if got := captureRoute(flow, config.Config{RoutingMode: config.RoutingGlobal}); got != "proxy" {
		t.Fatalf("global route=%q", got)
	}
	cfg := config.Config{RoutingMode: config.RoutingCustom, CustomRules: []config.Rule{{MatchType: "domain_suffix", Value: "example.com", Action: "block"}}}
	if got := captureRoute(flow, cfg); got != "block" {
		t.Fatalf("custom route=%q", got)
	}
	flow.Error = "timeout"
	if got := captureRoute(flow, cfg); got != "failed" {
		t.Fatalf("failed route=%q", got)
	}
}

func TestCaptureRouteCustomFallbackAndPort(t *testing.T) {
	flow := capture.Flow{Host: "api.example.com", URL: "https://api.example.com/v1"}
	cfg := config.Config{RoutingMode: config.RoutingCustom, CustomRules: []config.Rule{{MatchType: "port", Value: "443", Action: "direct"}}}
	if got := captureRoute(flow, cfg); got != "direct" {
		t.Fatalf("HTTPS port route=%q", got)
	}
	flow.URL = "https://api.example.com:8443/v1"
	if got := captureRoute(flow, cfg); got != "proxy" {
		t.Fatalf("custom fallback route=%q", got)
	}
}

func TestCaptureRoutePrivateAndSmart(t *testing.T) {
	privateFlow := capture.Flow{Host: "192.168.1.10", URL: "http://192.168.1.10/"}
	if got := captureRoute(privateFlow, config.Config{RoutingMode: config.RoutingGlobal}); got != "direct" {
		t.Fatalf("private route=%q", got)
	}
	publicFlow := capture.Flow{Host: "example.com", URL: "https://example.com/"}
	if got := captureRoute(publicFlow, config.Config{RoutingMode: config.RoutingSmart}); got != "smart" {
		t.Fatalf("smart route=%q", got)
	}
}
