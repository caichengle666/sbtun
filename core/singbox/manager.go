package singbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// ExitEvent 描述 sing-box 进程退出。
type ExitEvent struct {
	Err      error
	Expected bool
	// Generation identifies which Start produced the process. A coordinator
	// uses it to ignore an exit that belongs to a process already replaced.
	Generation uint64
}

// Manager 负责 sing-box 进程生命周期，不负责业务配置逻辑。
type Manager struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	pidPath  string
	logPath  string
	logFile  *os.File
	exited   chan ExitEvent
	done     chan struct{}
	stopping bool
	starting bool
	// generation counts successful Starts; current is the generation of the
	// running process, or 0 when none is running.
	generation uint64
	current    uint64
}

// gracefulStopTimeout is how long sing-box gets to remove its routes, rules
// and DNS settings after a stop signal before it is killed.
const gracefulStopTimeout = 3 * time.Second

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
	pidPath := filepath.Join(filepath.Dir(configPath), pidFileName)
	cleanupStalePID(pidPath, binary)
	cmd := exec.CommandContext(ctx, binary, "run", "-c", configPath)
	// ctx 取消（如 CLI 收到 Ctrl+C / SIGTERM）时先请求优雅退出让内核撤掉路由，
	// WaitDelay 后仍未退出才强杀；这次退出按预期退出处理。
	cmd.Cancel = func() error {
		m.markStopping(cmd)
		if err := requestGracefulStop(cmd); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = gracefulStopTimeout
	m.stopping = false
	configureProcess(cmd)
	cmd.Dir = filepath.Dir(binary)
	if runtime.GOOS == "android" {
		// APK 里的内核目录只读，cache.db 要写在数据目录。
		cmd.Dir = filepath.Dir(configPath)
	}
	logFile := filepath.Join(filepath.Dir(configPath), "sing-box.log")
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		m.starting = false
		return fmt.Errorf("打开 sing-box 日志失败: %w", err)
	}
	cmd.Stdout = f
	cmd.Stderr = f
	m.logFile = f
	m.logPath = logFile

	if err := startProcess(cmd); err != nil {
		_ = f.Close()
		m.logFile = nil
		m.starting = false
		return fmt.Errorf("启动 sing-box 失败: %w", err)
	}

	m.cmd = cmd
	m.pidPath = pidPath
	m.done = make(chan struct{})
	m.starting = false
	m.generation++
	m.current = m.generation
	_ = writePIDFile(pidPath, cmd.Process.Pid) // 写失败只影响下次残留清理，不阻止启动
	go m.wait(cmd, m.generation)
	return nil
}

// markStopping 把 cmd 的这次退出标记为预期退出。
func (m *Manager) markStopping(cmd *exec.Cmd) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd == cmd {
		m.stopping = true
	}
}

func (m *Manager) wait(cmd *exec.Cmd, generation uint64) {
	err := cmd.Wait()
	m.mu.Lock()
	if m.pidPath != "" {
		removePIDFile(m.pidPath, cmd.Process.Pid)
	}
	expected := m.stopping
	done := m.done
	ownsProcess := m.cmd == cmd
	if ownsProcess {
		m.cmd = nil
		m.done = nil
		m.current = 0
	}
	if m.logFile != nil {
		_ = m.logFile.Close()
		m.logFile = nil
	}
	m.stopping = false
	m.mu.Unlock()
	select {
	case m.exited <- ExitEvent{Err: err, Expected: expected, Generation: generation}:
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
	m.mu.Unlock()
	if done == nil {
		return killProcess(m, cmd)
	}
	// Ask sing-box to exit on its own first so it can undo auto_route,
	// strict_route rules and DNS changes; kill it only if it does not finish.
	if err := requestGracefulStop(cmd); err == nil {
		timer := time.NewTimer(gracefulStopTimeout)
		select {
		case <-done:
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	if err := killProcess(m, cmd); err != nil {
		return err
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

func killProcess(m *Manager, cmd *exec.Cmd) error {
	if err := cmd.Process.Kill(); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		// Kill 失败时不能继续保留 stopping=true，否则后续异常退出会被误判为预期退出。
		m.mu.Lock()
		if m.cmd == cmd {
			m.stopping = false
		}
		m.mu.Unlock()
		return err
	}
	return nil
}

// Generation returns the generation of the running process, or 0 when no
// process is running.
func (m *Manager) Generation() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
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

// InjectExitEvent 仅用于测试：注入一个完整的退出事件。
func (m *Manager) InjectExitEvent(event ExitEvent) {
	m.exited <- event
}

// SetGenerationForTest 仅用于测试：模拟某一代进程正在运行。
func (m *Manager) SetGenerationForTest(generation uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = generation
	if generation > m.generation {
		m.generation = generation
	}
}

// StartProcess starts cmd bound to sbtun's lifetime (Pdeathsig on Linux, a
// kill-on-close Job on Windows), so a crash never leaves sing-box behind.
func StartProcess(cmd *exec.Cmd) error { return startProcess(cmd) }
