package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RuleSet 描述一个分流规则集。
type RuleSet struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
	Source  string `json:"source"`
	Action  string `json:"action"`
}

// RuleInfo 是前端展示的规则集状态。
type RuleInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Enabled   bool   `json:"enabled"`
	Path      string `json:"path"`
	Source    string `json:"source"`
	Action    string `json:"action"`
	CanDelete bool   `json:"can_delete"`
	Exists    bool   `json:"exists"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	UpdatedAt string `json:"updated_at"`
}

// Manager 管理规则集。
type Manager struct {
	rulesDir  string
	statePath string
	sets      []RuleSet
}

// NewManager 创建规则集管理器。
// rulesDir 是程序 exe 所在目录下的 rules/（sing-box 运行时查找的路径）。
func NewManager(rulesDir string) *Manager {
	m := &Manager{rulesDir: rulesDir, statePath: filepath.Join(rulesDir, "sets.json")}
	m.sets = defaultRuleSets(rulesDir)
	m.load()
	return m
}

func defaultRuleSets(rulesDir string) []RuleSet {
	return []RuleSet{
		{
			ID:      "geosite-cn",
			Name:    "国内域名 (geosite)",
			URL:     "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-geolocation-cn.srs",
			Path:    filepath.Join(rulesDir, "geosite-geolocation-cn.srs"),
			Enabled: true,
			Source:  "default",
			Action:  "direct",
		},
		{
			ID:      "geoip-cn",
			Name:    "国内 IP (geoip)",
			URL:     "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-cn.srs",
			Path:    filepath.Join(rulesDir, "geoip-cn.srs"),
			Enabled: true,
			Source:  "default",
			Action:  "direct",
		},
		{
			ID:      "geosite-non-cn",
			Name:    "非国内域名 (geosite)",
			URL:     "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-geolocation-!cn.srs",
			Path:    filepath.Join(rulesDir, "geosite-geolocation-!cn.srs"),
			Enabled: true,
			Source:  "default",
			Action:  "proxy",
		},
	}
}

func (m *Manager) load() {
	data, err := os.ReadFile(m.statePath)
	if err != nil {
		return
	}
	var sets []RuleSet
	if json.Unmarshal(data, &sets) != nil {
		return
	}
	for i := range sets {
		if sets[i].Source == "" {
			sets[i].Source = "custom"
		}
		if sets[i].Action == "" {
			sets[i].Action = "block"
		}
		sets[i].Path = filepath.Join(m.rulesDir, filepath.Base(sets[i].Path))
	}
	m.sets = sets
}

func (m *Manager) save() error {
	if err := os.MkdirAll(m.rulesDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m.sets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.statePath, data, 0o600)
}

// List 返回所有规则集状态。
func (m *Manager) List() []RuleInfo {
	var infos []RuleInfo
	for _, s := range m.sets {
		info := RuleInfo{
			ID:        s.ID,
			Name:      s.Name,
			URL:       s.URL,
			Enabled:   s.Enabled,
			Path:      s.Path,
			Source:    s.Source,
			Action:    s.Action,
			CanDelete: true,
		}
		if fi, err := os.Stat(s.Path); err == nil {
			info.Exists = true
			info.Size = fi.Size()
			info.UpdatedAt = fi.ModTime().Format(time.RFC3339)
			if f, err := os.Open(s.Path); err == nil {
				h := sha256.New()
				io.Copy(h, f)
				f.Close()
				info.SHA256 = hex.EncodeToString(h.Sum(nil))[:16] + "..."
			}
		}
		infos = append(infos, info)
	}
	return infos
}

// Add 添加一个用户自定义规则集，默认按 DNS 阻断规则使用。
func (m *Manager) Add(name, rawURL string) error {
	name = strings.TrimSpace(name)
	rawURL = strings.TrimSpace(rawURL)
	if name == "" || rawURL == "" {
		return fmt.Errorf("规则集名称和 URL 不能为空")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return fmt.Errorf("规则集 URL 必须使用 http:// 或 https://")
	}
	for _, set := range m.sets {
		if set.URL == rawURL {
			return fmt.Errorf("规则集已经存在")
		}
	}
	hash := sha256.Sum256([]byte(rawURL))
	id := "custom-" + hex.EncodeToString(hash[:])[:12]
	fileName := id + ".srs"
	m.sets = append(m.sets, RuleSet{ID: id, Name: name, URL: rawURL, Path: filepath.Join(m.rulesDir, fileName), Enabled: true, Source: "custom", Action: "block"})
	return m.save()
}

// Edit 修改规则集名称和 URL，保留现有规则集 ID 与启用状态。
func (m *Manager) Edit(id, name, rawURL string) error {
	name = strings.TrimSpace(name)
	rawURL = strings.TrimSpace(rawURL)
	if name == "" || rawURL == "" {
		return fmt.Errorf("规则集名称和 URL 不能为空")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return fmt.Errorf("规则集 URL 必须使用 http:// 或 https://")
	}
	for i := range m.sets {
		if m.sets[i].ID == id {
			for j, other := range m.sets {
				if j != i && other.URL == rawURL {
					return fmt.Errorf("规则集已经存在")
				}
			}
			m.sets[i].Name = name
			m.sets[i].URL = rawURL
			return m.save()
		}
	}
	return fmt.Errorf("未知规则集: %s", id)
}

func (m *Manager) SetEnabled(id string, enabled bool) error {
	for i := range m.sets {
		if m.sets[i].ID == id {
			m.sets[i].Enabled = enabled
			return m.save()
		}
	}
	return fmt.Errorf("未知规则集: %s", id)
}

func (m *Manager) Delete(id string) error {
	for i, set := range m.sets {
		if set.ID != id {
			continue
		}
		if err := os.Remove(set.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		m.sets = append(m.sets[:i], m.sets[i+1:]...)
		return m.save()
	}
	return fmt.Errorf("未知规则集: %s", id)
}

func (m *Manager) RestoreDefaults() error {
	defaults := defaultRuleSets(m.rulesDir)
	defaultIDs := make(map[string]bool, len(defaults))
	for _, set := range defaults {
		defaultIDs[set.ID] = true
	}
	custom := make([]RuleSet, 0, len(m.sets))
	for _, set := range m.sets {
		if !defaultIDs[set.ID] {
			custom = append(custom, set)
		}
	}
	m.sets = append(defaults, custom...)
	if err := m.save(); err != nil {
		return err
	}

	var failures []string
	for _, set := range defaults {
		if err := m.Update(set.ID); err != nil {
			failures = append(failures, set.Name+": "+err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("默认规则已恢复，但部分规则下载失败: %s", strings.Join(failures, "; "))
	}
	return nil
}

// Update 下载指定规则集的最新版本。
func (m *Manager) Update(id string) error {
	var target *RuleSet
	for i := range m.sets {
		if m.sets[i].ID == id {
			target = &m.sets[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("未知规则集: %s", id)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, target.URL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "sbtun/0.1.1")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(target.Path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target.Path), ".rule-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, target.Path); err != nil {
		return err
	}
	return nil
}

// UpdateAll 更新所有启用的规则集。
func (m *Manager) UpdateAll() []error {
	var errs []error
	for _, s := range m.sets {
		if !s.Enabled {
			continue
		}
		if err := m.Update(s.ID); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.ID, err))
		}
	}
	return errs
}
