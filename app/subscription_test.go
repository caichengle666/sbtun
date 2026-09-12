package app

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestParseSubscriptionURLSplitsNodes(t *testing.T) {
	body := "socks://127.0.0.1:1080#one\nsocks://127.0.0.1:1081#two"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(body))))
	}))
	defer server.Close()

	nodes, err := parseSubscription(server.URL + "/sub")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].Port != 1080 || nodes[1].Port != 1081 {
		t.Fatalf("解析节点异常: %+v", nodes)
	}
}

func TestImportSubscriptionSelectsFirstNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("socks://127.0.0.1:1080#one\nsocks://127.0.0.1:1081#two"))
	}))
	defer server.Close()

	a := New()
	a.manager = config.NewManager(filepath.Join(t.TempDir(), "config.json"))
	added, err := a.ImportSubscription(server.URL + "/sub")
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 {
		t.Fatalf("added=%d want=2", added)
	}
	cfg, err := a.manager.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentNodeID != cfg.Nodes[0].ID {
		t.Fatalf("current node=%q first=%q", cfg.CurrentNodeID, cfg.Nodes[0].ID)
	}
}
