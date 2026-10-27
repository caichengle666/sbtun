package rules

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallKeepsExistingRuleOnValidationFailure(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	id, err := m.Add("ads", "https://example.com/ads.srs", "", []byte("old rule"), nil)
	if err != nil {
		t.Fatal(err)
	}
	set, _ := m.Get(id)
	if err := m.Install(id, []byte("new rule"), func(string) error { return errors.New("invalid srs") }); err == nil {
		t.Fatal("Install should reject an invalid rule-set")
	}
	if data, _ := os.ReadFile(set.Path); string(data) != "old rule" {
		t.Fatalf("existing rule was replaced: %q", data)
	}
}

func TestAddDoesNotSaveRuleWhenValidationFails(t *testing.T) {
	m := NewManager(t.TempDir())
	before, _ := m.Sets()
	if _, err := m.Add("invalid", "https://example.com/x.srs", "", []byte("x"), func(string) error { return errors.New("invalid srs") }); err == nil {
		t.Fatal("Add should reject an invalid rule-set")
	}
	if after, _ := m.Sets(); len(after) != len(before) {
		t.Fatalf("invalid rule-set was added: %+v", after)
	}
}

// 两个进程各有一个 Manager：一个添加后，另一个的任何修改都不能用旧列表覆盖掉它。
func TestManagersDoNotOverwriteEachOther(t *testing.T) {
	dir := t.TempDir()
	gui, cli := NewManager(dir), NewManager(dir)
	if _, err := gui.Sets(); err != nil {
		t.Fatal(err)
	}
	id, err := cli.Add("ads", "https://example.com/ads.srs", "block", []byte("rule"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := gui.SetEnabled("geosite-cn", false); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(dir).Get(id); err != nil {
		t.Fatalf("CLI 添加的规则集被覆盖: %v", err)
	}
	if list, _ := gui.List(); len(list) != 4 {
		t.Fatalf("GUI 列表没有读到最新清单: %d 条", len(list))
	}
}

// 删除先保存清单再删文件：保存失败时文件还在。
func TestDeleteKeepsFileWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	id, err := m.Add("ads", "https://example.com/ads.srs", "", []byte("rule"), nil)
	if err != nil {
		t.Fatal(err)
	}
	set, _ := m.Get(id)
	if err := os.WriteFile(m.StatePath(), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(id); err == nil {
		t.Fatal("清单损坏时删除应失败")
	}
	if _, err := os.Stat(set.Path); err != nil {
		t.Fatalf("保存失败却删了规则文件: %v", err)
	}
}

func TestSavedFilesAreReadableByOthers(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	id, err := m.Add("ads", "https://example.com/ads.srs", "", []byte("rule"), nil)
	if err != nil {
		t.Fatal(err)
	}
	set, _ := m.Get(id)
	for _, path := range []string{m.StatePath(), set.Path} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0o644 && filepath.Separator == '/' {
			t.Fatalf("%s 权限 %v，应为 0644", filepath.Base(path), mode)
		}
	}
}

func TestFetchReportsHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	_, err := Fetch(context.Background(), nil, server.URL)
	var status StatusError
	if !errors.As(err, &status) || status != 404 {
		t.Fatalf("err = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Fetch(ctx, nil, server.URL); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消后应立即返回: %v", err)
	}
}
