package app

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/caichengle666/sbtun/config"
)

const (
	failoverCheckInterval = 30 * time.Second
	// A failed check is confirmed once more after this delay before switching,
	// so a single dropped probe does not move the user to another node.
	failoverConfirmDelay = 5 * time.Second
	// A node that just failed is not picked again for this long.
	failoverCooldown       = 5 * time.Minute
	failoverMinBackoff     = 30 * time.Second
	failoverMaxBackoff     = 5 * time.Minute
	failoverTestTimeout    = 12 * time.Second
	failoverTestWorkers    = 4
	failoverEventRetention = 10 * time.Minute
)

// failoverState is guarded by App.failoverMu.
type failoverState struct {
	next        time.Time
	backoff     time.Duration
	blocked     map[string]time.Time
	health      map[string]NodeHealthDTO
	healthSeq   uint64
	event       AutoSwitchEventDTO
	lastWarning string
}

// AutoSwitchEventDTO is the latest automatic switch attempt shown in the UI.
type AutoSwitchEventDTO struct {
	Seq      uint64 `json:"seq"`
	OK       bool   `json:"ok"`
	FromID   string `json:"from_id,omitempty"`
	FromName string `json:"from_name,omitempty"`
	ToID     string `json:"to_id,omitempty"`
	ToName   string `json:"to_name,omitempty"`
	Message  string `json:"message"`
	At       int64  `json:"at"`
}

// AutoSwitchStatusDTO reports the failover setting and its latest activity.
type AutoSwitchStatusDTO struct {
	Enabled   bool               `json:"enabled"`
	HealthSeq uint64             `json:"health_seq"`
	Event     AutoSwitchEventDTO `json:"event"`
}

// SetAutoSwitch turns automatic node failover on or off.
func (a *App) SetAutoSwitch(enabled bool) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	err := a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		next.AutoSwitchDisabled = !enabled
		return nil
	}, false)
	if err == nil {
		a.failoverMu.Lock()
		a.failover.next, a.failover.backoff = time.Time{}, 0
		a.failoverMu.Unlock()
	}
	return err
}

// GetAutoSwitchStatus returns the setting, the latest switch event and a
// sequence number that changes whenever background tests produce results.
func (a *App) GetAutoSwitchStatus() AutoSwitchStatusDTO {
	status := AutoSwitchStatusDTO{Enabled: true}
	if a.manager != nil {
		if cfg, err := a.manager.Load(); err == nil {
			status.Enabled = !cfg.AutoSwitchDisabled
		}
	}
	a.failoverMu.Lock()
	defer a.failoverMu.Unlock()
	status.HealthSeq = a.failover.healthSeq
	status.Event = a.failover.event
	return status
}

// GetBackgroundNodeHealth returns the latest result of every node tested by
// the failover monitor, so the node list can show those latencies too.
func (a *App) GetBackgroundNodeHealth() []NodeHealthDTO {
	a.failoverMu.Lock()
	defer a.failoverMu.Unlock()
	results := make([]NodeHealthDTO, 0, len(a.failover.health))
	for _, health := range a.failover.health {
		results = append(results, health)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].NodeID < results[j].NodeID })
	return results
}

// recentAutoSwitchEvent returns the latest successful switch if it is recent
// enough to still be worth mentioning in the tray.
func (a *App) recentAutoSwitchEvent() (AutoSwitchEventDTO, bool) {
	a.failoverMu.Lock()
	defer a.failoverMu.Unlock()
	event := a.failover.event
	if event.Seq == 0 || !event.OK || time.Since(time.Unix(event.At, 0)) > failoverEventRetention {
		return AutoSwitchEventDTO{}, false
	}
	return event, true
}

