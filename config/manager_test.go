package config

import (
	"os"
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

// 旧版“自定义”模式升级为全局代理（自己的规则在全局下照样生效）。
func TestLoadMigratesCustomRoutingMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"routing_mode":"custom","dns_mode":"auto","current_node_id":"","nodes":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := NewManager(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RoutingMode != RoutingGlobal {
		t.Fatalf("mode=%s", cfg.RoutingMode)
	}
}
