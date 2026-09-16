package app

import (
	"testing"

	"github.com/caichengle666/sbtun/config"
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
