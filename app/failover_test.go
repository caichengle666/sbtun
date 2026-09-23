package app

import (
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestFirstHealthyReplacement(t *testing.T) {
	nodes := []config.Node{{ID: "current"}, {ID: "bad"}, {ID: "good"}}
	got, ok := firstHealthyReplacement(nodes, "current", func(node config.Node) bool { return node.ID == "good" })
	if !ok || got.ID != "good" {
		t.Fatalf("replacement=%+v ok=%v", got, ok)
	}
}

func TestFirstHealthyReplacementSkipsCooldownNode(t *testing.T) {
	nodes := []config.Node{{ID: "current"}, {ID: "recently-failed"}, {ID: "good"}}
	blocked := map[string]bool{"recently-failed": true}
	got, ok := firstHealthyReplacementWithCooldown(nodes, "current", func(id string) bool { return blocked[id] }, func(node config.Node) bool { return true })
	if !ok || got.ID != "good" {
		t.Fatalf("replacement=%+v ok=%v", got, ok)
	}
}
