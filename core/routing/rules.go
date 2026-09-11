package routing

// Rule 描述用户层路由规则，后续由配置生成器转换成 sing-box route rule。
type Rule struct {
	DomainSuffix []string `json:"domain_suffix,omitempty"`
	IPCIDR       []string `json:"ip_cidr,omitempty"`
	Outbound     string   `json:"outbound"`
}

func Validate(rules []Rule) error {
	for i, rule := range rules {
		if rule.Outbound == "" {
			return &RuleError{Index: i, Message: "规则缺少出口"}
		}
	}
	return nil
}

type RuleError struct { Index int; Message string }
func (e *RuleError) Error() string { return "第 " + itoa(e.Index+1) + " 条路由规则无效: " + e.Message }

func itoa(v int) string {
	if v == 0 { return "0" }
	buf := [20]byte{}
	i := len(buf)
	for v > 0 { i--; buf[i] = byte('0' + v%10); v /= 10 }
	return string(buf[i:])
}
