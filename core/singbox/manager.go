package singbox

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
)

// Manager 负责 sing-box 进程生命周期，不负责业务配置逻辑。
type Manager struct {
	mu  sync.Mutex
	cmd *exec.Cmd
}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) Start(ctx context.Context, binary, configPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil && m.cmd.Process != nil {
		return fmt.Errorf("sing-box 已经在运行")
	}
	cmd := exec.CommandContext(ctx, binary, "run", "-c", configPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 sing-box 失败: %w", err)
	}
	m.cmd = cmd
	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd == nil || m.cmd.Process == nil {
		return nil
	}
	err := m.cmd.Process.Kill()
	m.cmd = nil
	return err
}

func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && m.cmd.Process != nil
}
