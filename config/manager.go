package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Manager 负责用户配置的加载、校验和原子保存。
// 用户配置与 sing-box runtime 配置严格分离。
type Manager struct {
	Path string
}

func NewManager(path string) *Manager { return &Manager{Path: path} }

func (m *Manager) Load() (Config, error) {
	if m.Path == "" {
		return Config{}, errors.New("配置文件路径不能为空")
	}
	data, err := os.ReadFile(m.Path)
	if os.IsNotExist(err) {
		cfg := Default()
		if err := m.Save(cfg); err != nil {
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
	if err := os.MkdirAll(filepath.Dir(m.Path), 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.Path), ".sbtun-config-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时配置失败: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("写入配置失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("同步配置失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭配置失败: %w", err)
	}
	if err := os.Rename(tmpName, m.Path); err != nil {
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
	return nil
}
