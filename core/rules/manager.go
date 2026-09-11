package rules

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "time"
)

// RuleSet 描述一个分流规则集。
type RuleSet struct {
    ID      string `json:"id"`
    Name    string `json:"name"`
    URL     string `json:"url"`
    Path    string `json:"path"`
    Enabled bool   `json:"enabled"`
}

// RuleInfo 是前端展示的规则集状态。
type RuleInfo struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    URL       string `json:"url"`
    Enabled   bool   `json:"enabled"`
    Path      string `json:"path"`
    Exists    bool   `json:"exists"`
    Size      int64  `json:"size"`
    SHA256    string `json:"sha256"`
    UpdatedAt string `json:"updated_at"`
}

// Manager 管理规则集。
type Manager struct {
    rulesDir string
    sets     []RuleSet
}

// NewManager 创建规则集管理器。
// rulesDir 是程序 exe 所在目录下的 rules/（sing-box 运行时查找的路径）。
func NewManager(rulesDir string) *Manager {
    m := &Manager{rulesDir: rulesDir}
    m.sets = []RuleSet{
        {
            ID:      "geosite-cn",
            Name:    "国内域名 (geosite)",
            URL:     "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-geolocation-cn.srs",
            Path:    filepath.Join(m.rulesDir, "geosite-geolocation-cn.srs"),
            Enabled: true,
        },
        {
            ID:      "geoip-cn",
            Name:    "国内 IP (geoip)",
            URL:     "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-cn.srs",
            Path:    filepath.Join(m.rulesDir, "geoip-cn.srs"),
            Enabled: true,
        },
    }
    return m
}

// List 返回所有规则集状态。
func (m *Manager) List() []RuleInfo {
    var infos []RuleInfo
    for _, s := range m.sets {
        info := RuleInfo{
            ID:      s.ID,
            Name:    s.Name,
            URL:     s.URL,
            Enabled: s.Enabled,
            Path:    s.Path,
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
    f, err := os.Create(target.Path)
    if err != nil {
        return err
    }
    defer f.Close()
    if _, err := io.Copy(f, resp.Body); err != nil {
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
