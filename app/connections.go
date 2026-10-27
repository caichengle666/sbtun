package app

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/caichengle666/sbtun/config"
)

// ConnectionDTO 是一条正在进行的连接：访问的网站、命中的规则、实际走的出口。
type ConnectionDTO struct {
	ID       string `json:"id"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Network  string `json:"network"`
	Rule     string `json:"rule"`
	Target   string `json:"target"`
	Kind     string `json:"kind"` // direct / proxy / block / capture，界面配色用
	Via      string `json:"via"`  // 经过的节点组，例如“当前节点”“地区 JP”
	Upload   uint64 `json:"upload"`
	Download uint64 `json:"download"`
	Start    string `json:"start"`
}

type clashConnection struct {
	ID       string    `json:"id"`
	Upload   uint64    `json:"upload"`
	Download uint64    `json:"download"`
	Start    time.Time `json:"start"`
	Chains   []string  `json:"chains"`
	Rule     string    `json:"rule"`
	Metadata struct {
		Network         string `json:"network"`
		DestinationIP   string `json:"destinationIP"`
		DestinationPort string `json:"destinationPort"`
		SourcePort      string `json:"sourcePort"`
		Host            string `json:"host"`
	} `json:"metadata"`
}

var errNotRunning = errors.New("代理未运行")

func fetchConnections(ctx context.Context) ([]clashConnection, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clashAPIBaseURL+"/connections", nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(authorizeClash(req))
	if err != nil {
		return nil, errNotRunning
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("读取连接失败: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Connections []clashConnection `json:"connections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Connections, nil
}

// GetConnections 列出当前连接，最新的在前。
func (a *App) GetConnections() ([]ConnectionDTO, error) {
	conns, err := fetchConnections(context.Background())
	if err != nil {
		return nil, err
	}
	sort.SliceStable(conns, func(i, j int) bool { return conns[i].Start.After(conns[j].Start) })
	labels := a.connectionLabeler()
	out := make([]ConnectionDTO, 0, len(conns))
	for _, c := range conns {
		out = append(out, labels.describe(c))
	}
	return out, nil
}

