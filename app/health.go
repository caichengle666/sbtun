package app

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
}

type HealthCheckDTO struct {
	OK         bool   `json:"ok"`
	Message    string `json:"message"`
	Latency    int64  `json:"latency_ms"`
	StatusCode int    `json:"status_code,omitempty"`
}

func testNode(ctx context.Context, node config.Node) NodeHealthDTO {
	return testNodeWithBinary(ctx, node, "")
}

func testNodeWithBinary(ctx context.Context, node config.Node, binary string) NodeHealthDTO {
	if ctx == nil {
		ctx = context.Background()
	}
	address := net.JoinHostPort(node.Server, strconv.Itoa(int(node.Port)))
	transport := "tcp"
	if node.Protocol == "hysteria2" {
		transport = "udp"
	}
	result := NodeHealthDTO{
		NodeID:    node.ID,
		Protocol:  node.Protocol,
		Transport: transport,
	}
	result.Ping = pingNode(ctx, node.Server)
	result.Port = probePort(ctx, address, transport)
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

func probeURL(ctx context.Context, binary string, node config.Node) HealthCheckDTO {
	started := time.Now()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return HealthCheckDTO{Message: fmt.Sprintf("URL 测试端口失败: %v", err)}
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	data, err := singbox.BuildHealthConfig(node, port)
	if err != nil {
		return HealthCheckDTO{Message: fmt.Sprintf("URL 配置失败: %v", err)}
	}
	tmpDir, err := os.MkdirTemp("", "sbtun-health-")
	if err != nil {
		return HealthCheckDTO{Message: fmt.Sprintf("URL 临时目录失败: %v", err)}
	}
	defer os.RemoveAll(tmpDir)
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		return HealthCheckDTO{Message: fmt.Sprintf("URL 配置写入失败: %v", err)}
	}
	cmd := exec.CommandContext(ctx, binary, "run", "-c", configPath)
	cmd.Dir = filepath.Dir(binary)
	configureProcess(cmd)
	var logs bytes.Buffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		return HealthCheckDTO{Message: fmt.Sprintf("URL 启动 sing-box 失败: %v", err)}
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	client := &http.Client{
		Timeout:   8 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.gstatic.com/generate_204", nil)
	response, err := waitForURL(client, request, cmd, 10*time.Second)
	if err != nil {
		message := logs.String()
		if len(message) > 240 {
			message = message[len(message)-240:]
		}
		return HealthCheckDTO{Message: fmt.Sprintf("URL 测试失败: %v %s", err, message)}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return HealthCheckDTO{StatusCode: response.StatusCode, Message: fmt.Sprintf("URL 返回 HTTP %d", response.StatusCode)}
	}
	return HealthCheckDTO{OK: true, StatusCode: response.StatusCode, Message: "URL 访问成功", Latency: time.Since(started).Milliseconds()}
}

func waitForURL(client *http.Client, request *http.Request, cmd *exec.Cmd, timeout time.Duration) (*http.Response, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cmd.ProcessState != nil {
			return nil, fmt.Errorf("sing-box 已退出")
		}
		response, err := client.Do(request)
		if err == nil {
			return response, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return nil, fmt.Errorf("等待 URL 代理超时")
}
