package config

// RoutingMode 定义用户可选择的流量分流模式。
type RoutingMode string

const (
	RoutingSmart    RoutingMode = "smart"
	RoutingGlobal   RoutingMode = "global"
	RoutingDirect   RoutingMode = "direct"
	RoutingCustom   RoutingMode = "custom"
)

// DNSMode 定义 DNS 管理策略。
type DNSMode string

const (
	DNSAuto   DNSMode = "auto"
	DNSSystem DNSMode = "system"
	DNSCustom DNSMode = "custom"
)

// Config 是 sbtun 的用户配置模型。
// 它刻意与 sing-box runtime JSON 解耦，普通用户无需接触 sing-box 配置。
type Config struct {
	Version       int         `json:"version"`
	RoutingMode   RoutingMode `json:"routing_mode"`
	DNSMode       DNSMode     `json:"dns_mode"`
	TUNEnabled    bool        `json:"tun_enabled"`
	CurrentNodeID string      `json:"current_node_id"`
	Nodes         []Node      `json:"nodes"`
}

// Node 是统一的用户节点模型。
type Node struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Protocol string            `json:"protocol"`
	Server   string            `json:"server"`
	Port     uint16            `json:"port"`
	Settings map[string]string `json:"settings,omitempty"`
}

func Default() Config {
	return Config{
		Version:     1,
		RoutingMode: RoutingSmart,
		DNSMode:     DNSAuto,
		Nodes:       []Node{},
	}
}
