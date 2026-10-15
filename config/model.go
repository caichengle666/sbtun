package config

// RoutingMode 定义用户可选择的流量分流模式。
type RoutingMode string

const (
	RoutingSmart  RoutingMode = "smart"
	RoutingGlobal RoutingMode = "global"
	RoutingDirect RoutingMode = "direct"
	RoutingCustom RoutingMode = "custom"
)

// DNSMode 定义 DNS 管理策略。
type DNSMode string

const (
	DNSAuto   DNSMode = "auto"
	DNSSystem DNSMode = "system"
	DNSCustom DNSMode = "custom"
)

// Rule 定义用户可理解的自定义分流规则。
type Rule struct {
	MatchType string `json:"match_type"`
	Value     string `json:"value"`
	Action    string `json:"action"`
}

// Config 是 sbtun 的用户配置模型。
// 它刻意与 sing-box runtime JSON 解耦，普通用户无需接触 sing-box 配置。
type Config struct {
	Version                 int            `json:"version"`
	RoutingMode             RoutingMode    `json:"routing_mode"`
	DNSMode                 DNSMode        `json:"dns_mode"`
	TUNEnabled              bool           `json:"tun_enabled"`
	IPv6Enabled             bool           `json:"ipv6_enabled"`
	WebRTCProtectionEnabled bool           `json:"webrtc_protection_enabled,omitempty"`
	DiagnosticsEnabled      bool           `json:"diagnostics_enabled,omitempty"`
	CaptureEnabled          bool           `json:"capture_enabled,omitempty"`
	CaptureDomains          []string       `json:"capture_domains,omitempty"`
	CurrentNodeID           string         `json:"current_node_id"`
	Nodes                   []Node         `json:"nodes"`
	Subscriptions           []Subscription `json:"subscriptions,omitempty"`
	CustomRules             []Rule         `json:"custom_rules,omitempty"`
	DNSFilterRules          []Rule         `json:"dns_filter_rules,omitempty"`
}

// Node 是统一的用户节点模型。
type Node struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Protocol string            `json:"protocol"`
	Server   string            `json:"server"`
	Port     uint16            `json:"port"`
	Settings map[string]string `json:"settings,omitempty"`
	// Subscription 是节点所属订阅的 ID，手动添加的节点为空。
	Subscription string `json:"subscription,omitempty"`
}

// Subscription 记录已导入的远程订阅，用于之后一键更新节点。
type Subscription struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func Default() Config {
	return Config{
		Version:        1,
		RoutingMode:    RoutingSmart,
		DNSMode:        DNSAuto,
		Nodes:          []Node{},
		CustomRules:    []Rule{},
		DNSFilterRules: []Rule{},
		CaptureDomains: []string{},
	}
}
