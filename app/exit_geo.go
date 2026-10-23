package app

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/caichengle666/sbtun/config"
)

// 落地 IP 和地区：测速成功、节点还没查过时，经该节点访问查询接口，结果写回节点。
// 订阅更新、编辑节点时参数没变就保留，参数变了清空、下次测速重查。
var exitLookupURLs = []string{"https://api.ip.sb/geoip", "https://ipinfo.io/json", "https://api.country.is/"}

func lookupExit(ctx context.Context, client *http.Client) (ip, country string) {
	for _, u := range exitLookupURLs {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		request.Header.Set("User-Agent", "sbtun")
		response, err := client.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return "", ""
			}
			continue
		}
		var body struct {
			IP          string `json:"ip"`
			Country     string `json:"country"`
			CountryCode string `json:"country_code"`
		}
		err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&body)
		response.Body.Close()
		code := strings.ToUpper(body.CountryCode)
		if code == "" && len(body.Country) == 2 {
			code = strings.ToUpper(body.Country)
		}
		if err == nil && net.ParseIP(body.IP) != nil && len(code) == 2 && code[0] >= 'A' && code[0] <= 'Z' && code[1] >= 'A' && code[1] <= 'Z' {
			return body.IP, code
		}
	}
	return "", ""
}

// detectExit 起一个只带该节点的临时 sing-box 查落地；节点服务器本就绕过 TUN，运行中也能查。
func detectExit(ctx context.Context, binary string, node config.Node) (ip, country string) {
	if binary == "" {
		return "", ""
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	proxy, err := startNodeProxy(ctx, binary, node, 6*time.Second)
	if err != nil {
		return "", ""
	}
	defer proxy.Close()
	if proxy.waitReady(5*time.Second) != nil {
		return "", ""
	}
	return lookupExit(ctx, proxy.client)
}

// saveExits 把这次测速查到的落地写回配置，一批只保存一次。地区变化影响按地区/节点组
// 分流时才重载 sing-box（重载不会重建 TUN）。
func (a *App) saveExits(results []NodeHealthDTO) {
	found := map[string]NodeHealthDTO{}
	for _, r := range results {
		if r.ExitIP != "" {
			found[r.NodeID] = r
		}
	}
	if len(found) == 0 || a.manager == nil {
		return
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	regionChanged := false
	_, after, err := a.manager.UpdateResult(func(_ config.Config, next *config.Config) error {
		for i := range next.Nodes {
			if r, ok := found[next.Nodes[i].ID]; ok {
				regionChanged = regionChanged || next.Nodes[i].Country != r.Country
				next.Nodes[i].Country, next.Nodes[i].ExitIP = r.Country, r.ExitIP
			}
		}
		return nil
	})
	if err == nil && regionChanged && rulesUseRegions(after) {
		a.scheduleExitReload()
	}
}

var exitReload struct {
	sync.Mutex
	timer *time.Timer
}

// scheduleExitReload 等测速告一段落（3 秒内没有新结果）再重载一次，批量测速不会反复重载。
func (a *App) scheduleExitReload() {
	exitReload.Lock()
	defer exitReload.Unlock()
	if exitReload.timer != nil {
		exitReload.timer.Stop()
	}
	exitReload.timer = time.AfterFunc(3*time.Second, func() {
		a.operationMu.Lock()
		defer a.operationMu.Unlock()
		_ = a.reloadIfRunningLocked()
	})
}

func rulesUseRegions(cfg config.Config) bool {
	for _, rule := range cfg.CustomRules {
		kind, arg := config.SplitAction(rule.Action)
		if kind == config.TargetRegion {
			return true
		}
		if kind == config.TargetGroup {
			for _, g := range cfg.NodeGroups {
				if g.ID == arg && len(g.Regions) > 0 {
					return true
				}
			}
		}
	}
	return false
}
