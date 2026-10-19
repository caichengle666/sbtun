package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func newSubscriptionTestApp(t *testing.T) *App {
	t.Helper()
	a := New()
	a.manager = config.NewManager(filepath.Join(t.TempDir(), "config.json"))
	a.runtime = NewRuntimeCoordinator(filepath.Join(t.TempDir(), "runtime-data"), "sing-box")
	return a
}

func nodeIDs(nodes []config.Node) []string {
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids
}

func TestUpdateSubscriptionReplacesNodesAndKeepsCurrent(t *testing.T) {
	var body atomic.Value
	body.Store("socks://127.0.0.1:1080#one\nsocks://127.0.0.1:1081#two")
	fail := atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		w.Write([]byte(body.Load().(string)))
	}))
	defer server.Close()

	a := newSubscriptionTestApp(t)
	if err := a.AddNode(config.Node{ID: "manual", Name: "manual", Protocol: "socks", Server: "10.0.0.1", Port: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ImportSubscription(server.URL + "/sub"); err != nil {
		t.Fatal(err)
	}
	cfg := a.GetConfig()
	if len(cfg.Subscriptions) != 1 {
		t.Fatalf("subscriptions=%+v", cfg.Subscriptions)
	}
	subID := cfg.Subscriptions[0].ID
	second := cfg.Nodes[2].ID
	if err := a.SelectNode(second); err != nil {
		t.Fatal(err)
	}

	body.Store("socks://127.0.0.1:1081#two-renamed\nsocks://127.0.0.1:1082#three")
	result, err := a.UpdateSubscription(subID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.Added != 1 || result.Removed != 1 {
		t.Fatalf("result=%+v", result)
	}
	cfg = a.GetConfig()
	if got := nodeIDs(cfg.Nodes); len(got) != 3 || got[0] != "manual" || got[1] != second {
		t.Fatalf("nodes=%v", got)
	}
	if cfg.Nodes[1].Name != "two-renamed" || cfg.Nodes[1].Subscription != subID {
		t.Fatalf("node not refreshed: %+v", cfg.Nodes[1])
	}
	if cfg.CurrentNodeID != second {
		t.Fatalf("current=%q want %q", cfg.CurrentNodeID, second)
	}

	fail.Store(true)
	if _, err := a.UpdateSubscription(subID); err == nil {
		t.Fatal("expected download error")
	}
	if got := len(a.GetConfig().Nodes); got != 3 {
		t.Fatalf("failed update changed nodes: %d", got)
	}

	if err := a.RemoveSubscription(subID); err != nil {
		t.Fatal(err)
	}
	cfg = a.GetConfig()
	if len(cfg.Subscriptions) != 0 || len(cfg.Nodes) != 1 || cfg.CurrentNodeID != "manual" {
		t.Fatalf("after remove: %+v", cfg)
	}
}

func TestImportSameSubscriptionAdoptsLegacyNodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("socks://127.0.0.1:1080#one"))
	}))
	defer server.Close()

	a := newSubscriptionTestApp(t)
	legacy, err := parseSubscriptionLine("socks://127.0.0.1:1080#old")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.AddNode(legacy); err != nil {
		t.Fatal(err)
	}
	added, err := a.ImportSubscription(server.URL + "/sub")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ImportSubscription(server.URL + "/sub"); err != nil {
		t.Fatal(err)
	}
	cfg := a.GetConfig()
	if added != 0 || len(cfg.Nodes) != 1 || len(cfg.Subscriptions) != 1 || cfg.Nodes[0].Subscription != cfg.Subscriptions[0].ID || cfg.Nodes[0].Name != "one" {
		t.Fatalf("added=%d cfg=%+v", added, cfg)
	}
}

func vlessNode(id, server, path string) config.Node {
	return config.Node{ID: id, Protocol: "vless", Server: server, Port: 443, Settings: map[string]string{"transport_path": path}}
}

