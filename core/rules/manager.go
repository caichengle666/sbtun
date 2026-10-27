package rules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/caichengle666/sbtun/config"
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

// RuleSetValidator validates a downloaded temporary rule-set before install.
type RuleSetValidator func(path string) error

// Manager 管理规则集。GUI、CLI 和运行中的实例共用磁盘上的 sets.json：
// 读取总是重新读文件，修改在文件锁里“重读 → 修改 → 保存”，不会用旧列表覆盖别的进程的修改。
type Manager struct {
	mu        sync.Mutex
	rulesDir  string
	statePath string
	sets      []RuleSet
	// loadErr 记录 sets.json 存在但读不出或解析失败；此时拒绝保存，免得默认列表覆盖用户的自定义规则集。
	loadErr error
}

// NewManager 创建规则集管理器。
// rulesDir 是程序 exe 所在目录下的 rules/（sing-box 运行时查找的路径）。
func NewManager(rulesDir string) *Manager {
	return &Manager{rulesDir: rulesDir, statePath: filepath.Join(rulesDir, "sets.json")}
}

// StatePath 是 sets.json 的路径，运行中的实例据此监视别的进程的修改。
func (m *Manager) StatePath() string { return m.statePath }

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

// reloadLocked 从磁盘重新读取清单；文件不存在时是默认列表。sets.json 总是原子替换，读取不需要文件锁。
func (m *Manager) reloadLocked() {
	m.sets, m.loadErr = defaultRuleSets(m.rulesDir), nil
	data, err := os.ReadFile(m.statePath)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		m.loadErr = fmt.Errorf("读取规则集清单失败: %w", err)
		return
	}
	var sets []RuleSet
	if err := json.Unmarshal(data, &sets); err != nil {
		m.loadErr = fmt.Errorf("规则集清单 %s 已损坏（%v），为免覆盖自定义规则集已停止保存，可恢复默认规则或从备份恢复", m.statePath, err)
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

// mutate 在文件锁里重读清单、执行 fn、保存。overwrite 为 true 时允许覆盖读不出的旧清单（恢复默认、从备份替换）。
func (m *Manager) mutate(overwrite bool, fn func() error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return config.WithFileLock(m.statePath, func() error {
		m.reloadLocked()
		if m.loadErr != nil && !overwrite {
			return m.loadErr
		}
		m.loadErr = nil
		if err := fn(); err != nil {
			return err
		}
		return m.save()
	})
}

// save 先写同目录临时文件再原子替换，崩溃或并发读取时不会出现半截的 sets.json。
// 清单不含密钥，0644 并在 sudo/pkexec 下交还调用者，普通用户的 CLI 也能读。
func (m *Manager) save() error {
	data, err := json.MarshalIndent(m.sets, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(m.rulesDir, 0o755); err != nil {
		return err
	}
	_ = config.ShareWithInvoker(m.rulesDir, 0o755)
	tmp, err := os.CreateTemp(m.rulesDir, ".sets-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := config.ShareWithInvoker(tmp.Name(), 0o644); err != nil {
		return err
	}
	return config.ReplaceConfigFile(tmp.Name(), m.statePath)
}

// Sets 返回磁盘上最新的规则集列表；清单读不出时返回错误（列表是默认值，不能拿去备份或展示成用户的列表）。
func (m *Manager) Sets() ([]RuleSet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadLocked()
	return append([]RuleSet(nil), m.sets...), m.loadErr
}

// Get 返回指定规则集的最新状态。
func (m *Manager) Get(id string) (RuleSet, error) {
	sets, err := m.Sets()
	if err != nil {
		return RuleSet{}, err
	}
	for _, set := range sets {
		if set.ID == id {
			return set, nil
		}
	}
	return RuleSet{}, fmt.Errorf("未知规则集: %s", id)
}

func (m *Manager) index(id string) (int, error) {
	for i := range m.sets {
		if m.sets[i].ID == id {
			return i, nil
		}
	}
	return -1, fmt.Errorf("未知规则集: %s", id)
}

// Replace 用备份里的规则集列表整体替换当前列表，返回本地还没有规则文件的规则集 ID。
func (m *Manager) Replace(sets []RuleSet) ([]string, error) {
	var missing []string
	err := m.mutate(true, func() error {
		next := make([]RuleSet, 0, len(sets))
		seen := map[string]bool{}
		for _, set := range sets {
			name := filepath.Base(set.Path)
			if set.ID == "" || seen[set.ID] || strings.TrimSpace(set.URL) == "" || name == "." || name == string(filepath.Separator) {
				return fmt.Errorf("备份中的规则集无效: %q", set.ID)
			}
			seen[set.ID] = true
			set.Path = filepath.Join(m.rulesDir, name)
			if set.Source == "" {
				set.Source = "custom"
			}
			if set.Action == "" {
				set.Action = "block"
			}
			if _, err := os.Stat(set.Path); err != nil {
				missing = append(missing, set.ID)
			}
			next = append(next, set)
		}
		m.sets = next
		return nil
	})
	if err != nil {
		return nil, err
	}
	return missing, nil
}

// List 返回所有规则集状态；清单读不出时返回错误，不把默认列表当成用户的列表展示。
func (m *Manager) List() ([]RuleInfo, error) {
	sets, err := m.Sets()
	if err != nil {
		return nil, err
	}
	infos := make([]RuleInfo, 0, len(sets))
	for _, s := range sets {
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
	return infos, nil
}

// Add 用已经 Fetch 好的内容添加规则集，返回新规则集的 ID。action 是命中后的去向，空表示屏蔽（广告规则最常见）。
func (m *Manager) Add(name, rawURL, action string, data []byte, validate RuleSetValidator) (string, error) {
	name = strings.TrimSpace(name)
	rawURL = strings.TrimSpace(rawURL)
	action, err := normalizeAction(action)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(rawURL))
	id := "custom-" + hex.EncodeToString(hash[:])[:12]
	installed := ""
	err = m.mutate(false, func() error {
		if err := validateRuleSetInput(m.sets, name, rawURL, ""); err != nil {
			return err
		}
		path, err := install(filepath.Join(m.rulesDir, id+".srs"), data, validate)
		if err != nil {
			return fmt.Errorf("添加规则集失败: %w", err)
		}
		installed = path
		m.sets = append(m.sets, RuleSet{ID: id, Name: name, URL: rawURL, Path: path, Enabled: true, Source: "custom", Action: action})
		return nil
	})
	if err != nil {
		if installed != "" {
			_ = os.Remove(installed)
		}
		return "", err
	}
	return id, nil
}

// HasURL 说明这个链接是否已经添加过。
func (m *Manager) HasURL(rawURL string) bool {
	sets, _ := m.Sets()
	for _, set := range sets {
		if set.URL == strings.TrimSpace(rawURL) {
			return true
		}
	}
	return false
}

// ValidateInput 在下载之前检查名称和链接，免得白下载。
func (m *Manager) ValidateInput(name, rawURL, existingID string) error {
	sets, err := m.Sets()
	if err != nil {
		return err
	}
	return validateRuleSetInput(sets, strings.TrimSpace(name), strings.TrimSpace(rawURL), existingID)
}

func normalizeAction(action string) (string, error) {
	action = strings.TrimSpace(action)
	if action == "" {
		return "block", nil
	}
	if config.IsBasicAction(action) {
		kind, _ := config.SplitAction(action)
		return kind, nil
	}
	return action, config.ValidateAction(action)
}

// NeedsDownload 说明编辑后是否要重新下载：链接变了，或者本地规则文件不在。
func (set RuleSet) NeedsDownload(rawURL string) bool {
	_, err := os.Stat(set.Path)
	return strings.TrimSpace(rawURL) != set.URL || err != nil
}

// Edit 修改名称、链接和去向；需要重新下载时 data 是调用方 Fetch 好的内容。内置规则集的去向不能改。
func (m *Manager) Edit(id, name, rawURL, action string, data []byte, validate RuleSetValidator) error {
	name = strings.TrimSpace(name)
	rawURL = strings.TrimSpace(rawURL)
	var oldPath, newPath string
	err := m.mutate(false, func() error {
		i, err := m.index(id)
		if err != nil {
			return err
		}
		if err := validateRuleSetInput(m.sets, name, rawURL, id); err != nil {
			return err
		}
		set := m.sets[i]
		if set.Source != "default" {
			if set.Action, err = normalizeAction(action); err != nil {
				return err
			}
		}
		set.Name = name
		if set.NeedsDownload(rawURL) {
			if data == nil {
				return errors.New("规则集已被修改，请重试")
			}
			path, err := install(set.Path, data, validate)
			if err != nil {
				return fmt.Errorf("更新规则集失败: %w", err)
			}
			oldPath, newPath = set.Path, path
			set.URL, set.Path = rawURL, path
		}
		m.sets[i] = set
		return nil
	})
	cleanupReplaced(oldPath, newPath, err)
	return err
}

// cleanupReplaced 在规则集格式变化（.srs ↔ 文本转换的 .json）换了文件名时收尾：
// 保存成功删旧文件，保存失败删新文件，磁盘上的清单始终指向存在的文件。
func cleanupReplaced(oldPath, newPath string, err error) {
	if oldPath == newPath {
		return
	}
	if err != nil {
		_ = os.Remove(newPath)
	} else {
		_ = os.Remove(oldPath)
	}
}

// SetAction 只改用户规则集命中后的去向。
func (m *Manager) SetAction(id, action string) error {
	action, err := normalizeAction(action)
	if err != nil {
		return err
	}
	return m.mutate(false, func() error {
		i, err := m.index(id)
		if err != nil {
			return err
		}
		if m.sets[i].Source == "default" {
			return fmt.Errorf("内置规则集的去向不能修改")
		}
		m.sets[i].Action = action
		return nil
	})
}

func (m *Manager) SetEnabled(id string, enabled bool) error {
	return m.mutate(false, func() error {
		i, err := m.index(id)
		if err != nil {
			return err
		}
		m.sets[i].Enabled = enabled
		return nil
	})
}

// Delete 先保存清单，成功后再删规则文件。
func (m *Manager) Delete(id string) error {
	path := ""
	err := m.mutate(false, func() error {
		i, err := m.index(id)
		if err != nil {
			return err
		}
		path = m.sets[i].Path
		m.sets = append(m.sets[:i], m.sets[i+1:]...)
		return nil
	})
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Defaults 返回内置规则集的默认设置。
func (m *Manager) Defaults() []RuleSet { return defaultRuleSets(m.rulesDir) }

// RestoreDefaults 把内置规则集恢复成默认设置（保留用户自己的规则集）；清单损坏时也可以用它覆盖。
func (m *Manager) RestoreDefaults() error {
	return m.mutate(true, func() error {
		next := m.Defaults()
		ids := make(map[string]bool, len(next))
		for _, set := range next {
			ids[set.ID] = true
		}
		for _, set := range m.sets {
			if !ids[set.ID] {
				next = append(next, set)
			}
		}
		m.sets = next
		return nil
	})
}

// Install 用 Fetch 好的内容更新规则集文件，校验通过才替换。
func (m *Manager) Install(id string, data []byte, validate RuleSetValidator) error {
	var oldPath, newPath string
	err := m.mutate(false, func() error {
		i, err := m.index(id)
		if err != nil {
			return err
		}
		path, err := install(m.sets[i].Path, data, validate)
		if err != nil {
			return err
		}
		oldPath, newPath = m.sets[i].Path, path
		m.sets[i].Path = path
		return nil
	})
	cleanupReplaced(oldPath, newPath, err)
	return err
}

// FetchTimeout 是单个规则集的下载超时。
const FetchTimeout = 30 * time.Second

// StatusError 是规则集服务器返回的非 2xx 状态。
type StatusError int

func (e StatusError) Error() string { return fmt.Sprintf("HTTP %d", int(e)) }

// Fetch 下载规则集内容并统一格式：.srs 原样返回；文本列表（Clash/Surge 规则列表、域名列表等）
// 转换成 sing-box 源格式 JSON。client 为 nil 时直连。不碰管理器状态，可以在锁外并发调用。
func Fetch(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: FetchTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "sbtun")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, StatusError(resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRuleSetBytes+1))
	if err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
	}
	if len(data) > maxRuleSetBytes {
		return nil, fmt.Errorf("规则集超过 %d MB 上限", maxRuleSetBytes>>20)
	}
	if isBinaryRuleSet(data) {
		return data, nil
	}
	data, _, err = ConvertText(data)
	return data, err
}

// install 把 Fetch 的结果写到同目录临时文件，校验通过后替换；扩展名按内容定为 .srs 或 .json。
func install(path string, data []byte, validate RuleSetValidator) (string, error) {
	ext := ".json"
	if isBinaryRuleSet(data) {
		ext = ".srs"
	}
	path = strings.TrimSuffix(path, filepath.Ext(path)) + ext
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".rule-*"+ext)
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if validate != nil {
		if err := validate(tmpName); err != nil {
			return "", err
		}
	}
	if err := config.ShareWithInvoker(tmpName, 0o644); err != nil {
		return "", err
	}
	return path, os.Rename(tmpName, path)
}

const maxRuleSetBytes = 32 << 20

func validateRuleSetInput(sets []RuleSet, name, rawURL, existingID string) error {
	if name == "" || rawURL == "" {
		return fmt.Errorf("规则集名称和 URL 不能为空")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return fmt.Errorf("规则集 URL 必须使用 http:// 或 https://")
	}
	for _, set := range sets {
		if set.ID != existingID && set.URL == rawURL {
			return fmt.Errorf("规则集已经存在")
		}
	}
	return nil
}
