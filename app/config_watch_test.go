package app

import (
	"os"
	"path/filepath"
	"testing"
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