func (a *App) monitorNodes() {
	if a.ctx == nil {
		return
	}
	ticker := time.NewTicker(failoverCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.autoSwitchNode(a.ctx)
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *App) autoSwitchNode(ctx context.Context) {
	if a.runtime == nil || a.runtime.State == nil || a.manager == nil || !a.runtime.State.IsRunning() {
		return
	}
	cfg, err := a.manager.Load()
	if err != nil || cfg.AutoSwitchDisabled {
		return
	}
	a.failoverMu.Lock()
	waiting := time.Now().Before(a.failover.next)
	a.failoverMu.Unlock()
	if waiting {
		return
	}
	current, ok := currentNode(cfg)
	if !ok {
		return
	}
	if a.checkNode(ctx, current).Healthy {
		a.failoverMu.Lock()
		a.failover.backoff, a.failover.lastWarning = 0, ""
		a.failoverMu.Unlock()
		return
	}
	select {
	case <-time.After(failoverConfirmDelay):
	case <-ctx.Done():
		return
	}
	if a.checkNode(ctx, current).Healthy {
		return
	}
	a.blockFailoverNode(current.ID)

	candidates := failoverCandidates(cfg.Nodes, current, a.isFailoverNodeBlocked)
	results := a.checkNodes(ctx, candidates)
	candidate, ok := pickFailoverCandidate(current, candidates, results)
	if !ok {
		a.failoverFailed(current, "", "当前节点不可用，且没有可用的备选节点")
		return
	}

	// Testing above runs without operationMu so manual actions stay
	// responsive; re-check that nothing changed before switching.
	a.operationMu.Lock()
	latest, err := a.manager.Load()
	if err != nil || latest.AutoSwitchDisabled || latest.CurrentNodeID != current.ID || !a.runtime.State.IsRunning() || !hasNode(latest.Nodes, candidate.ID) {
		a.operationMu.Unlock()
		return
	}
	err = a.selectNodeLocked(candidate.ID)
	a.operationMu.Unlock()
	if err != nil {
		a.failoverFailed(current, candidate.Name, fmt.Sprintf("自动切换到 %s 失败: %v", candidate.Name, err))
		return
	}
	message := fmt.Sprintf("%s 不可用，已自动切换到 %s", current.Name, candidate.Name)
	fmt.Println(message)
	a.failoverMu.Lock()
	a.failover.next, a.failover.backoff, a.failover.lastWarning = time.Time{}, 0, ""
	a.setFailoverEventLocked(AutoSwitchEventDTO{OK: true, FromID: current.ID, FromName: current.Name, ToID: candidate.ID, ToName: candidate.Name, Message: message})
	a.failoverMu.Unlock()
}

func (a *App) failoverFailed(current config.Node, target, message string) {
	a.failoverMu.Lock()
	defer a.failoverMu.Unlock()
	a.failover.backoff = nextFailoverBackoff(a.failover.backoff)
	a.failover.next = time.Now().Add(a.failover.backoff)
	// Repeated identical failures (e.g. the network is down) are reported once.
	if message == a.failover.lastWarning {
		return
	}
	a.failover.lastWarning = message
	fmt.Println(message)
	a.setFailoverEventLocked(AutoSwitchEventDTO{FromID: current.ID, FromName: current.Name, ToName: target, Message: message})
}

func (a *App) setFailoverEventLocked(event AutoSwitchEventDTO) {
	event.Seq = a.failover.event.Seq + 1
	event.At = time.Now().Unix()
	a.failover.event = event
}

func nextFailoverBackoff(current time.Duration) time.Duration {
	if current < failoverMinBackoff {
		return failoverMinBackoff
	}
	if current*2 > failoverMaxBackoff {
		return failoverMaxBackoff
	}
	return current * 2
}

func (a *App) checkNode(ctx context.Context, node config.Node) NodeHealthDTO {
	ctx, cancel := context.WithTimeout(ctx, failoverTestTimeout)
	defer cancel()
	health := a.testNode(ctx, node)
	a.failoverMu.Lock()
	if a.failover.health == nil {
		a.failover.health = make(map[string]NodeHealthDTO)
	}
	a.failover.health[node.ID] = health
	a.failover.healthSeq++
	a.failoverMu.Unlock()
	return health
}

func (a *App) checkNodes(ctx context.Context, nodes []config.Node) map[string]NodeHealthDTO {
	results := make(map[string]NodeHealthDTO, len(nodes))
	var mu sync.Mutex
	var wg sync.WaitGroup
	queue := make(chan config.Node)
	for range min(failoverTestWorkers, len(nodes)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for node := range queue {
				health := a.checkNode(ctx, node)
				mu.Lock()
				results[node.ID] = health
				mu.Unlock()
			}
		}()
	}
	for _, node := range nodes {
		queue <- node
	}
	close(queue)
	wg.Wait()
	return results
}

// failoverCandidates lists every node except the current one and nodes still
// cooling down after a failure.
func failoverCandidates(nodes []config.Node, current config.Node, blocked func(string) bool) []config.Node {
	candidates := make([]config.Node, 0, len(nodes))
	for _, node := range nodes {
		if node.ID == current.ID || (blocked != nil && blocked(node.ID)) {
			continue
		}
		candidates = append(candidates, node)
	}
	return candidates
}

// pickFailoverCandidate prefers healthy nodes from the current node's
// subscription (manual nodes count as one group), then the lowest URL delay.
func pickFailoverCandidate(current config.Node, candidates []config.Node, results map[string]NodeHealthDTO) (config.Node, bool) {
	var best config.Node
	found := false
	better := func(node config.Node) bool {
		if !found {
			return true
		}
		sameNode, sameBest := node.Subscription == current.Subscription, best.Subscription == current.Subscription
		if sameNode != sameBest {
			return sameNode
		}
		return failoverLatency(results[node.ID]) < failoverLatency(results[best.ID])
	}
	for _, node := range candidates {
		if !results[node.ID].Healthy {
			continue
		}
		if better(node) {
			best, found = node, true
		}
	}
	return best, found
}

func failoverLatency(health NodeHealthDTO) int64 {
	if health.URL.Latency > 0 {
		return health.URL.Latency
	}
	return 1 << 62
}

func hasNode(nodes []config.Node, id string) bool {
	for _, node := range nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

func (a *App) blockFailoverNode(id string) {
	if id == "" {
		return
	}
	a.failoverMu.Lock()
	defer a.failoverMu.Unlock()
	if a.failover.blocked == nil {
		a.failover.blocked = make(map[string]time.Time)
	}
	a.failover.blocked[id] = time.Now().Add(failoverCooldown)
}

func (a *App) isFailoverNodeBlocked(id string) bool {
	a.failoverMu.Lock()
	defer a.failoverMu.Unlock()
	expires, ok := a.failover.blocked[id]
	if !ok {
		return false
	}
	if !time.Now().Before(expires) {
		delete(a.failover.blocked, id)
		return false
	}
	return true
}
