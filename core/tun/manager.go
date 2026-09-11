package tun

import "sync"

// Manager 只记录 TUN 的已验证状态。
// 真正的 TUN 创建/销毁由 sing-box TUN inbound 完成；这里绝不伪造“启动成功”。
type Manager struct {
	mu      sync.RWMutex
	running bool
}

func NewManager() *Manager { return &Manager{} }

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
