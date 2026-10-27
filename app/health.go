package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core/singbox"
)

const nodeHealthTimeout = 4 * time.Second

type NodeHealthDTO struct {
	NodeID    string         `json:"node_id"`
	Protocol  string         `json:"protocol"`
	Transport string         `json:"transport"`
	Healthy   bool           `json:"healthy"`
	Message   string         `json:"message"`
	Ping      HealthCheckDTO `json:"ping"`
	Port      HealthCheckDTO `json:"port"`
	URL       HealthCheckDTO `json:"url"`
	// 这次新查到的落地 IP 和地区；节点之前查过就不再查，留空。
	ExitIP  string `json:"exit_ip,omitempty"`
	Country string `json:"country,omitempty"`
}

type HealthCheckDTO struct {
	OK         bool   `json:"ok"`
	Message    string `json:"message"`
	Latency    int64  `json:"latency_ms"`
	StatusCode int    `json:"status_code,omitempty"`
}

func testNodeWithBinary(ctx context.Context, node config.Node, binary string) NodeHealthDTO {
	if ctx == nil {
		ctx = context.Background()
	}
	address := net.JoinHostPort(node.Server, strconv.Itoa(int(node.Port)))
	result := newNodeHealth(node)
	result.Ping = pingNode(ctx, node.Server)
	result.Port = probePort(ctx, address, result.Transport)
	if binary == "" {
		result.Healthy = result.Port.OK
		result.Message = result.Port.Message
		return result
	}
	result.URL = probeURL(ctx, binary, node)
	result.Healthy = result.URL.OK
	if result.Healthy {
		result.Message = "URL 访问成功"
	} else {
		result.Message = result.URL.Message
	}
	return result
}

func testNodeOnRunningInstance(ctx context.Context, node config.Node) NodeHealthDTO {
	if ctx == nil {
		ctx = context.Background()
	}
	result := newNodeHealth(node)
	result.Ping = pingNode(ctx, node.Server)
	result.Port = probePort(ctx, net.JoinHostPort(node.Server, strconv.Itoa(int(node.Port))), result.Transport)
	result.URL = probeURLViaClash(ctx, node)
	result.Healthy = result.URL.OK
	if result.Healthy {
		result.Message = "URL 访问成功"
	} else {
		result.Message = result.URL.Message
	}
	return result
}

func newNodeHealth(node config.Node) NodeHealthDTO {
	transport := "tcp"
	switch node.Protocol {
	case "hysteria", "hysteria2", "tuic", "wireguard":
		transport = "udp"
	}
	return NodeHealthDTO{NodeID: node.ID, Protocol: node.Protocol, Transport: transport}
}

func pingNode(ctx context.Context, host string) HealthCheckDTO {
	started := time.Now()
	pingCtx, cancel := context.WithTimeout(ctx, nodeHealthTimeout)
	defer cancel()
	cmd := pingCommand(pingCtx, host)
	if err := cmd.Run(); err != nil {
		return HealthCheckDTO{Message: fmt.Sprintf("Ping 失败: %v", err)}
	}
	return HealthCheckDTO{OK: true, Message: "Ping 成功", Latency: time.Since(started).Milliseconds()}
}

func probePort(ctx context.Context, address, transport string) HealthCheckDTO {
	started := time.Now()
	probeCtx, cancel := context.WithTimeout(ctx, nodeHealthTimeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(probeCtx, transport, address)
	if err != nil {
		return HealthCheckDTO{Message: fmt.Sprintf("%s 失败: %v", transportLabel(transport), err)}
	}
	_ = conn.Close()
	if transport == "udp" {
		return HealthCheckDTO{OK: true, Message: "UDP 可发送", Latency: time.Since(started).Milliseconds()}
	}
	return HealthCheckDTO{OK: true, Message: "TCPing 成功", Latency: time.Since(started).Milliseconds()}
}

func transportLabel(transport string) string {
	if transport == "udp" {
		return "UDP"
	}
	return "TCPing"
}

// lockedBuffer 收集 sing-box 输出；exec 的拷贝协程在写，tail 可能同时在读，需要加锁。
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// nodeProxy 是只带一个节点的临时 sing-box 进程，对外提供本地 HTTP 代理。
type nodeProxy struct {
	addr   string
	cmd    *exec.Cmd
	exited chan struct{} // cmd.Wait 返回后关闭；ProcessState 只有 Wait 之后才有值，不能拿来判断进程是否已退出
	dir    string
	logs   lockedBuffer
	client *http.Client
}

func startNodeProxy(ctx context.Context, binary string, node config.Node, timeout time.Duration) (*nodeProxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("端口失败: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	user, err := randomToken(8)
	if err != nil {
		return nil, err
	}
	pass, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	data, err := singbox.BuildHealthConfig(node, port, user, pass)
	if err != nil {
		return nil, fmt.Errorf("配置失败: %v", err)
	}
	dir, err := os.MkdirTemp("", "sbtun-health-")
	if err != nil {
		return nil, fmt.Errorf("临时目录失败: %v", err)
	}
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("配置写入失败: %v", err)
	}
	p := &nodeProxy{dir: dir, addr: fmt.Sprintf("127.0.0.1:%d", port)}
	p.cmd = exec.CommandContext(ctx, binary, "run", "-c", configPath)
	p.cmd.Dir = filepath.Dir(binary)
	configureProcess(p.cmd)
	p.cmd.Stdout = &p.logs
	p.cmd.Stderr = &p.logs
	// 绑定到 sbtun 的生命周期：sbtun 崩溃或被强杀时临时代理一起结束。
	if err := singbox.StartProcess(p.cmd); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("启动 sing-box 失败: %v", err)
	}
	p.exited = make(chan struct{})
	go func() {
		_ = p.cmd.Wait()
		close(p.exited)
	}()
	proxyURL := &url.URL{Scheme: "http", Host: p.addr, User: url.UserPassword(user, pass)}
	p.client = &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	return p, nil
}

