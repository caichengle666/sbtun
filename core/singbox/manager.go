package singbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
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
	logFile  *os.File
	exited   chan ExitEvent
	done     chan struct{}
	stopping bool
	starting bool
}

func NewManager() *Manager { return &Manager{exited: make(chan ExitEvent, 1)} }

func (m *Manager) Start(ctx context.Context, binary, configPath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.starting || (m.cmd != nil && m.cmd.Process != nil) {
		return fmt.Errorf("sing-box 已经在运行或正在启动")
	}
	m.starting = true
	cmd := exec.CommandContext(ctx, binary, "run", "-c", configPath)
	m.stopping = false
	configureProcess(cmd)
	cmd.Dir = filepath.Dir(binary)
	logFile := filepath.Join(filepath.Dir(configPath), "sing-box.log")
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		m.starting = false
		return fmt.Errorf("打开 sing-box 日志失败: %w", err)
	}
	cmd.Stdout = f
	cmd.Stderr = f
	m.logFile = f

	if err := cmd.Start(); err != nil {
		_ = f.Close()
		m.logFile = nil
		m.starting = false
		return fmt.Errorf("启动 sing-box 失败: %w", err)
	}

	m.cmd = cmd
	m.done = make(chan struct{})
	m.starting = false
	go m.wait(cmd)
	return nil
}

func (m *Manager) wait(cmd *exec.Cmd) {
	err := cmd.Wait()
	m.mu.Lock()
	expected := m.stopping
	done := m.done
	ownsProcess := m.cmd == cmd
	if ownsProcess {
		m.cmd = nil
		m.done = nil
	}
	if m.logFile != nil {
		_ = m.logFile.Close()
		m.logFile = nil
	}
	m.stopping = false
	m.mu.Unlock()
	select {
	case m.exited <- ExitEvent{Err: err, Expected: expected}:
	default:
	}
	if done != nil {
		close(done)
	}
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	if m.starting {
		m.mu.Unlock()
		return fmt.Errorf("sing-box 正在启动，暂时无法停止")
	}
	cmd := m.cmd
	done := m.done
	m.stopping = cmd != nil
	if cmd == nil || cmd.Process == nil {
		m.mu.Unlock()
		return nil
	}
	if err := cmd.Process.Kill(); err != nil {
		// Kill 失败时不能继续保留 stopping=true，否则后续异常退出会被误判为预期退出。
		m.stopping = false
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()
	if done == nil {
		return nil
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		return fmt.Errorf("等待 sing-box 退出超时")
	}
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
