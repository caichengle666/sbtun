package tun

import (
	"fmt"
	"sync"
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

	return cleanupPlatform(name)
}

// waitForInterface 等待 TUN 网卡出现。
func waitForInterface(name string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if interfaceExists(name) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
