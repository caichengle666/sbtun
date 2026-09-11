package singbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
)

// ExitEvent 描述 sing-box 进程退出。
type ExitEvent struct {
	Err      error
	Expected bool
}

// Manager 负责 sing-box 进程生命周期，不负责业务配置逻辑。
type Manager struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	exited   chan ExitEvent
	stopping bool
}

func NewManager() *Manager { return &Manager{exited: make(chan ExitEvent, 1)} }

func (m *Manager) Start(ctx context.Context, binary, configPath string) error {
	m.mu.Lock()
	if m.cmd != nil && m.cmd.Process != nil {
		m.mu.Unlock()
		return fmt.Errorf("sing-box 已经在运行")
	}
	cmd := exec.CommandContext(ctx, binary, "run", "-c", configPath)
	m.stopping = false
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	// Set working directory to where sing-box.exe lives so relative paths resolve
	cmd.Dir = filepath.Dir(binary) // CREATE_NO_WINDOW
	// Capture logs to file for debugging
	logFile := filepath.Join(filepath.Dir(configPath), "sing-box.log")
	if f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		cmd.Stdout = f
		cmd.Stderr = f
	}
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 sing-box 失败: %w", err)
	}

	m.mu.Lock()
	m.cmd = cmd
	m.mu.Unlock()
	go m.wait(cmd)
	return nil
}

func (m *Manager) wait(cmd *exec.Cmd) {
	err := cmd.Wait()
	m.mu.Lock()
	expected := m.stopping
	if m.cmd == cmd {
		m.cmd = nil
	}
	m.stopping = false
	m.mu.Unlock()
	select {
	case m.exited <- ExitEvent{Err: err, Expected: expected}:
	default:
	}
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	cmd := m.cmd
	m.stopping = cmd != nil
	m.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Kill(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && m.cmd.Process != nil
}

func (m *Manager) Exited() <-chan ExitEvent { return m.exited }

// InjectExit 仅用于测试：向 exited 通道注入退出事件。
func (m *Manager) InjectExit(err error) {
	m.exited <- ExitEvent{Err: err}
}
