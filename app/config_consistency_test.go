package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestConfigMutationRollsBackWhenRuntimeSyncFails(t *testing.T) {
	dir := t.TempDir()
	manager := config.NewManager(filepath.Join(dir, "config.json"))
	cfg := config.Default()
	cfg.Nodes = []config.Node{{ID: "one", Name: "one", Protocol: "socks", Server: "127.0.0.1", Port: 1080}}
	cfg.CurrentNodeID = "one"
	if err := manager.Save(cfg); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	a := New()
	a.manager = manager
	a.runtime = NewRuntimeCoordinator(filepath.Join(blocker, "runtime-data"), "sing-box")
	err := a.AddNode(config.Node{ID: "two", Name: "two", Protocol: "socks", Server: "127.0.0.1", Port: 1081})
	if err == nil {
		t.Fatal("AddNode should fail when runtime config cannot be generated")
	}
	got, loadErr := manager.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].ID != "one" || got.CurrentNodeID != "one" {
		t.Fatalf("configuration was not rolled back: %+v", got)
	}
}

func TestGetStatusDoesNotRewriteConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	manager := config.NewManager(path)
	cfg := config.Default()
	cfg.Nodes = []config.Node{{ID: "one", Name: "one", Protocol: "socks", Server: "127.0.0.1", Port: 1080}}
	cfg.CurrentNodeID = "one"
	if err := manager.Save(cfg); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	a := New()
	a.manager = manager
	a.runtime = NewRuntimeCoordinator(filepath.Join(dir, "runtime-data"), "sing-box")
	_ = a.GetStatus()

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("GetStatus rewrote config.json")
	}
}

func TestUninitializedAppReturnsErrors(t *testing.T) {
	a := New()
	_ = a.GetStatus()
	_ = a.GetDiagnostics()
	if err := a.Start(); err == nil {
		t.Fatal("Start should fail before initialization")
	}
	if err := a.Stop(); err == nil {
		t.Fatal("Stop should fail before initialization")
	}
	if err := a.SelectNode("one"); err == nil {
		t.Fatal("SelectNode should fail before initialization")
	}
	if err := a.SaveConfig(config.Default()); err == nil {
		t.Fatal("SaveConfig should fail before initialization")
	}
}
