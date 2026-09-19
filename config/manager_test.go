package config

import (
	"path/filepath"
	"testing"
)

func TestManagerSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	m := NewManager(path)
	cfg := Default()
	cfg.Nodes = []Node{{ID: "n1", Name: "测试节点", Protocol: "vless", Server: "example.com", Port: 443}}
	cfg.CurrentNodeID = "n1"
	if err := m.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := m.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentNodeID != "n1" || len(got.Nodes) != 1 {
		t.Fatalf("配置加载结果异常: %+v", got)
	}
}

func TestValidateRejectsDuplicateID(t *testing.T) {
	cfg := Default()
	cfg.Nodes = []Node{
		{ID: "same", Server: "a", Port: 443, Protocol: "vless"},
		{ID: "same", Server: "b", Port: 443, Protocol: "vless"},
	}
	if err := Validate(cfg); err == nil {
		t.Fatal("重复节点 ID 应该校验失败")
	}
}

func TestValidateCaptureRequiresDomain(t *testing.T) {
	cfg := Default()
	cfg.CaptureEnabled = true
	if err := Validate(cfg); err == nil {
		t.Fatal("启用流量分析但没有域名时应校验失败")
	}
	cfg.CaptureDomains = []string{"example.com"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("有效分析域名不应失败: %v", err)
	}
}

func TestManagerUpdatePersistsMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	m := NewManager(path)
	if err := m.Save(Default()); err != nil {
		t.Fatal(err)
	}
	var beforeMode RoutingMode
	err := m.Update(func(before Config, next *Config) error {
		beforeMode = before.RoutingMode
		next.RoutingMode = RoutingGlobal
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if beforeMode != RoutingSmart {
		t.Fatalf("before routing mode = %q, want %q", beforeMode, RoutingSmart)
	}
	got, err := m.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.RoutingMode != RoutingGlobal {
		t.Fatalf("saved routing mode = %q, want %q", got.RoutingMode, RoutingGlobal)
	}
}

func TestManagerUpdateResultReturnsBothSnapshots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	m := NewManager(path)
	if err := m.Save(Default()); err != nil {
		t.Fatal(err)
	}
	before, after, err := m.UpdateResult(func(_ Config, next *Config) error {
		next.RoutingMode = RoutingDirect
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if before.RoutingMode != RoutingSmart || after.RoutingMode != RoutingDirect {
		t.Fatalf("snapshots = %q -> %q", before.RoutingMode, after.RoutingMode)
	}
}
