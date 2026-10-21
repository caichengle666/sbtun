package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
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
	nodes, usage, err := a.parseSubscription(link)
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
	sub := config.Subscription{ID: subscriptionID(link), Name: subscriptionName(link), URL: link, Usage: usage}
	result, err := a.applySubscriptionLocked(sub, nodes)
	return result.Added, err
}

func (a *App) addNodesLocked(nodes []config.Node) (int, error) {
	added := 0
	err := a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		taken := make(map[string]struct{}, len(next.Nodes))
		seen := make(map[string]struct{}, len(next.Nodes))
		for _, n := range next.Nodes {
			taken[n.ID] = struct{}{}
			seen[nodeFingerprint(n)] = struct{}{}
		}
		added = 0
		for _, n := range nodes {
			fp := nodeFingerprint(n)
			if _, ok := seen[fp]; ok {
				continue
			}
			seen[fp] = struct{}{}
			n.ID = uniqueNodeID(n.ID, fp, taken)
			n.Subscription = ""
			next.Nodes = append(next.Nodes, n)
			added++
		}
		ensureCurrentNode(next)
		return nil
	}, true)
	return added, err
}

// UpdateSubscription 重新下载订阅并用最新节点替换该订阅原有节点。
func (a *App) UpdateSubscription(id string) (SubscriptionUpdateDTO, error) {
	sub, err := a.findSubscription(id)
	if err != nil {
		return SubscriptionUpdateDTO{ID: id}, err
	}
	nodes, usage, err := a.parseSubscription(sub.URL)
	sub.Usage = usage
	if err == nil && len(nodes) == 0 {
		err = errors.New("订阅中未识别到任何节点")
	}
	if err != nil {
		return SubscriptionUpdateDTO{ID: sub.ID, Name: sub.Name}, err
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	// 下载期间订阅可能已被删除。
	if _, err := a.findSubscription(id); err != nil {
		return SubscriptionUpdateDTO{ID: id}, err
	}
	return a.applySubscriptionLocked(sub, nodes)
}

// UpdateAllSubscriptions 依次更新全部订阅；单个失败不影响其他订阅。
// 配置读不出来时返回一条带错误的结果，不能当成“没有订阅”静默返回。
func (a *App) UpdateAllSubscriptions() []SubscriptionUpdateDTO {
	cfg, err := a.LoadConfig()
	if err != nil {
		return []SubscriptionUpdateDTO{{Name: "读取配置", Error: err.Error()}}
	}
	subs := cfg.Subscriptions
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

// findSubscription 查找订阅；配置读取失败时返回真实错误，而不是“订阅不存在”。
func (a *App) findSubscription(id string) (config.Subscription, error) {
	cfg, err := a.LoadConfig()
	if err != nil {
		return config.Subscription{}, fmt.Errorf("读取配置失败: %w", err)
	}
	for _, sub := range cfg.Subscriptions {
		if sub.ID == id {
			return sub, nil
		}
	}
	return config.Subscription{}, fmt.Errorf("订阅不存在: %s", id)
}

func (a *App) applySubscriptionLocked(sub config.Subscription, fresh []config.Node) (SubscriptionUpdateDTO, error) {
	result := SubscriptionUpdateDTO{ID: sub.ID, Name: sub.Name}
	err := a.mutateConfigAndSyncLocked(func(before config.Config, next *config.Config) error {
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
		keepCurrentNodeByName(before, next, sub.ID)
		ensureCurrentNode(next)
		return nil
	}, true)
	return result, err
}

// mergeSubscriptionNodes 用 fresh 替换属于 subID 的节点，并保持其在列表中的位置。
// 节点按参数指纹比对：参数没变的节点沿用原 ID（当前选中不丢）；未归属任何订阅、
// 但参数完全相同的旧节点会被收编；属于其他订阅的相同节点跳过。
func mergeSubscriptionNodes(nodes []config.Node, subID string, fresh []config.Node) ([]config.Node, SubscriptionUpdateDTO) {
	freshFPs := make(map[string]struct{}, len(fresh))
	for _, n := range fresh {
		freshFPs[nodeFingerprint(n)] = struct{}{}
	}
	otherFPs := make(map[string]struct{}) // 其他订阅里节点的指纹
	for _, n := range nodes {
		if n.Subscription != "" && n.Subscription != subID {
			otherFPs[nodeFingerprint(n)] = struct{}{}
		}
	}
	ownedIDs := make(map[string]string) // 指纹 -> 本订阅（或待收编）旧节点的 ID
	taken := make(map[string]struct{}, len(nodes))
	replaced := make([]bool, len(nodes))
	for i, n := range nodes {
		fp := nodeFingerprint(n)
		_, inFresh := freshFPs[fp]
		_, inOther := otherFPs[fp]
		if n.Subscription == subID || n.Subscription == "" && inFresh && !inOther {
			replaced[i] = true
			if _, ok := ownedIDs[fp]; !ok {
				ownedIDs[fp] = n.ID
			}
			continue
		}
		taken[n.ID] = struct{}{}
	}

	incoming := make([]config.Node, 0, len(fresh))
	fps := make([]string, 0, len(fresh))
	bases := make([]string, 0, len(fresh))
	seen := make(map[string]struct{}, len(fresh))
	for _, n := range fresh {
		fp := nodeFingerprint(n)
		if _, ok := seen[fp]; ok {
			continue
		}
		if _, ok := otherFPs[fp]; ok {
			continue
		}
		seen[fp] = struct{}{}
		bases = append(bases, n.ID)
		n.Subscription = subID
		n.ID = ""
		if id, ok := ownedIDs[fp]; ok {
			if _, used := taken[id]; !used {
				n.ID = id
				taken[id] = struct{}{}
			}
		}
		incoming = append(incoming, n)
		fps = append(fps, fp)
	}
	// 先让参数没变的节点占住原 ID，再给新节点分配，避免新节点抢走旧 ID。
	for i := range incoming {
		if incoming[i].ID == "" {
			incoming[i].ID = uniqueNodeID(bases[i], fps[i], taken)
		}
	}

	var stats SubscriptionUpdateDTO
	out := make([]config.Node, 0, len(nodes)+len(incoming))
	inserted := false
	for i, n := range nodes {
		if replaced[i] {
			if _, ok := seen[nodeFingerprint(n)]; !ok {
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
	for _, fp := range fps {
		if _, ok := ownedIDs[fp]; !ok {
			stats.Added++
		}
	}
	stats.Total = len(incoming)
	return out, stats
}

// nodeFingerprint 用协议、地址和全部参数（不含名称）标识节点。同一地址、不同参数的
// 节点（比如 CF 优选里同一域名下不同 path、uuid 的节点）指纹不同，不会被当成重复。
func nodeFingerprint(n config.Node) string {
	keys := make([]string, 0, len(n.Settings))
	for k := range n.Settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%d", n.Protocol, strings.ToLower(n.Server), n.Port)
	for _, k := range keys {
		fmt.Fprintf(h, "\x00%s=%s", k, n.Settings[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:8]
}

// uniqueNodeID 优先使用解析器给出的基础 ID（协议+地址），被占用时加上指纹后缀。
func uniqueNodeID(base, fp string, taken map[string]struct{}) string {
	id := base
	for i := 1; ; i++ {
		if _, ok := taken[id]; !ok {
			taken[id] = struct{}{}
			return id
		}
		if i == 1 {
			id = base + "-" + fp
		} else {
			id = fmt.Sprintf("%s-%s-%d", base, fp, i)
		}
	}
}

// keepCurrentNodeByName：当前选中的订阅节点参数变了（ID 跟着变）时，改选同一订阅里对应的节点，
// 不让选中跳到列表第一个。
func keepCurrentNodeByName(before config.Config, next *config.Config, subID string) {
	var current *config.Node
	for i := range before.Nodes {
		if before.Nodes[i].ID == before.CurrentNodeID {
			current = &before.Nodes[i]
		}
	}
	if current == nil || current.Subscription != subID {
		return
	}
	for _, n := range next.Nodes {
		if n.ID == next.CurrentNodeID {
			return
		}
	}
	// 依次尝试：同名同地址、同名、同地址（名称也变了，比如以前没有名称的 vmess 节点）。
	var sameName, sameAddr string
	for _, n := range next.Nodes {
		if n.Subscription != subID {
			continue
		}
		addr := n.Protocol == current.Protocol && strings.EqualFold(n.Server, current.Server) && n.Port == current.Port
		if n.Name == current.Name && addr {
			next.CurrentNodeID = n.ID
			return
		}
		if n.Name == current.Name && sameName == "" {
			sameName = n.ID
		}
		if addr && sameAddr == "" {
			sameAddr = n.ID
		}
	}
	if id := firstNonEmpty(sameName, sameAddr); id != "" {
		next.CurrentNodeID = id
	}
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
