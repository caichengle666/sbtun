package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/caichengle666/sbtun/config"
)

// 直连下载订阅失败（被墙、连接被重置）时，最多换几个已有节点代理下载。
const subscriptionProxyTries = 3

// parseSubscription 和包级同名函数一样，但订阅地址直连不通时会经已有节点下载。
// 安卓上 App 自身不走 VPN，开着 VPN 也是直连，所以这一步在各平台都需要。
func (a *App) parseSubscription(link string) ([]config.Node, error) {
	fetcher := &proxiedFetcher{app: a}
	defer fetcher.close()
	return parseSubscriptionWith(link, fetcher.fetch)
}

type proxiedFetcher struct {
	app       *App
	direct    error // 直连失败的原因；失败过就不再直连
	proxy     *nodeProxy
	proxyNode string
}

func (f *proxiedFetcher) fetch(link, userAgent string) (string, error) {
	if f.direct == nil {
		body, err := fetchURL(link, userAgent)
		var status httpStatusError
		if err == nil || errors.As(err, &status) {
			return body, err
		}
		f.direct = err
	}
	if f.proxy != nil {
		return f.fetchVia(link, userAgent)
	}
	cfg, err := f.app.LoadConfig()
	if err != nil || f.app.binary == "" || !fileExists(f.app.binary) {
		return "", f.direct
	}
	var errs []string
	for _, node := range proxyCandidates(cfg) {
		proxy, err := startNodeProxy(context.Background(), f.app.binary, node, 20*time.Second)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", node.Name, err))
			continue
		}
		f.proxy, f.proxyNode = proxy, node.Name
		body, err := f.fetchVia(link, userAgent)
		var status httpStatusError
		if err == nil || errors.As(err, &status) {
			return body, err
		}
		errs = append(errs, fmt.Sprintf("%s: %v", node.Name, err))
		f.close()
	}
	if len(errs) == 0 {
		return "", f.direct
	}
	return "", fmt.Errorf("%v；经节点下载也失败（%s）", f.direct, errs[len(errs)-1])
}

func (f *proxiedFetcher) fetchVia(link, userAgent string) (string, error) {
	if err := f.proxy.waitReady(5 * time.Second); err != nil {
		return "", err
	}
	return fetchURLWith(f.proxy.client, link, userAgent)
}

func (f *proxiedFetcher) close() {
	if f.proxy != nil {
		f.proxy.Close()
		f.proxy = nil
	}
}

// proxyCandidates 先用当前节点，再按列表顺序补足。
func proxyCandidates(cfg config.Config) []config.Node {
	var out []config.Node
	for _, node := range cfg.Nodes {
		if node.ID == cfg.CurrentNodeID {
			out = append(out, node)
		}
	}
	for _, node := range cfg.Nodes {
		if len(out) >= subscriptionProxyTries {
			break
		}
		if node.ID != cfg.CurrentNodeID {
			out = append(out, node)
		}
	}
	return out
}
