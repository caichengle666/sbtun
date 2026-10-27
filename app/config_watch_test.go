package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core/rules"
)

func TestConfigStampChangesWithFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if configStamp(path) != "" {
		t.Fatal("missing file should have empty stamp")
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := configStamp(path)
	if err := os.WriteFile(path, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if first == "" || configStamp(path) == first {
		t.Fatal("stamp should change after rewrite")
	}
}

// 运行中的实例也要看到别的进程（CLI）对规则集清单的修改。
func TestWatchStampTracksRuleSets(t *testing.T) {
	dir := t.TempDir()
	a := &App{manager: config.NewManager(filepath.Join(dir, "config.json")), rulesManager: rules.NewManager(filepath.Join(dir, "rules"))}
	before := a.watchStamp()
	if err := rules.NewManager(filepath.Join(dir, "rules")).SetEnabled("geoip-cn", false); err != nil {
		t.Fatal(err)
	}
	if a.watchStamp() == before {
		t.Fatal("sets.json 变化没有被监视到")
	}
}
