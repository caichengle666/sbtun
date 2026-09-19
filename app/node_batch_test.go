package app

import (
	"path/filepath"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestRemoveNodesSelectsRemainingNode(t *testing.T) {
	dir := t.TempDir()
	a := New()
	a.manager = config.NewManager(filepath.Join(dir, "config.json"))
	a.runtime = NewRuntimeCoordinator(filepath.Join(dir, "runtime-data"), "sing-box")
	cfg := config.Default()
	cfg.Nodes = []config.Node{
		{ID: "one", Name: "one", Protocol: "socks", Server: "127.0.0.1", Port: 1080},
		{ID: "two", Name: "two", Protocol: "socks", Server: "127.0.0.1", Port: 1081},
		{ID: "three", Name: "three", Protocol: "socks", Server: "127.0.0.1", Port: 1082},
	}
	cfg.CurrentNodeID = "two"
	if err := a.manager.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveNodes([]string{"one", "two"}); err != nil {
		t.Fatal(err)
	}
	got, err := a.manager.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].ID != "three" || got.CurrentNodeID != "three" {
		t.Fatalf("nodes=%+v current=%q", got.Nodes, got.CurrentNodeID)
	}
}

func TestUpdateNodePreservesID(t *testing.T) {
	dir := t.TempDir()
	a := New()
	a.manager = config.NewManager(filepath.Join(dir, "config.json"))
	a.runtime = NewRuntimeCoordinator(filepath.Join(dir, "runtime-data"), "sing-box")
	cfg := config.Default()
	cfg.Nodes = []config.Node{{ID: "one", Name: "old", Protocol: "socks", Server: "127.0.0.1", Port: 1080}}
	cfg.CurrentNodeID = "one"
	if err := a.manager.Save(cfg); err != nil {
		t.Fatal(err)
	}
	node := cfg.Nodes[0]
	node.ID = "changed"
	node.Name = "new"
	if err := a.UpdateNode("one", node); err != nil {
		t.Fatal(err)
	}
	got, err := a.manager.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Nodes[0].ID != "one" || got.Nodes[0].Name != "new" {
		t.Fatalf("node=%+v", got.Nodes[0])
	}
}
