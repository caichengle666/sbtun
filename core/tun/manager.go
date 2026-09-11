package tun

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Manager 负责 TUN 网卡和路由的生命周期。
// 启动时记录原始配置，停止时清理路由并恢复网卡状态。
type Manager struct {
	mu      sync.RWMutex
	running bool
	tunName string
}

func NewManager() *Manager { return &Manager{tunName: "tun0"} }

func (m *Manager) MarkRunning() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = true
}

func (m *Manager) MarkStopped() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
}

func (m *Manager) Running() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// WaitReady waits until sing-box has created the configured TUN adapter.
func (m *Manager) WaitReady(timeout time.Duration) error {
	m.mu.RLock()
	name := m.tunName
	m.mu.RUnlock()
	if name == "" {
		name = "tun0"
	}
	if !waitForInterface(name, timeout) {
		return fmt.Errorf("TUN 网卡未出现: %s", name)
	}
	return nil
}

// SetTunName 设置要管理的 TUN 网卡名称。
func (m *Manager) SetTunName(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tunName = name
}

// Cleanup 清理 sing-box 残留的路由和 TUN 网卡配置。
// sing-box 被 kill 后不会自己清理路由，必须由调用方处理。
func (m *Manager) Cleanup() error {
	m.mu.Lock()
	name := m.tunName
	m.mu.Unlock()

	if name == "" {
		name = "tun0"
	}

	var firstErr error
	// 1. 删除默认路由 0.0.0.0 -> TUN 网关
	if err := runWindows("route", "DELETE", "0.0.0.0", "MASK", "0.0.0.0", "172.18.0.1"); err != nil {
		firstErr = err
	}
	// 2. 删除 172.18.0.0/30 子网路由
	_ = runWindows("route", "DELETE", "172.18.0.0", "MASK", "255.255.255.252", "172.18.0.1")
	// 3. 重置 TUN 网卡 IP 为 DHCP（清除静态配置）
	_ = runWindows("netsh", "interface", "ipv4", "set", "address", "name="+name, "source=dhcp")
	// 4. 禁用再启用网卡，强制清除残留状态
	disableInterface(name)
	enableInterface(name)

	return firstErr
}

// disableInterface 禁用指定网卡。
func disableInterface(name string) error {
	return runWindows("netsh", "interface", "set", "interface", name, "admin=disabled")
}

// enableInterface 启用指定网卡。
func enableInterface(name string) error {
	return runWindows("netsh", "interface", "set", "interface", name, "admin=enabled")
}

// runWindows 执行 Windows 命令并隐藏窗口。
func runWindows(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// interfaceIndex 通过 netsh 获取网卡索引。
func interfaceIndex(name string) (int, error) {
	cmd := exec.Command("netsh", "interface", "ipv4", "show", "interfaces")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("读取网络接口失败: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		idx, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		candidate := strings.TrimSpace(strings.Join(fields[4:], " "))
		if strings.EqualFold(candidate, name) {
			return idx, nil
		}
	}
	return 0, fmt.Errorf("找不到接口: %s", name)
}

// waitForInterface 等待 TUN 网卡出现。
func waitForInterface(name string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := interfaceIndex(name); err == nil {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
