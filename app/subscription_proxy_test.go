package app

import (
	"testing"

	"github.com/caichengle666/sbtun/config"
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
