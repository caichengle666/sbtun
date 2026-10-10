package capture

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/caichengle666/sbtun/config"
)

// flushDelay batches captured flows before they are written to disk. Writing
// the whole JSON file on every request made capture cost grow with the size of
// the history and blocked the proxied response while the file was rewritten.
const flushDelay = time.Second

type captureFile struct {
	Version int    `json:"version"`
	Flows   []Flow `json:"flows"`
}

// fileStamp identifies the on-disk capture file version this process last
// read or wrote. A different stamp means another process changed the file,
// for example `sbtun capture clear` while the GUI is capturing.
type fileStamp struct {
	exists  bool
	modTime time.Time
	size    int64
}

func (m *Manager) statStorage() fileStamp {
	info, err := os.Stat(m.storagePath())
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{exists: true, modTime: info.ModTime(), size: info.Size()}
}

func (s fileStamp) equal(other fileStamp) bool {
	return s.exists == other.exists && s.size == other.size && s.modTime.Equal(other.modTime)
}

// readStoredFlows reads the capture file. A missing file is an empty history;
// a corrupt file is moved aside so capture can continue.
func (m *Manager) readStoredFlows() ([]Flow, error) {
	data, err := os.ReadFile(m.storagePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取抓包文件失败: %w", err)
	}
	var stored captureFile
	if err := json.Unmarshal(data, &stored); err != nil {
		backup := m.storagePath() + ".corrupt"
		if renameErr := os.Rename(m.storagePath(), backup); renameErr != nil {
			return nil, fmt.Errorf("解析抓包文件失败: %w（备份失败: %v）", err, renameErr)
		}
		return nil, fmt.Errorf("解析抓包文件失败，已备份到 %s: %w", backup, err)
	}
	if len(stored.Flows) > maxStoredFlows {
		stored.Flows = stored.Flows[len(stored.Flows)-maxStoredFlows:]
	}
	return stored.Flows, nil
}

func (m *Manager) loadLocked() error {
	if m.loaded {
		return nil
	}
	stamp := m.statStorage()
	flows, err := m.readStoredFlows()
	if err != nil {
		return err
	}
	m.loaded = true
	m.flows = flows
	m.stamp = stamp
	m.flushedMaxID = 0
	for _, flow := range m.flows {
		if flow.ID > m.nextID.Load() {
			m.nextID.Store(flow.ID)
		}
		if flow.ID > m.flushedMaxID {
			m.flushedMaxID = flow.ID
		}
	}
	return nil
}

// refreshLocked reloads the history when another process changed the file
// and this process has nothing unsaved that a reload would lose.
func (m *Manager) refreshLocked() error {
	if !m.loaded {
		return m.loadLocked()
	}
	if m.dirty || m.statStorage().equal(m.stamp) {
		return nil
	}
	m.loaded = false
	return m.loadLocked()
}

// mergeExternalLocked keeps another process's change to the file (such as a
// clear) and re-applies only the flows this process has not written yet.
func (m *Manager) mergeExternalLocked() error {
	if m.statStorage().equal(m.stamp) {
		return nil
	}
	stored, err := m.readStoredFlows()
	if err != nil {
		return err
	}
	merged := stored
	for _, flow := range m.flows {
		if flow.ID > m.flushedMaxID {
			merged = append(merged, flow)
		}
	}
	if len(merged) > maxStoredFlows {
		merged = append([]Flow(nil), merged[len(merged)-maxStoredFlows:]...)
	}
	m.flows = merged
	for _, flow := range m.flows {
		if flow.ID > m.nextID.Load() {
			m.nextID.Store(flow.ID)
		}
	}
	return nil
}

func (m *Manager) writeStoredFlows(flows []Flow) error {
	if err := os.MkdirAll(m.workDir, 0o700); err != nil {
		return fmt.Errorf("创建抓包目录失败: %w", err)
	}
	data, err := json.Marshal(captureFile{Version: 1, Flows: flows})
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

// appendFlow records a flow in memory only. Disk writes are batched by the
// flush loop, so the proxied connection never waits on file I/O.
func (m *Manager) appendFlow(flow Flow) error {
	m.mu.Lock()
	if err := m.loadLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	m.flows = append(m.flows, flow)
	if len(m.flows) > maxStoredFlows {
		m.flows = append([]Flow(nil), m.flows[len(m.flows)-maxStoredFlows:]...)
	}
	m.dirty = true
	signal := m.flushSignal
	m.mu.Unlock()
	if signal != nil {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
	return nil
}

// flush writes the in-memory history when it has unsaved flows, or always
// when force is set. Lock order is always file lock, then m.mu.
func (m *Manager) flush(force bool) error {
	return config.WithFileLock(m.storagePath(), func() error {
		m.mu.Lock()
		if err := m.loadLocked(); err != nil {
			m.mu.Unlock()
			return err
		}
		if !m.dirty && !force {
			m.mu.Unlock()
			return nil
		}
		if err := m.mergeExternalLocked(); err != nil {
			m.mu.Unlock()
			return err
		}
		snapshot := append([]Flow(nil), m.flows...)
		var maxID uint64
		for _, flow := range snapshot {
			if flow.ID > maxID {
				maxID = flow.ID
			}
		}
		m.dirty = false
		m.mu.Unlock()

		// Encoding and writing happen outside m.mu so new flows can still be
		// recorded while a large history is being saved.
		err := m.writeStoredFlows(snapshot)
		stamp := m.statStorage()

		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.dirty = true
			return err
		}
		m.stamp = stamp
		if maxID > m.flushedMaxID {
			m.flushedMaxID = maxID
		}
		return nil
	})
}

func (m *Manager) flushLoop(signal <-chan struct{}, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-stop:
			return
		case <-signal:
		}
		timer := time.NewTimer(flushDelay)
		select {
		case <-stop:
			timer.Stop()
			return
		case <-timer.C:
		}
		if err := m.flush(false); err != nil {
			m.mu.Lock()
			m.lastError = err.Error()
			m.mu.Unlock()
		}
	}
}

func (m *Manager) Save() error {
	return m.flush(true)
}

func (m *Manager) Flows() ([]Flow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.refreshLocked(); err != nil {
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
	if err := m.refreshLocked(); err != nil {
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
	return config.WithFileLock(m.storagePath(), func() error {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.loaded = true
		m.flows = nil
		m.nextID.Store(0)
		m.flushedMaxID = 0
		m.dirty = false
		if err := os.Remove(m.storagePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		m.stamp = fileStamp{}
		return nil
	})
}

func (m *Manager) flowCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = m.refreshLocked()
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
