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
	Version                 int         `json:"version"`
	RoutingMode             RoutingMode `json:"routing_mode"`
	DNSMode                 DNSMode     `json:"dns_mode"`
	TUNEnabled              bool        `json:"tun_enabled"`
	IPv6Enabled             bool        `json:"ipv6_enabled"`
	WebRTCProtectionEnabled bool        `json:"webrtc_protection_enabled,omitempty"`
	DiagnosticsEnabled      bool        `json:"diagnostics_enabled,omitempty"`
	// AutoSwitchDisabled 关闭当前节点失效时的自动切换（默认开启）。
	AutoSwitchDisabled bool           `json:"auto_switch_disabled,omitempty"`
	CaptureEnabled     bool           `json:"capture_enabled,omitempty"`
	CaptureDomains     []string       `json:"capture_domains,omitempty"`
	CurrentNodeID      string         `json:"current_node_id"`
	Nodes              []Node         `json:"nodes"`
	Subscriptions      []Subscription `json:"subscriptions,omitempty"`
	// 订阅默认每 24 小时自动更新；SubscriptionUpdateHours 为 0 时用默认间隔。
	SubscriptionAutoUpdateDisabled bool   `json:"subscription_auto_update_disabled,omitempty"`
	SubscriptionUpdateHours        int    `json:"subscription_update_hours,omitempty"`
	CustomRules                    []Rule `json:"custom_rules,omitempty"`
	// NodeGroups 是规则可以直接指向的节点组（group:<ID>）。
	NodeGroups     []NodeGroup `json:"node_groups,omitempty"`
	DNSFilterRules []Rule      `json:"dns_filter_rules,omitempty"`
	// WebDAV 是云端备份设置，只保存在本机，不会写进备份文件。
	WebDAV *WebDAVSettings `json:"webdav,omitempty"`
}

// WebDAVSettings 是备份到 WebDAV（坚果云、Nextcloud、Alist、NAS 等）所需的信息。
type WebDAVSettings struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	// EncryptPassword 不为空时备份文件用它加密，恢复时需要同一个密码。
	EncryptPassword string `json:"encrypt_password,omitempty"`
	// AutoBackup 开启后，节点、订阅、规则或设置有变化时自动上传一份。
	AutoBackup bool `json:"auto_backup,omitempty"`
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
	// Country 是落地地区代码（US、HK…）：测速查到的落地 IP 归属地，没测过时按节点名推测。
	Country string `json:"country,omitempty"`
	// ExitIP 是测速时经该节点查到的落地 IP，为空表示 Country 只是按名字猜的。
	ExitIP string `json:"exit_ip,omitempty"`
}

// Subscription 记录已导入的远程订阅，用于之后一键更新节点。
type Subscription struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at,omitempty"`
	// Usage 来自订阅响应头 subscription-userinfo；机场没给就是 nil。
	Usage *SubscriptionUsage `json:"usage,omitempty"`
}

// SubscriptionUsage 是订阅的流量与到期信息。字节数；Expire 为 Unix 秒，0 表示长期有效或未知。
type SubscriptionUsage struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire,omitempty"`
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
