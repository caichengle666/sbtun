package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The release version lives in three places; a release must update all of them.
func TestVersionMatchesReleaseFiles(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(data))
	if Version() != want {
		t.Fatalf("app version=%s VERSION=%s", Version(), want)
	}
	raw, err := os.ReadFile(filepath.Join("..", "wails.json"))
	if err != nil {
		t.Fatal(err)
	}
	var wails struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(raw, &wails); err != nil {
		t.Fatal(err)
	}
	if wails.Info.ProductVersion != want {
		t.Fatalf("wails productVersion=%s VERSION=%s", wails.Info.ProductVersion, want)
	}
}