// CloseConnection 断开一条连接，id 为空时断开全部。
func (a *App) CloseConnection(id string) error {
	path := "/connections"
	if id != "" {
		path += "/" + url.PathEscape(id)
	}
	req, err := http.NewRequest(http.MethodDelete, clashAPIBaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(authorizeClash(req))
	if err != nil {
		return errNotRunning
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("断开连接失败: HTTP %d", resp.StatusCode)
	}
	return nil
}

// TestRoute 实际连一次目标网站的 443 端口，从连接列表里读出它命中的规则和出口，结果和浏览器访问一致。
func (a *App) TestRoute(target string) (ConnectionDTO, error) {
	host, err := routeTestHost(target)
	if err != nil {
		return ConnectionDTO{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if _, err := fetchConnections(ctx); err != nil {
		return ConnectionDTO{}, err
	}
	// 先解析一次，免得把不存在的域名误报成被拦截（桌面版这一步本身就经过 sing-box 的 DNS）。
	if net.ParseIP(host) == nil {
		if _, err := net.DefaultResolver.LookupHost(ctx, host); err != nil {
			return ConnectionDTO{}, fmt.Errorf("%s 解析失败：可能被 DNS 过滤规则拦截，或域名不存在", host)
		}
	}
	conn, err := a.routeTestDial(ctx, net.JoinHostPort(host, "443"))
	if err != nil {
		return ConnectionDTO{}, fmt.Errorf("连接 %s 失败: %v", host, err)
	}
	defer conn.Close()
	started := time.Now().Add(-time.Second)
	_, localPort, _ := net.SplitHostPort(conn.LocalAddr().String())
	// 发 TLS ClientHello 让 sing-box 嗅探出域名，再按域名匹配规则。
	handshake := make(chan error, 1)
	go func() {
		handshake <- tls.Client(conn, &tls.Config{ServerName: host, InsecureSkipVerify: true}).HandshakeContext(ctx)
	}()
	labels := a.connectionLabeler()
	ticker := time.NewTicker(60 * time.Millisecond)
	defer ticker.Stop()
	finished := false
	var handshakeErr error
	for {
		conns, _ := fetchConnections(ctx)
		if c, ok := findTestConnection(conns, host, localPort, started); ok {
			result := labels.describe(c)
			result.Host = host
			return result, nil
		}
		if finished {
			if handshakeErr != nil {
				// 连接被当场断开、从没出现在连接列表里：被阻断规则拒绝了。
				return ConnectionDTO{Host: host, Port: "443", Rule: "阻断或过滤规则", Target: "拦截", Kind: "block"}, nil
			}
			return ConnectionDTO{}, fmt.Errorf("没有测到 %s 的连接，这条连接可能没有经过 sbtun", host)
		}
		select {
		case <-ctx.Done():
			return ConnectionDTO{}, fmt.Errorf("没有测到 %s 的连接，请确认代理正在运行", host)
		case handshakeErr = <-handshake:
			finished = true
		case <-ticker.C:
		}
	}
}

// findTestConnection 优先按本地端口认出测试连接；端口对不上（经过 NAT）时退回按域名和 443 端口找刚建立的连接。
func findTestConnection(conns []clashConnection, host, localPort string, since time.Time) (clashConnection, bool) {
	for _, c := range conns {
		if localPort != "" && c.Metadata.SourcePort == localPort {
			return c, true
		}
	}
	for _, c := range conns {
		if strings.EqualFold(c.Metadata.Host, host) && c.Metadata.DestinationPort == "443" && !c.Start.Before(since) {
			return c, true
		}
	}
	return clashConnection{}, false
}

// routeTestHost 接受域名、IP 或网址，去掉协议、路径和端口。
func routeTestHost(target string) (string, error) {
	value := strings.TrimSpace(target)
	if strings.Contains(value, "://") {
		if u, err := url.Parse(value); err == nil {
			value = u.Host
		}
	}
	if i := strings.IndexAny(value, "/?#"); i >= 0 {
		value = value[:i]
	}
	if h, _, err := net.SplitHostPort(value); err == nil {
		value = h
	}
	value = strings.ToLower(strings.Trim(value, "[]."))
	if value == "" || strings.ContainsAny(value, " \t@") {
		return "", errors.New("请输入域名或 IP，例如 google.com")
	}
	return value, nil
}

type connectionLabeler struct {
	cfg      config.Config
	ruleSets map[string]string
}

var (
	ruleSetNamesMu sync.Mutex
	ruleSetNames   = map[string]string{}
)

func (a *App) connectionLabeler() connectionLabeler {
	l := connectionLabeler{}
	if a.manager != nil {
		l.cfg, _ = a.manager.Load()
	}
	ruleSetNamesMu.Lock()
	defer ruleSetNamesMu.Unlock()
	// 别和长时间的操作（测速、更新订阅）抢锁：拿不到就用上次的规则集名称。
	if a.rulesManager != nil && a.operationMu.TryLock() {
		names := map[string]string{}
		for _, set := range a.rulesManager.Sets() {
			names[set.ID] = set.Name
		}
		a.operationMu.Unlock()
		ruleSetNames = names
	}
	l.ruleSets = ruleSetNames
	return l
}

func (l connectionLabeler) describe(c clashConnection) ConnectionDTO {
	host := c.Metadata.Host
	if host == "" {
		host = c.Metadata.DestinationIP
	}
	dto := ConnectionDTO{
		ID: c.ID, Host: host, Port: c.Metadata.DestinationPort, Network: c.Metadata.Network,
		Upload: c.Upload, Download: c.Download, Rule: l.ruleLabel(c.Rule),
	}
	if !c.Start.IsZero() {
		dto.Start = c.Start.Format(time.RFC3339)
	}
	if len(c.Chains) == 0 {
		return dto
	}
	dto.Target, dto.Kind = l.outboundLabel(c.Chains[0])
	for _, group := range c.Chains[1:] {
		switch {
		case group == "proxy":
			dto.Via = "当前节点"
		case strings.HasPrefix(group, "auto-"):
			dto.Via = DescribeAction(l.cfg, strings.TrimPrefix(group, "auto-"))
		}
	}
	return dto
}

func (l connectionLabeler) outboundLabel(tag string) (string, string) {
	switch tag {
	case "direct":
		return "直连", "direct"
	case "block":
		return "拦截", "block"
	case "capture":
		return "流量分析", "capture"
	}
	if id, ok := strings.CutPrefix(tag, "node-"); ok {
		for _, n := range l.cfg.Nodes {
			if n.ID == id {
				return n.Name, "proxy"
			}
		}
	}
	return tag, "proxy"
}

// ruleLabel 把 sing-box 的规则描述（如 "rule_set=geosite-cn => route(direct)"）写成中文。
func (l connectionLabeler) ruleLabel(rule string) string {
	if rule == "" || rule == "final" {
		return "未命中规则（默认）"
	}
	cond, action, _ := strings.Cut(rule, " => ")
	key, value := "", ""
	for _, item := range splitRuleItems(cond) {
		k, v, _ := strings.Cut(item, "=")
		if k == "inbound" {
			continue
		}
		key, value = k, strings.Trim(v, "[]")
		break
	}
	switch key {
	case "rule_set":
		names := strings.Fields(value)
		for i, id := range names {
			if name := l.ruleSets[id]; name != "" {
				names[i] = name
			}
		}
		return "规则集 " + strings.Join(names, "、")
	case "ip_is_private":
		return "局域网地址"
	case "domain", "domain_suffix", "domain_keyword", "ip_cidr", "port":
		if strings.Contains(action, "capture") {
			return "流量分析 " + value
		}
		for _, r := range l.cfg.DNSFilterRules {
			if strings.TrimSpace(r.Value) == value {
				return "过滤规则 " + value
			}
		}
		return "分流规则 " + value
	}
	return cond
}

// splitRuleItems 按空格切分 "a=x b=[y z]"，方括号里的空格不算。
func splitRuleItems(s string) []string {
	var items []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		case ' ':
			if depth == 0 {
				if i > start {
					items = append(items, s[start:i])
				}
				start = i + 1
			}
		}
	}
	if start < len(s) {
		items = append(items, s[start:])
	}
	return items
}

// DescribeAction 把规则去向写成人能看懂的样子，目标没了时提示暂走代理。GUI 连接列表和 CLI 共用。
func DescribeAction(cfg config.Config, action string) string {
	if config.IsBasicAction(action) {
		kind, _ := config.SplitAction(action)
		return map[string]string{"proxy": "代理", "direct": "直连", "block": "阻断"}[kind]
	}
	kind, arg := config.SplitAction(action)
	name := ""
	switch kind {
	case config.TargetNode:
		for _, n := range cfg.Nodes {
			if n.ID == arg {
				name = "节点 " + n.Name
			}
		}
	case config.TargetGroup:
		for _, g := range cfg.NodeGroups {
			if g.ID == arg {
				name = "节点组 " + g.Name
			}
		}
	case config.TargetSub:
		for _, s := range cfg.Subscriptions {
			if s.ID == arg {
				name = "订阅 " + s.Name
			}
		}
	case config.TargetRegion:
		name = "地区 " + arg
	case config.TargetProto:
		name = "协议 " + arg
	}
	if name == "" {
		return action + "（目标已删除，暂走代理）"
	}
	if count := len(cfg.TargetNodes(action)); count == 0 {
		return name + "（没有匹配节点，暂走代理）"
	} else if count > 1 {
		return name + "（" + strconv.Itoa(count) + " 个节点，自动选最快）"
	}
	return name
}
