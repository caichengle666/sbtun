package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type Manager struct {
	Path string
	mu   sync.RWMutex
}

func NewManager(path string) *Manager { return &Manager{Path: path} }

func (m *Manager) Load() (Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var cfg Config
	err := withConfigLock(m.Path, func() error {
		var loadErr error
		cfg, loadErr = m.loadLocked()
		return loadErr
	})
	return cfg, err
}

func (m *Manager) loadLocked() (Config, error) {
	if m.Path == "" {
		return Config{}, errors.New("配置文件路径不能为空")
	}
	data, err := os.ReadFile(m.Path)
	if os.IsNotExist(err) {
		cfg := Default()
		if err := m.saveLocked(cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("读取配置失败: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("配置格式错误: %w", err)
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (m *Manager) Save(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return withConfigLock(m.Path, func() error {
		return m.saveLocked(cfg)
	})
}

// Update serializes a read-modify-write operation across processes.
func (m *Manager) Update(mutate func(before Config, next *Config) error) error {
	_, _, err := m.UpdateResult(mutate)
	return err
}

// UpdateResult is like Update but also returns the configuration snapshots on
// both sides of the mutation. Callers can use them to roll back a later runtime
// synchronization failure.
func (m *Manager) UpdateResult(mutate func(before Config, next *Config) error) (Config, Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var before, next Config
	err := withConfigLock(m.Path, func() error {
		var err error
		before, err = m.loadLocked()
		if err != nil {
			return err
		}
		cloned, err := cloneConfig(before)
		if err != nil {
			return err
		}
		if err := mutate(before, &cloned); err != nil {
			return err
		}
		next = cloned
		return m.saveLocked(cloned)
	})
	return before, next, err
}

func (m *Manager) saveLocked(cfg Config) error {
	if m.Path == "" {
		return errors.New("配置文件路径不能为空")
	}
	if err := Validate(cfg); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("编码配置失败: %w", err)
	}
	dir := filepath.Dir(m.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".sbtun-config-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时配置失败: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("写入配置失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("同步配置失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭配置失败: %w", err)
	}
	info, statErr := os.Stat(m.Path)
	if statErr == nil {
		if err := preserveFileMetadata(tmpName, info); err != nil {
			return fmt.Errorf("保留配置权限失败: %w", err)
		}
	} else if os.IsNotExist(statErr) {
		if err := applyInvokerOwnership(tmpName); err != nil {
			return fmt.Errorf("设置配置归属失败: %w", err)
		}
	} else {
		return fmt.Errorf("读取配置文件权限失败: %w", statErr)
	}
	if err := replaceConfigFile(tmpName, m.Path); err != nil {
		return fmt.Errorf("替换配置失败: %w", err)
	}
	return nil
}

func Validate(cfg Config) error {
	if cfg.Version != 1 {
		return fmt.Errorf("不支持的配置版本: %d", cfg.Version)
	}
	switch cfg.RoutingMode {
	case RoutingSmart, RoutingGlobal, RoutingDirect, RoutingCustom:
	default:
		return fmt.Errorf("未知路由模式: %q", cfg.RoutingMode)
	}
	switch cfg.DNSMode {
	case DNSAuto, DNSSystem, DNSCustom:
	default:
		return fmt.Errorf("未知 DNS 模式: %q", cfg.DNSMode)
	}
	seen := make(map[string]struct{}, len(cfg.Nodes))
	for i, n := range cfg.Nodes {
		if n.ID == "" {
			return fmt.Errorf("第 %d 个节点缺少 ID", i+1)
		}
		if _, ok := seen[n.ID]; ok {
			return fmt.Errorf("节点 ID 重复: %s", n.ID)
		}
		seen[n.ID] = struct{}{}
		if n.Server == "" {
			return fmt.Errorf("节点 %q 缺少服务器地址", n.Name)
		}
		if n.Port == 0 {
			return fmt.Errorf("节点 %q 端口无效", n.Name)
		}
		if n.Protocol == "" {
			return fmt.Errorf("节点 %q 缺少协议", n.Name)
		}
	}
	if cfg.CurrentNodeID != "" {
		if _, ok := seen[cfg.CurrentNodeID]; !ok {
			return fmt.Errorf("当前节点不存在: %s", cfg.CurrentNodeID)
		}
	}
	for i, r := range cfg.CustomRules {
		if err := validateRule(r); err != nil {
			return fmt.Errorf("第 %d 条自定义规则无效: %w", i+1, err)
		}
	}
	return nil
}

func validateRule(r Rule) error {
	match := strings.ToLower(strings.TrimSpace(r.MatchType))
	switch match {
	case "domain_suffix", "domain_keyword", "domain", "ip_cidr", "port":
	default:
		return fmt.Errorf("不支持匹配类型 %q", r.MatchType)
	}
	value := strings.TrimSpace(r.Value)
	if value == "" {
		return errors.New("匹配值不能为空")
	}
	action := strings.ToLower(strings.TrimSpace(r.Action))
	if action != "proxy" && action != "direct" && action != "block" {
		return fmt.Errorf("不支持动作 %q", r.Action)
	}
	if match == "port" {
		p, err := strconv.Atoi(value)
		if err != nil || p < 1 || p > 65535 {
			return errors.New("端口必须为 1-65535")
		}
	}
	if match == "ip_cidr" {
		if _, _, err := net.ParseCIDR(value); err != nil {
			return errors.New("IP 网段格式无效")
		}
	}
	return nil
}

func cloneConfig(cfg Config) (Config, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return Config{}, fmt.Errorf("复制配置失败: %w", err)
	}
	var cloned Config
	if err := json.Unmarshal(data, &cloned); err != nil {
		return Config{}, fmt.Errorf("复制配置失败: %w", err)
	}
	return cloned, nil
}
