package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestLookupExitFallsBack(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("oops")) }))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"ip":"203.0.113.7","country":"jp"}`))
	}))
	defer good.Close()
	old := exitLookupURLs
	exitLookupURLs = []string{bad.URL, good.URL}
	defer func() { exitLookupURLs = old }()
	ip, country := lookupExit(context.Background(), http.DefaultClient)
	if ip != "203.0.113.7" || country != "JP" {
		t.Fatalf("got %q %q", ip, country)
	}
}

func TestMergeKeepsExitForSameNode(t *testing.T) {
	old := []config.Node{{ID: "a", Name: "美国", Protocol: "socks", Server: "1.1.1.1", Port: 1, Subscription: "s", Country: "JP", ExitIP: "203.0.113.1"}}
	fresh := []config.Node{{ID: "a", Name: "美国 改名", Protocol: "socks", Server: "1.1.1.1", Port: 1}, {ID: "b", Name: "新", Protocol: "socks", Server: "2.2.2.2", Port: 1}}
	nodes, _ := mergeSubscriptionNodes(old, "s", fresh)
	if nodes[0].ExitIP != "203.0.113.1" || nodes[0].Country != "JP" || nodes[1].ExitIP != "" {
		t.Fatalf("nodes=%+v", nodes)
	}
}
