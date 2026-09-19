package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core"
)

func TestNodeIDFromSelector(t *testing.T) {
	cfg := config.Config{Nodes: []config.Node{{ID: "node-one"}, {ID: "two"}}}
	tests := []struct {
		selector string
		want     string
	}{
		{selector: "node-node-one", want: "node-one"},
		{selector: "node-two", want: "two"},
		{selector: "direct", want: ""},
		{selector: "node-missing", want: ""},
	}
	for _, test := range tests {
		if got := nodeIDFromSelector(cfg, test.selector); got != test.want {
			t.Fatalf("nodeIDFromSelector(%q) = %q, want %q", test.selector, got, test.want)
		}
	}
}

func TestSelectorSyncRetriesUntilClashAPIIsReady(t *testing.T) {
	var putAttempts atomic.Int32
	selected := atomic.Value{}
	selected.Store("node-old")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxies/proxy" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodPut:
			if putAttempts.Add(1) < 3 {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			selected.Store(body.Name)
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]string{"now": selected.Load().(string)})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	previousURL := clashAPIBaseURL
	clashAPIBaseURL = server.URL
	defer func() { clashAPIBaseURL = previousURL }()

	a := New()
	a.ctx = context.Background()
	a.runtime = &RuntimeCoordinator{State: core.NewStateStore()}
	a.runtime.State.Set(core.StateRunning, "")
	a.startSelectorSync("wanted")
	defer a.stopSelectorSync()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, message := a.selectorSyncStatus()
		if state == "synced" {
			if selected.Load().(string) != "node-wanted" {
				t.Fatalf("selector=%q", selected.Load())
			}
			if putAttempts.Load() != 3 {
				t.Fatalf("put attempts=%d", putAttempts.Load())
			}
			return
		}
		if state == "failed" {
			t.Fatalf("selector sync failed: %s", message)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("selector sync did not complete")
}

func TestStopSelectorSyncCancelsRetries(t *testing.T) {
	var putAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		putAttempts.Add(1)
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	previousURL := clashAPIBaseURL
	clashAPIBaseURL = server.URL
	defer func() { clashAPIBaseURL = previousURL }()

	a := New()
	a.ctx = context.Background()
	a.runtime = &RuntimeCoordinator{State: core.NewStateStore()}
	a.runtime.State.Set(core.StateRunning, "")
	a.startSelectorSync("wanted")
	deadline := time.Now().Add(time.Second)
	for putAttempts.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	a.stopSelectorSync()
	afterStop := putAttempts.Load()
	time.Sleep(250 * time.Millisecond)
	if putAttempts.Load() != afterStop {
		t.Fatalf("selector sync continued after stop: before=%d after=%d", afterStop, putAttempts.Load())
	}
	state, message := a.selectorSyncStatus()
	if state != "idle" || message != "" {
		t.Fatalf("selector sync state=%q message=%q", state, message)
	}
}
