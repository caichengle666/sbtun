package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// sets.json 原子写：保存后内容完整、没有遗留临时文件。
func TestSaveWritesStateAtomically(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.SetEnabled("geosite-cn", false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "sets.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sets []RuleSet
	if err := json.Unmarshal(data, &sets); err != nil {
		t.Fatalf("sets.json 不完整: %v", err)
	}
	if tmp, _ := filepath.Glob(filepath.Join(dir, ".sets-*")); len(tmp) != 0 {
		t.Fatalf("遗留临时文件: %v", tmp)
	}
	list, _ := NewManager(dir).List()
	for _, info := range list {
		if info.ID == "geosite-cn" && info.Enabled {
			t.Fatal("重新加载后禁用状态丢失")
		}
	}
}

// sets.json 损坏时不能悄悄用默认列表覆盖；恢复默认后才允许重新保存。
func TestCorruptStateRefusesSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sets.json")
	corrupt := []byte(`[{"id":"my-ads","name":"广告","url":"https://example.com/a.srs"`)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	m := NewManager(dir)
	if _, err := m.List(); err == nil {
		t.Fatal("应报告清单损坏")
	}
	if err := m.SetEnabled("geosite-cn", false); err == nil {
		t.Fatal("清单损坏时不应保存")
	}
	if data, _ := os.ReadFile(path); string(data) != string(corrupt) {
		t.Fatal("损坏的清单被默认列表覆盖")
	}
	// 从备份整体替换属于用户主动覆盖，允许写入。
	defaults, _ := m.Sets()
	if _, err := m.Replace(defaults); err != nil {
		t.Fatalf("Replace 应能覆盖损坏的清单: %v", err)
	}
	if _, err := m.List(); err != nil {
		t.Fatalf("Replace 后清单应可读: %v", err)
	}
}
