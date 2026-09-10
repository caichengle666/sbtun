package tun

import "fmt"

// Manager 是 TUN 生命周期的抽象边界。
// Windows 具体实现后续接入 sing-box TUN 与系统权限/网络恢复逻辑。
type Manager struct {
	running bool
}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) Start() error {
	if m.running {
		return fmt.Errorf("TUN 已经在运行")
	}
	m.running = true
	return nil
}

func (m *Manager) Stop() error {
	m.running = false
	return nil
}

func (m *Manager) Running() bool { return m.running }