// waitReady 等本地代理端口可以连上。
func (p *nodeProxy) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-p.exited:
			return fmt.Errorf("sing-box 已退出 %s", p.tail())
		default:
		}
		conn, err := net.DialTimeout("tcp", p.addr, 300*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("等待本地代理超时 %s", p.tail())
		}
		select {
		case <-p.exited:
			return fmt.Errorf("sing-box 已退出 %s", p.tail())
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// tail 返回 sing-box 日志的末尾，便于在错误里说明原因。
func (p *nodeProxy) tail() string {
	message := p.logs.String()
	if len(message) > 240 {
		message = message[len(message)-240:]
	}
	return message
}

func (p *nodeProxy) Close() {
	p.client.CloseIdleConnections()
	_ = p.cmd.Process.Kill()
	<-p.exited
	_ = os.RemoveAll(p.dir)
}

func probeURL(ctx context.Context, binary string, node config.Node) HealthCheckDTO {
	started := time.Now()
	proxy, err := startNodeProxy(ctx, binary, node, 8*time.Second)
	if err != nil {
		return HealthCheckDTO{Message: "URL " + err.Error()}
	}
	defer proxy.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.gstatic.com/generate_204", nil)
	if err != nil {
		return HealthCheckDTO{Message: fmt.Sprintf("URL 请求创建失败: %v", err)}
	}
	response, err := waitForURL(ctx, proxy.client, request, proxy.exited, 10*time.Second)
	if err != nil {
		return HealthCheckDTO{Message: fmt.Sprintf("URL 测试失败: %v %s", err, proxy.tail())}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return HealthCheckDTO{StatusCode: response.StatusCode, Message: fmt.Sprintf("URL 返回 HTTP %d", response.StatusCode)}
	}
	return HealthCheckDTO{OK: true, StatusCode: response.StatusCode, Message: "URL 访问成功", Latency: time.Since(started).Milliseconds()}
}

func probeURLViaClash(ctx context.Context, node config.Node) HealthCheckDTO {
	endpoint := clashAPIBaseURL + "/proxies/" + url.PathEscape("node-"+node.ID) + "/delay"
	query := url.Values{}
	query.Set("timeout", "5000")
	query.Set("url", "https://www.gstatic.com/generate_204")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return HealthCheckDTO{Message: fmt.Sprintf("URL 请求创建失败: %v", err)}
	}
	client := &http.Client{Timeout: 8 * time.Second}
	response, err := client.Do(authorizeClash(request))
	if err != nil {
		return HealthCheckDTO{Message: fmt.Sprintf("URL 测试失败: %v", err)}
	}
	defer response.Body.Close()
	var payload struct {
		Delay   int64  `json:"delay"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return HealthCheckDTO{Message: fmt.Sprintf("URL 测试失败: 解析响应失败: %v", err)}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := payload.Message
		if message == "" {
			message = response.Status
		}
		return HealthCheckDTO{Message: fmt.Sprintf("URL 测试失败: %s", message)}
	}
	if payload.Delay <= 0 {
		return HealthCheckDTO{Message: "URL 测试失败: sing-box 未返回有效延迟"}
	}
	return HealthCheckDTO{OK: true, StatusCode: http.StatusNoContent, Message: "URL 访问成功", Latency: payload.Delay}
}

// waitForURL 反复请求直到成功；exited 关闭说明 sing-box 已退出，立即报错，不白等到超时。
func waitForURL(ctx context.Context, client *http.Client, request *http.Request, exited <-chan struct{}, timeout time.Duration) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		response, err := client.Do(request.WithContext(waitCtx))
		if err == nil {
			return response, nil
		}
		select {
		case <-exited:
			return nil, fmt.Errorf("sing-box 已退出")
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("等待 URL 代理超时: %w", waitCtx.Err())
		case <-time.After(150 * time.Millisecond):
		}
	}
}
