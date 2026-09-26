package rules

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateWithValidationKeepsExistingRuleOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.srs")
	if err := os.WriteFile(path, []byte("old rule"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not an srs rule-set"))
	}))
	defer server.Close()
	manager := &Manager{
		rulesDir:  dir,
		statePath: filepath.Join(dir, "sets.json"),
		sets:      []RuleSet{{ID: "custom", URL: server.URL, Path: path}},
	}
	if err := manager.UpdateWithValidation("custom", func(string) error { return errors.New("invalid srs") }); err == nil {
		t.Fatal("UpdateWithValidation should reject an invalid rule-set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old rule" {
		t.Fatalf("existing rule was replaced: %q", data)
	}
}

func TestAddAndUpdateDoesNotSaveRuleWhenValidationFails(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not an srs rule-set"))
	}))
	defer server.Close()
	manager := NewManager(dir)
	before := len(manager.sets)
	if err := manager.AddAndUpdate("invalid", server.URL, func(string) error { return errors.New("invalid srs") }); err == nil {
		t.Fatal("AddAndUpdate should reject an invalid rule-set")
	}
	if len(manager.sets) != before {
		t.Fatalf("invalid rule-set was added: %+v", manager.sets)
	}
}
