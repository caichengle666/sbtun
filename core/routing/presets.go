package routing

import "github.com/caichengle666/sbtun/config"

// Preset 描述用户看到的路由模式。
type Preset struct {
	Mode        config.RoutingMode
	Name        string
	Description string
}

func Presets() []Preset {
	return []Preset{
		{Mode: config.RoutingSmart, Name: "智能分流", Description: "国内直连，其他流量自动代理"},
		{Mode: config.RoutingGlobal, Name: "全局代理", Description: "所有非本机流量通过代理"},
		{Mode: config.RoutingDirect, Name: "全局直连", Description: "所有流量直接连接"},
		{Mode: config.RoutingCustom, Name: "自定义", Description: "后续按用户规则精细控制"},
	}
}
