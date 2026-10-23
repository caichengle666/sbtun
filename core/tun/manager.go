package tun

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Manager 负责 TUN 网卡和路由的生命周期。
// 启动时记录原始配置，停止时清理路由并恢复网卡状态。
type Manager struct {
	mu      sync.RWMutex
	running bool
}

// InterfaceName 是 Windows / Linux 上 sbtun 让 sing-box 创建的 TUN 网卡名。显式指定而不用
// sing-box 默认的 tunN：别的 VPN 或残留网卡占着 tun0 时，sing-box 会悄悄改用 tun1，
// sbtun 就等不到网卡。macOS 由系统分配 utunN，安卓由 VpnService 创建，都不用这个名字。
const InterfaceName = "sbtun"

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

// ErrProcessExited 表示等待网卡期间 sing-box 已经退出。
var ErrProcessExited = errors.New("sing-box 启动后退出")

// WaitReady 等 sing-box 创建好 TUN 网卡；alive 返回 false（进程已退出）时立即返回
// ErrProcessExited，不再傻等到超时。
func (m *Manager) WaitReady(timeout time.Duration, alive func() bool) error {
	name := m.name()
	deadline := time.Now().Add(timeout)
	for !interfaceExists(name) {
		if alive != nil && !alive() {
			return ErrProcessExited
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("TUN 网卡未出现: %s", name)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return configurePlatform(name)
}

func (m *Manager) name() string { return InterfaceName }

// RecoverStale undoes system settings left behind by a run that ended without
// cleanup (for example a killed process). Call it only while no instance runs.
func (m *Manager) RecoverStale() error { return recoverPlatform() }

// Cleanup 清理 sing-box 残留的路由和 TUN 网卡配置。
// sing-box 被 kill 后不会自己清理路由，必须由调用方处理。
func (m *Manager) Cleanup() error {
	return cleanupPlatform(m.name())
}

// CleanupRoutes removes routes and DNS settings without disabling the TUN
// adapter. It is used for an in-process configuration reload, where the new
// sing-box process immediately recreates the adapter.
func (m *Manager) CleanupRoutes() error {
	return cleanupRoutesPlatform(m.name())
}
