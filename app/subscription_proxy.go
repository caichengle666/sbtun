package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/caichengle666/sbtun/config"
)

// 直连请求（订阅、规则集、新版本检查与下载、WebDAV 备份）网络不通（被墙、连接被重置）时，最多换几个已有节点代理。
const subscriptionProxyTries = 3

// parseSubscription 和包级同名函数一样，但订阅地址直连不通时会经已有节点下载，
// 并返回订阅响应头里的流量与到期信息（单个节点链接或机场没给时为 nil）。
// 安卓上 App 自身不走 VPN，开着 VPN 也是直连，所以这一步在各平台都需要。
func (a *App) parseSubscription(link string) ([]config.Node, *config.SubscriptionUsage, error) {
	fetcher := &proxiedFetcher{app: a, timeout: 15 * time.Second}
	defer fetcher.close()
	nodes, err := parseSubscriptionWith(link, fetcher.fetch)
	if err != nil {
		return nil, nil, err
	}
	return nodes, fetcher.usage, nil
}

// proxiedFetcher 先直连；直连网络不通后改经已有节点的临时代理，之后的请求都沿用这个节点。可以并发使用。
type proxiedFetcher struct {
	app     *App
	timeout time.Duration
	mu      sync.Mutex
	direct  error // 直连失败的原因；失败过就不再直连
	proxy   *nodeProxy
	tried   bool                      // 已经试过经节点下载，全失败就不再反复启动临时代理
	usage   *config.SubscriptionUsage // 最近一次成功下载的流量信息，即最终被解析的那份（订阅下载是串行的）
}

func (f *proxiedFetcher) fetch(link, userAgent string) (string, error) {
	var body string
	err := f.do(f.app.baseContext(), func(client *http.Client) error {
		var err error
		body, f.usage, err = fetchURLWith(client, link, userAgent)
		return err
	})
	return body, err
}

// needsProxy 说明错误是不是网络不通（值得换节点再试）；HTTP 状态、内容错误和主动取消都不是。
func needsProxy(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var urlErr *url.Error
	var netErr net.Error
	return errors.As(err, &urlErr) || errors.As(err, &netErr)
}

// do 用直连或节点代理的 client 执行 request，每次尝试各有 f.timeout；ctx 已结束（取消、超时）就不再经节点重试。
func (f *proxiedFetcher) do(ctx context.Context, request func(*http.Client) error) error {
	f.mu.Lock()
	direct := f.direct
	f.mu.Unlock()
	if direct == nil {
		err := request(&http.Client{Timeout: f.timeout})
		if !needsProxy(err) || ctx.Err() != nil {
			return err
		}
		direct = err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.direct == nil {
		f.direct = direct
	}
	if f.proxy != nil {
		return request(f.proxy.client)
	}
	if f.tried || f.app == nil || ctx.Err() != nil {
		return f.direct
	}
	f.tried = true
	cfg, err := f.app.LoadConfig()
	if err != nil || f.app.binary == "" || !fileExists(f.app.binary) {
		return f.direct
	}
	last := ""
	for _, node := range proxyCandidates(cfg) {
		proxy, err := startNodeProxy(f.app.baseContext(), f.app.binary, node, f.timeout+5*time.Second)
		if err == nil {
			if err = proxy.waitReady(5 * time.Second); err == nil {
				f.proxy = proxy
				if err = request(proxy.client); !needsProxy(err) {
					return err
				}
				f.proxy = nil
			}
			proxy.Close()
		}
		last = fmt.Sprintf("%s: %v", node.Name, err)
	}
	if last == "" {
		return f.direct
	}
	return fmt.Errorf("%v；经节点下载也失败（%s）", f.direct, last)
}

// RoundTrip 让任意 http.Client 用上同样的兜底（WebDAV 的 PUT/PROPFIND、带认证的请求都行）：
// 每次尝试都重放请求体；重定向不在这里跟随，交给外层 client，它的 CheckRedirect 照常生效。
func (f *proxiedFetcher) RoundTrip(req *http.Request) (*http.Response, error) {
	getBody := req.GetBody
	if req.Body != nil && req.Body != http.NoBody {
		if getBody == nil {
			data, err := io.ReadAll(req.Body)
			if err != nil {
				req.Body.Close()
				return nil, err
			}
			getBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }
		}
		req.Body.Close()
	}
	var response *http.Response
	err := f.do(req.Context(), func(client *http.Client) error {
		attempt := req.Clone(req.Context())
		if getBody != nil {
			body, err := getBody()
			if err != nil {
				return err
			}
			attempt.Body = body
		}
		once := *client
		once.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		var err error
		response, err = once.Do(attempt)
		return err
	})
	if urlErr, ok := err.(*url.Error); ok {
		err = urlErr.Err // 外层 client 会再包一层 url.Error
	}
	return response, err
}

func (f *proxiedFetcher) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
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
