package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/caichengle666/sbtun/config"
)

// SubscriptionUpdateDTO 是单个订阅的更新结果。
type SubscriptionUpdateDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Total   int    `json:"total"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Error   string `json:"error,omitempty"`
}

func subscriptionID(link string) string {
	sum := sha256.Sum256([]byte(link))
	return "sub-" + hex.EncodeToString(sum[:6])
}

func subscriptionName(link string) string {
	if u, err := url.Parse(link); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return link
}

// ImportSubscription 导入节点链接或订阅地址。订阅地址会被记录下来，之后可一键更新。
func (a *App) ImportSubscription(link string) (int, error) {
	link = normalizeNodeLink(link)
	// 下载订阅可能耗时，放在锁外完成。
	nodes, err := parseSubscription(link)
	if err != nil {
		return 0, err
	}
	if len(nodes) == 0 {
		return 0, errors.New("订阅中未识别到任何节点")
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if !isSubscriptionURL(link) {
		return a.addNodesLocked(nodes)
	}
	sub := config.Subscription{ID: subscriptionID(link), Name: subscriptionName(link), URL: link}
	result, err := a.applySubscriptionLocked(sub, nodes)
	return result.Added, err
}

func (a *App) addNodesLocked(nodes []config.Node) (int, error) {
	added := 0
	err := a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		existing := make(map[string]struct{}, len(next.Nodes))
		for _, n := range next.Nodes {
			existing[n.ID] = struct{}{}
		}
		added = 0
		for _, n := range nodes {
			if _, ok := existing[n.ID]; ok {
				continue
			}
			n.Subscription = ""
			next.Nodes = append(next.Nodes, n)
			existing[n.ID] = struct{}{}
			added++
		}
		ensureCurrentNode(next)
		return nil
	}, true)
	return added, err
}

// UpdateSubscription 重新下载订阅并用最新节点替换该订阅原有节点。
func (a *App) UpdateSubscription(id string) (SubscriptionUpdateDTO, error) {
	sub, ok := a.findSubscription(id)
	if !ok {
		return SubscriptionUpdateDTO{ID: id}, fmt.Errorf("订阅不存在: %s", id)
	}
	nodes, err := parseSubscription(sub.URL)
	if err == nil && len(nodes) == 0 {
		err = errors.New("订阅中未识别到任何节点")
	}
	if err != nil {
		return SubscriptionUpdateDTO{ID: sub.ID, Name: sub.Name}, err
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	// 下载期间订阅可能已被删除。
	if _, ok := a.findSubscription(id); !ok {
		return SubscriptionUpdateDTO{ID: id}, fmt.Errorf("订阅不存在: %s", id)
	}
	return a.applySubscriptionLocked(sub, nodes)
}

// UpdateAllSubscriptions 依次更新全部订阅；单个失败不影响其他订阅。
func (a *App) UpdateAllSubscriptions() []SubscriptionUpdateDTO {
	subs := a.GetConfig().Subscriptions
	results := make([]SubscriptionUpdateDTO, 0, len(subs))
	for _, sub := range subs {
		result, err := a.UpdateSubscription(sub.ID)
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results
}

// RemoveSubscription 删除订阅及其全部节点。
func (a *App) RemoveSubscription(id string) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		subs := make([]config.Subscription, 0, len(next.Subscriptions))
		found := false
		for _, sub := range next.Subscriptions {
			if sub.ID == id {
				found = true
				continue
			}
			subs = append(subs, sub)
		}
		if !found {
			return fmt.Errorf("订阅不存在: %s", id)
		}
		next.Subscriptions = subs
		nodes := make([]config.Node, 0, len(next.Nodes))
		for _, n := range next.Nodes {
			if n.Subscription != id {
				nodes = append(nodes, n)
			}
		}
		next.Nodes = nodes
		ensureCurrentNode(next)
		return nil
	}, true)
}

func (a *App) findSubscription(id string) (config.Subscription, bool) {
	for _, sub := range a.GetConfig().Subscriptions {
		if sub.ID == id {
			return sub, true
		}
	}
	return config.Subscription{}, false
}

func (a *App) applySubscriptionLocked(sub config.Subscription, fresh []config.Node) (SubscriptionUpdateDTO, error) {
	result := SubscriptionUpdateDTO{ID: sub.ID, Name: sub.Name}
	err := a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		sub.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		replaced := false
		for i := range next.Subscriptions {
			if next.Subscriptions[i].ID == sub.ID {
				sub.Name = next.Subscriptions[i].Name
				next.Subscriptions[i] = sub
				replaced = true
			}
		}
		if !replaced {
			next.Subscriptions = append(next.Subscriptions, sub)
		}
		var stats SubscriptionUpdateDTO
		next.Nodes, stats = mergeSubscriptionNodes(next.Nodes, sub.ID, fresh)
		result.Name, result.Total, result.Added, result.Removed = sub.Name, stats.Total, stats.Added, stats.Removed
		ensureCurrentNode(next)
		return nil
	}, true)
	return result, err
}

// mergeSubscriptionNodes 用 fresh 替换属于 subID 的节点，并保持其在列表中的位置。
// 未归属任何订阅、但 ID 与订阅节点相同的旧节点会被收编；属于其他订阅的同 ID 节点保持不动。
func mergeSubscriptionNodes(nodes []config.Node, subID string, fresh []config.Node) ([]config.Node, SubscriptionUpdateDTO) {
	before := make(map[string]struct{}, len(nodes))
	blocked := make(map[string]struct{})
	for _, n := range nodes {
		before[n.ID] = struct{}{}
		if n.Subscription != "" && n.Subscription != subID {
			blocked[n.ID] = struct{}{}
		}
	}
	incoming := make([]config.Node, 0, len(fresh))
	incomingIDs := make(map[string]struct{}, len(fresh))
	for _, n := range fresh {
		if _, ok := blocked[n.ID]; ok {
			continue
		}
		if _, ok := incomingIDs[n.ID]; ok {
			continue
		}
		n.Subscription = subID
		incoming = append(incoming, n)
		incomingIDs[n.ID] = struct{}{}
	}
	var stats SubscriptionUpdateDTO
	out := make([]config.Node, 0, len(nodes)+len(incoming))
	inserted := false
	for _, n := range nodes {
		_, adopt := incomingIDs[n.ID]
		if n.Subscription == subID || n.Subscription == "" && adopt {
			if !adopt {
				stats.Removed++
			}
			if !inserted {
				out = append(out, incoming...)
				inserted = true
			}
			continue
		}
		out = append(out, n)
	}
	if !inserted {
		out = append(out, incoming...)
	}
	for _, n := range incoming {
		if _, ok := before[n.ID]; !ok {
			stats.Added++
		}
	}
	stats.Total = len(incoming)
	return out, stats
}

func ensureCurrentNode(cfg *config.Config) {
	for _, n := range cfg.Nodes {
		if n.ID == cfg.CurrentNodeID {
			return
		}
	}
	cfg.CurrentNodeID = ""
	if len(cfg.Nodes) > 0 {
		cfg.CurrentNodeID = cfg.Nodes[0].ID
	}
}