func TestMergeSubscriptionNodesSkipsOtherSubscriptions(t *testing.T) {
	x := vlessNode("x", "a.example", "/x")
	x.Subscription = "other"
	y := vlessNode("y", "b.example", "/y")
	y.Subscription = "mine"
	z := vlessNode("z", "c.example", "/z")
	out, stats := mergeSubscriptionNodes([]config.Node{x, y}, "mine", []config.Node{vlessNode("x", "a.example", "/x"), z, z})
	if got := nodeIDs(out); len(got) != 2 || got[0] != "x" || got[1] != "z" || out[0].Subscription != "other" {
		t.Fatalf("out=%+v", out)
	}
	if stats.Total != 1 || stats.Added != 1 || stats.Removed != 1 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestMergeSubscriptionNodesKeepsSameAddressNodes(t *testing.T) {
	manual := vlessNode("vless-cf.example:443", "cf.example", "/manual")
	old := vlessNode("vless-cf.example:443-old", "cf.example", "/a")
	old.Subscription = "mine"
	fresh := []config.Node{
		vlessNode("vless-cf.example:443", "cf.example", "/new"),
		vlessNode("vless-cf.example:443", "cf.example", "/a"),
	}
	out, stats := mergeSubscriptionNodes([]config.Node{manual, old}, "mine", fresh)
	ids := nodeIDs(out)
	if len(ids) != 3 || ids[0] != manual.ID || out[0].Settings["transport_path"] != "/manual" {
		t.Fatalf("manual node replaced: %+v", out)
	}
	// 参数没变的节点保留原 ID，新节点不能抢走被手动节点占用的 ID。
	if ids[2] != old.ID || ids[1] == manual.ID || ids[1] == old.ID {
		t.Fatalf("ids=%v", ids)
	}
	if stats.Total != 2 || stats.Added != 1 || stats.Removed != 0 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestAddNodesKeepsSameAddressDifferentParams(t *testing.T) {
	a := newSubscriptionTestApp(t)
	first := "vless://80f98886-b370-4fd0-8ad5-e567421d79e1@cf.example:443?encryption=none&security=tls&sni=t.example&type=ws&path=%2Fa#A"
	second := "vless://80f98886-b370-4fd0-8ad5-e567421d79e1@cf.example:443?encryption=none&security=tls&sni=t.example&type=ws&path=%2Fb#B"
	for i, link := range []string{first, second, first} {
		added, err := a.ImportSubscription(link)
		if err != nil {
			t.Fatal(err)
		}
		if want := map[int]int{0: 1, 1: 1, 2: 0}[i]; added != want {
			t.Fatalf("import %d added=%d want %d", i, added, want)
		}
	}
	cfg := a.GetConfig()
	if len(cfg.Nodes) != 2 || cfg.Nodes[0].ID == cfg.Nodes[1].ID {
		t.Fatalf("nodes=%+v", cfg.Nodes)
	}
}

func TestUpdateNodeKeepsSubscription(t *testing.T) {
	a := newSubscriptionTestApp(t)
	if err := a.manager.Save(config.Config{Version: 1, RoutingMode: config.RoutingSmart, DNSMode: config.DNSAuto, CurrentNodeID: "n",
		Subscriptions: []config.Subscription{{ID: "s", URL: "https://example.com/sub"}},
		Nodes:         []config.Node{{ID: "n", Name: "n", Protocol: "socks", Server: "a", Port: 1, Subscription: "s"}}}); err != nil {
		t.Fatal(err)
	}
	if err := a.UpdateNode("n", config.Node{Name: "edited", Protocol: "socks", Server: "b", Port: 2}); err != nil {
		t.Fatal(err)
	}
	if got := a.GetConfig().Nodes[0].Subscription; got != "s" {
		t.Fatalf("subscription=%q", got)
	}
}

func TestMergeSubscriptionNodesKeepsManualNodeSharedWithOtherSubscription(t *testing.T) {
	manual := vlessNode("m", "a.example", "/x")
	other := vlessNode("o", "a.example", "/x")
	other.Subscription = "other"
	out, _ := mergeSubscriptionNodes([]config.Node{manual, other}, "mine", []config.Node{vlessNode("n", "a.example", "/x")})
	if got := nodeIDs(out); len(got) != 2 || got[0] != "m" || got[1] != "o" {
		t.Fatalf("out=%+v", out)
	}
}
