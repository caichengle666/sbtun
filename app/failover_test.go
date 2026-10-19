package app

import (
	"testing"
	"time"

	"github.com/caichengle666/sbtun/config"
)

func healthy(latency int64) NodeHealthDTO {
	return NodeHealthDTO{Healthy: true, URL: HealthCheckDTO{OK: true, Latency: latency}}
}

func TestFailoverCandidatesSkipCurrentAndCooldown(t *testing.T) {
	nodes := []config.Node{{ID: "current"}, {ID: "recently-failed"}, {ID: "good"}}
	got := failoverCandidates(nodes, nodes[0], func(id string) bool { return id == "recently-failed" })
	if len(got) != 1 || got[0].ID != "good" {
		t.Fatalf("candidates=%+v", got)
	}
}

func TestPickFailoverCandidatePrefersLowestLatency(t *testing.T) {
	current := config.Node{ID: "current"}
	candidates := []config.Node{{ID: "slow"}, {ID: "down"}, {ID: "fast"}}
	results := map[string]NodeHealthDTO{"slow": healthy(600), "down": {}, "fast": healthy(120)}
	got, ok := pickFailoverCandidate(current, candidates, results)
	if !ok || got.ID != "fast" {
		t.Fatalf("candidate=%+v ok=%v", got, ok)
	}
}

func TestPickFailoverCandidatePrefersSameSubscription(t *testing.T) {
	current := config.Node{ID: "current", Subscription: "sub-a"}
	candidates := []config.Node{{ID: "other", Subscription: "sub-b"}, {ID: "same", Subscription: "sub-a"}}
	results := map[string]NodeHealthDTO{"other": healthy(50), "same": healthy(400)}
	got, ok := pickFailoverCandidate(current, candidates, results)
	if !ok || got.ID != "same" {
		t.Fatalf("candidate=%+v ok=%v", got, ok)
	}

	// Falls back to another subscription when the own group is down.
	results["same"] = NodeHealthDTO{}
	got, ok = pickFailoverCandidate(current, candidates, results)
	if !ok || got.ID != "other" {
		t.Fatalf("fallback candidate=%+v ok=%v", got, ok)
	}
}

func TestPickFailoverCandidateNoneHealthy(t *testing.T) {
	_, ok := pickFailoverCandidate(config.Node{ID: "current"}, []config.Node{{ID: "a"}}, map[string]NodeHealthDTO{"a": {}})
	if ok {
		t.Fatal("expected no candidate")
	}
}

func TestNextFailoverBackoff(t *testing.T) {
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute}
	var backoff time.Duration
	for i, expected := range want {
		backoff = nextFailoverBackoff(backoff)
		if backoff != expected {
			t.Fatalf("step %d backoff=%s want=%s", i, backoff, expected)
		}
	}
}

func TestFailoverFailedReportsRepeatedMessageOnce(t *testing.T) {
	a := New()
	node := config.Node{ID: "n1", Name: "香港"}
	a.failoverFailed(node, "", "没有可用节点")
	a.failoverFailed(node, "", "没有可用节点")
	status := a.GetAutoSwitchStatus()
	if status.Event.Seq != 1 || status.Event.OK || status.Event.FromName != "香港" {
		t.Fatalf("event=%+v", status.Event)
	}
	if a.failover.backoff != time.Minute {
		t.Fatalf("backoff=%s", a.failover.backoff)
	}
}

func TestCooldownExpires(t *testing.T) {
	a := New()
	a.blockFailoverNode("n1")
	if !a.isFailoverNodeBlocked("n1") {
		t.Fatal("node should be cooling down")
	}
	a.failover.blocked["n1"] = time.Now().Add(-time.Second)
	if a.isFailoverNodeBlocked("n1") {
		t.Fatal("cooldown should have expired")
	}
}
