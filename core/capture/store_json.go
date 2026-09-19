package capture

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/caichengle666/sbtun/config"
)

type captureFile struct {
	Version int    `json:"version"`
	Flows   []Flow `json:"flows"`
}

func (m *Manager) loadLocked() error {
	if m.loaded {
		return nil
	}
	data, err := os.ReadFile(m.storagePath())
	if errors.Is(err, os.ErrNotExist) {
		m.loaded = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取抓包文件失败: %w", err)
	}
	var stored captureFile
	if err := json.Unmarshal(data, &stored); err != nil {
		backup := m.storagePath() + ".corrupt"
		if renameErr := os.Rename(m.storagePath(), backup); renameErr != nil {
			return fmt.Errorf("解析抓包文件失败: %w（备份失败: %v）", err, renameErr)
		}
		return fmt.Errorf("解析抓包文件失败，已备份到 %s: %w", backup, err)
	}
	if len(stored.Flows) > maxStoredFlows {
		stored.Flows = stored.Flows[len(stored.Flows)-maxStoredFlows:]
	}
	m.loaded = true
	m.flows = stored.Flows
	for _, flow := range m.flows {
		if flow.ID > m.nextID.Load() {
			m.nextID.Store(flow.ID)
		}
	}
	return nil
}

func (m *Manager) saveLocked() error {
	if err := os.MkdirAll(m.workDir, 0o700); err != nil {
		return fmt.Errorf("创建抓包目录失败: %w", err)
	}
	data, err := json.MarshalIndent(captureFile{Version: 1, Flows: m.flows}, "", "  ")
	if err != nil {
		return fmt.Errorf("编码抓包文件失败: %w", err)
	}
	temporary, err := os.CreateTemp(m.workDir, ".capture-*.tmp")
	if err != nil {
		return fmt.Errorf("创建抓包临时文件失败: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("设置抓包文件权限失败: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入抓包文件失败: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("同步抓包文件失败: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭抓包临时文件失败: %w", err)
	}
	if err := config.ReplaceConfigFile(temporaryPath, m.storagePath()); err != nil {
		return fmt.Errorf("替换抓包文件失败: %w", err)
	}
	return nil
}

func (m *Manager) appendFlow(flow Flow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return config.WithFileLock(m.storagePath(), func() error {
		m.loaded = false
		if err := m.loadLocked(); err != nil {
			return err
		}
		m.flows = append(m.flows, flow)
		if len(m.flows) > maxStoredFlows {
			m.flows = append([]Flow(nil), m.flows[len(m.flows)-maxStoredFlows:]...)
		}
		if err := m.saveLocked(); err != nil {
			return err
		}
		m.dirty = false
		return nil
	})
}

func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := config.WithFileLock(m.storagePath(), func() error {
		if err := m.loadLocked(); err != nil {
			return err
		}
		return m.saveLocked()
	}); err != nil {
		return err
	}
	m.dirty = false
	return nil
}

func (m *Manager) Flows() ([]Flow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.loadLocked(); err != nil {
		return nil, err
	}
	count := len(m.flows)
	if count > maxVisibleFlows {
		count = maxVisibleFlows
	}
	result := make([]Flow, count)
	for i := 0; i < count; i++ {
		flow := m.flows[len(m.flows)-1-i]
		flow.RequestBody = ""
		flow.ResponseBody = ""
		result[i] = flow
	}
	return result, nil
}

func (m *Manager) Flow(id uint64) (Flow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.loadLocked(); err != nil {
		return Flow{}, err
	}
	for i := len(m.flows) - 1; i >= 0; i-- {
		if m.flows[i].ID == id {
			return m.flows[i], nil
		}
	}
	return Flow{}, errors.New("抓包记录不存在")
}

func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return config.WithFileLock(m.storagePath(), func() error {
		m.loaded = true
		m.flows = nil
		m.nextID.Store(0)
		m.dirty = false
		if err := os.Remove(m.storagePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	})
}

func (m *Manager) flowCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = m.loadLocked()
	return len(m.flows)
}

func (m *Manager) isDirty() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dirty
}

func bodyForDisplay(body []byte, contentType string) (string, string) {
	if len(body) == 0 {
		return "", "text"
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	textual := strings.HasPrefix(mediaType, "text/") || strings.Contains(mediaType, "json") ||
		strings.Contains(mediaType, "xml") || strings.Contains(mediaType, "javascript") ||
		strings.Contains(mediaType, "x-www-form-urlencoded")
	if textual && utf8.Valid(body) {
		return string(body), "text"
	}
	return base64.StdEncoding.EncodeToString(body), "base64"
}
