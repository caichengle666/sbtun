package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core"
	"github.com/caichengle666/sbtun/core/capture"
	"github.com/caichengle666/sbtun/core/rules"
)

type App struct {
	ctx             context.Context
	workDir         string
	binary          string
	manager         *config.Manager
	runtime         *RuntimeCoordinator
	rulesManager    *rules.Manager
	trayIcon        []byte
	trayEnd         func()
	shutdownOnce    sync.Once
	operationMu     sync.Mutex
	trafficMu       sync.RWMutex
	failoverMu      sync.Mutex
	selectorSyncMu  sync.Mutex
	selectorSyncEnd context.CancelFunc
	selectorSyncSeq uint64
	selectorState   string
	selectorMessage string
	failoverNext    time.Time
	failoverBackoff time.Duration
	failoverBlocked map[string]time.Time
	uploadRate      uint64
	downloadRate    uint64
}

var clashAPIBaseURL = "http://127.0.0.1:9090"

const failoverCooldown = 30 * time.Second

func New() *App { return &App{} }

func (a *App) SetTrayIcon(icon []byte) {
	a.trayIcon = append([]byte(nil), icon...)
}

func (a *App) Startup(ctx context.Context) {
	a.startupCore(ctx)
	go a.StartTray()
	go a.monitorNodes()
}

// StartupCLI initializes the runtime without starting the desktop tray.
func (a *App) StartupCLI(ctx context.Context) {
	a.startupCore(ctx)
}

// StartMonitoring enables health checks for the long-running CLI mode.
func (a *App) StartMonitoring() {
	if a.ctx == nil || a.runtime == nil {
		return
	}
	go a.monitorNodes()
}

func (a *App) startupCore(ctx context.Context) {
	a.ctx = ctx
	a.initPaths()
	a.runtime = NewRuntimeCoordinator(a.workDir, a.binary)
	a.runtime.ReadyCheck = a.checkReady
	exeDir := filepath.Dir(a.binary)
	if exeDir == "." || exeDir == "" {
		exeDir, _ = os.Getwd()
	}
	a.rulesManager = rules.NewManager(filepath.Join(exeDir, "rules"))
}

// Shutdown stops the proxy before Wails tears down the application process.
func (a *App) Shutdown(ctx context.Context) {
	a.shutdownOnce.Do(func() {
		a.stopSelectorSync()
		if a.runtime != nil {
			_ = a.runtime.Stop()
		}
		if a.trayEnd != nil {
			a.trayEnd()
		}
	})
}

func (a *App) initPaths() {
	execPath, err := os.Executable()
	if execPath != "" && err == nil {
		base := filepath.Dir(execPath)
		a.workDir = runtimeDataDir(base)
		a.binary = filepath.Join(base, singBoxBinaryName())
	} else {
		a.workDir = filepath.Join(".", "runtime-data")
		a.binary = singBoxBinaryName()
	}
	a.manager = config.NewManager(filepath.Join(a.workDir, "config.json"))
}

type StatusDTO struct {
	Version             string `json:"version"`
	State               string `json:"state"`
	Message             string `json:"message"`
	Running             bool   `json:"running"`
	UploadBytes         uint64 `json:"upload_bytes"`
	DownloadBytes       uint64 `json:"download_bytes"`
	Selector            string `json:"selector,omitempty"`
	CurrentNodeID       string `json:"current_node_id,omitempty"`
	SelectorSyncState   string `json:"selector_sync_state,omitempty"`
	SelectorSyncMessage string `json:"selector_sync_message,omitempty"`
}

type DiagnosticsDTO struct {
	Enabled       bool   `json:"enabled"`
	Selector      string `json:"selector"`
	CurrentNodeID string `json:"current_node_id"`
	Message       string `json:"message"`
}

func (a *App) GetDiagnostics() DiagnosticsDTO {
	if a.manager == nil {
		return DiagnosticsDTO{Message: "配置管理器未初始化"}
	}
	cfg, err := a.manager.Load()
	if err != nil {
		return DiagnosticsDTO{Message: err.Error()}
	}
	result := DiagnosticsDTO{Enabled: cfg.DiagnosticsEnabled, CurrentNodeID: cfg.CurrentNodeID}
	if !cfg.DiagnosticsEnabled {
		return result
	}
	if a.runtime == nil || a.runtime.State == nil {
		result.Message = "运行时未初始化"
		return result
	}
	if !a.runtime.State.IsRunning() {
		result.Message = "TUN 未运行"
		return result
	}
	selector, err := currentSelector()
	if err != nil {
		result.Message = err.Error()
		return result
	}
	result.Selector = selector
	result.Message = "selector 已连接"
	return result
}

func (a *App) GetCaptureStatus() capture.Status {
	if a.runtime == nil || a.runtime.Capture == nil {
		return capture.Status{Message: "流量分析器未初始化"}
	}
	cfg := a.GetConfig()
	status := a.runtime.Capture.Status(cfg.CaptureEnabled)
	if status.Message != "" {
		return status
	}
	if !cfg.CaptureEnabled {
		status.Message = "分析功能未启用"
	} else if status.Running {
		status.Message = "分析器运行中"
	} else if a.runtime.State == nil || !a.runtime.State.IsRunning() {
		status.Message = "等待 TUN 启动"
	} else {
		status.Message = "分析器启动异常"
	}
	return status
}

func (a *App) GetCaptureFlows() ([]capture.Flow, error) {
	if a.runtime == nil || a.runtime.Capture == nil {
		return []capture.Flow{}, nil
	}
	flows, err := a.runtime.Capture.Flows()
	if err != nil {
		return nil, err
	}
	annotateCaptureRoutes(flows, a.GetConfig())
	return flows, nil
}

func (a *App) GetCaptureFlow(id uint64) (capture.Flow, error) {
	if a.runtime == nil || a.runtime.Capture == nil {
		return capture.Flow{}, errors.New("流量分析器未初始化")
	}
	flow, err := a.runtime.Capture.Flow(id)
	if err != nil {
		return capture.Flow{}, err
	}
	flow.Route = captureRoute(flow, a.GetConfig())
	return flow, nil
}

func (a *App) ClearCaptureFlows() error {
	if a.runtime != nil && a.runtime.Capture != nil {
		return a.runtime.Capture.Clear()
	}
	return nil
}

func (a *App) SaveCaptureFlows() error {
	if a.runtime == nil || a.runtime.Capture == nil {
		return errors.New("流量分析器未初始化")
	}
	return a.runtime.Capture.Save()
}

func (a *App) InstallCaptureCertificate() error {
	if a.runtime == nil || a.runtime.Capture == nil {
		return errors.New("流量分析器未初始化")
	}
	return a.runtime.Capture.InstallCertificate()
}

func (a *App) UninstallCaptureCertificate() error {
	if a.runtime == nil || a.runtime.Capture == nil {
		return errors.New("流量分析器未初始化")
	}
	return a.runtime.Capture.UninstallCertificate()
}

func (a *App) GetStatus() StatusDTO {
	if a.runtime == nil || a.runtime.State == nil {
		return StatusDTO{Version: Version(), State: string(core.StateStopped), Message: "运行时未初始化"}
	}
	state, message := a.runtime.State.Get()
	a.trafficMu.RLock()
	up, down := a.uploadRate, a.downloadRate
	a.trafficMu.RUnlock()
	result := StatusDTO{Version: Version(), State: string(state), Message: message, Running: state == core.StateRunning, UploadBytes: up, DownloadBytes: down}
	result.SelectorSyncState, result.SelectorSyncMessage = a.selectorSyncStatus()
	if a.manager == nil {
		if result.Message == "" {
			result.Message = "配置管理器未初始化"
		}
		return result
	}
	cfg, err := a.manager.Load()
	if err != nil {
		if result.Message == "" {
			result.Message = err.Error()
		}
		return result
	}
	result.CurrentNodeID = cfg.CurrentNodeID
	if selector, selectorErr := currentSelector(); selectorErr == nil {
		if !result.Running {
			// A CLI status invocation has no in-memory runtime state, but it
			// must still report an already-running sing-box process.
			result.State = string(core.StateRunning)
			result.Message = "sing-box 运行中"
			result.Running = true
		}
		result.Selector = selector
		if id := nodeIDFromSelector(cfg, selector); id != "" {
			result.CurrentNodeID = id
		}
	}
	return result
}

func nodeIDFromSelector(cfg config.Config, selector string) string {
	const prefix = "node-"
	if len(selector) <= len(prefix) || selector[:len(prefix)] != prefix {
		return ""
	}
	id := selector[len(prefix):]
	for _, node := range cfg.Nodes {
		if node.ID == id {
			return id
		}
	}
	return ""
}

func (a *App) refreshTraffic() (uint64, uint64) {
	if a.runtime == nil || a.runtime.SingBox == nil || !a.runtime.SingBox.Running() {
		a.trafficMu.Lock()
		a.uploadRate, a.downloadRate = 0, 0
		a.trafficMu.Unlock()
		return 0, 0
	}
	up, down := trafficStats()
	a.trafficMu.Lock()
	a.uploadRate, a.downloadRate = up, down
	a.trafficMu.Unlock()
	return up, down
}

func trafficStats() (uint64, uint64) {
	client := http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Get(clashAPIBaseURL + "/traffic?interval=1000")
	if err != nil {
		return 0, 0
	}
	defer resp.Body.Close()
	var v struct {
		Up   uint64 `json:"up"`
		Down uint64 `json:"down"`
	}
	if resp.StatusCode != http.StatusOK {
		return 0, 0
	}
	decoder := json.NewDecoder(resp.Body)
	// /traffic is a streaming endpoint; the first frame is often a zero-value snapshot.
	if err := decoder.Decode(&v); err != nil {
		return 0, 0
	}
	var latest struct {
		Up   uint64 `json:"up"`
		Down uint64 `json:"down"`
	}
	if err := decoder.Decode(&latest); err == nil {
		v = latest
	}
	return v.Up, v.Down
}

func (a *App) GetConfig() config.Config {
	cfg, _ := a.LoadConfig()
	return cfg
}

// LoadConfig exposes configuration errors to CLI callers instead of silently
// turning permission or format failures into an empty default configuration.
func (a *App) LoadConfig() (config.Config, error) {
	if a.manager == nil {
		return config.Config{}, errors.New("配置管理器未初始化")
	}
	return a.manager.Load()
}

func (a *App) SaveConfig(cfg config.Config) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		*next = cfg
		return nil
	}, true)
}

func (a *App) SetCaptureSettings(enabled bool, domains []string) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		next.CaptureEnabled = enabled
		next.CaptureDomains = append([]string(nil), domains...)
		return nil
	}, true)
}

func (a *App) Start() error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.startLocked()
}

func (a *App) startLocked() error {
	if a.manager == nil {
		return errors.New("配置管理器未初始化")
	}
	if a.runtime == nil {
		return errors.New("运行时未初始化")
	}
	cfg, err := a.manager.Load()
	if err != nil {
		return err
	}
	return a.startWithConfigLocked(cfg)
}

func (a *App) startWithConfigLocked(cfg config.Config) error {
	if a.runtime == nil {
		return errors.New("运行时未初始化")
	}
	if a.binary == "" || !fileExists(a.binary) {
		return fmt.Errorf("未找到 %s，请把它放在 sbtun 同目录下", singBoxBinaryName())
	}
	node, ok := currentNode(cfg)
	if !ok {
		return errors.New("尚未选择节点")
	}
	health := testNodeWithBinary(a.ctx, node, a.binary)
	if !health.Healthy {
		return fmt.Errorf("节点不可用: %s", health.Message)
	}
	if err := a.runtime.Start(a.ctx, cfg); err != nil {
		return err
	}
	a.startSelectorSync(cfg.CurrentNodeID)
	return nil
}

func (a *App) mutateConfigAndSyncLocked(mutate func(before config.Config, next *config.Config) error, reload bool) error {
	if a.manager == nil {
		return errors.New("配置管理器未初始化")
	}
	if a.runtime == nil || a.runtime.State == nil {
		return errors.New("运行时未初始化")
	}
	wasRunning := a.runtime.State.IsRunning()
	before, after, err := a.manager.UpdateResult(mutate)
	if err != nil {
		return err
	}
	if err := a.applyRuntimeConfigLocked(after, wasRunning, reload); err != nil {
		if rollbackErr := a.rollbackConfigAndRuntimeLocked(before, wasRunning); rollbackErr != nil {
			return fmt.Errorf("%w; 回滚失败: %v", err, rollbackErr)
		}
		return err
	}
	return nil
}

func (a *App) applyRuntimeConfigLocked(next config.Config, wasRunning, reload bool) error {
	if len(next.Nodes) == 0 {
		if wasRunning {
			if err := a.runtime.Stop(); err != nil {
				return fmt.Errorf("停止旧运行配置失败: %w", err)
			}
		}
		return nil
	}
	if reload && wasRunning {
		if err := a.runtime.Reload(a.ctx, next); err != nil {
			return fmt.Errorf("重载运行配置失败: %w", err)
		}
		a.startSelectorSync(next.CurrentNodeID)
		return nil
	}
	if _, err := a.runtime.SyncConfig(next); err != nil {
		return fmt.Errorf("生成运行配置失败: %w", err)
	}
	return nil
}

func (a *App) rollbackConfigAndRuntimeLocked(before config.Config, wasRunning bool) error {
	var rollbackErrs []error
	if err := a.manager.Save(before); err != nil {
		rollbackErrs = append(rollbackErrs, fmt.Errorf("恢复配置失败: %w", err))
	}
	if len(before.Nodes) > 0 {
		if _, err := a.runtime.SyncConfig(before); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("恢复运行配置失败: %w", err))
		}
	}
	if wasRunning {
		if a.runtime.State.IsRunning() {
			if err := a.runtime.Reload(a.ctx, before); err != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("恢复运行配置失败: %w", err))
			} else {
				a.startSelectorSync(before.CurrentNodeID)
			}
		} else if err := a.startWithConfigLocked(before); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("恢复运行节点失败: %w", err))
		}
	}
	return errors.Join(rollbackErrs...)
}

func (a *App) TestNode(id string) NodeHealthDTO {
	if a.manager == nil {
		return NodeHealthDTO{NodeID: id, Message: "配置管理器未初始化"}
	}
	cfg, err := a.manager.Load()
	if err != nil {
		return NodeHealthDTO{NodeID: id, Message: err.Error()}
	}
	for _, node := range cfg.Nodes {
		if node.ID == id {
			return testNodeWithBinary(a.ctx, node, a.binary)
		}
	}
	return NodeHealthDTO{NodeID: id, Message: "节点不存在"}
}

func currentNode(cfg config.Config) (config.Node, bool) {
	for _, node := range cfg.Nodes {
		if node.ID == cfg.CurrentNodeID {
			return node, true
		}
	}
	return config.Node{}, false
}

func (a *App) Stop() error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.stopLocked()
}

func (a *App) stopLocked() error {
	if a.runtime == nil {
		return errors.New("运行时未初始化")
	}
	a.stopSelectorSync()
	return a.runtime.Stop()
}

func (a *App) AddNode(node config.Node) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		for i := range next.Nodes {
			if next.Nodes[i].ID == node.ID {
				next.Nodes[i] = node
				return nil
			}
		}
		next.Nodes = append(next.Nodes, node)
		if next.CurrentNodeID == "" {
			next.CurrentNodeID = node.ID
		}
		return nil
	}, true)
}

func (a *App) RemoveNode(id string) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		found := false
		filtered := make([]config.Node, 0, len(next.Nodes))
		for _, n := range next.Nodes {
			if n.ID == id {
				found = true
				continue
			}
			filtered = append(filtered, n)
		}
		if !found {
			return fmt.Errorf("节点不存在: %s", id)
		}
		next.Nodes = filtered
		if next.CurrentNodeID == id {
			next.CurrentNodeID = ""
			if len(next.Nodes) > 0 {
				next.CurrentNodeID = next.Nodes[0].ID
			}
		}
		return nil
	}, true)
}

// RemoveNodes removes several nodes atomically by ID.
func (a *App) RemoveNodes(ids []string) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			wanted[id] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return errors.New("未选择节点")
	}
	return a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		filtered := make([]config.Node, 0, len(next.Nodes))
		removed := 0
		for _, node := range next.Nodes {
			if _, ok := wanted[node.ID]; ok {
				removed++
				continue
			}
			filtered = append(filtered, node)
		}
		if removed == 0 {
			return errors.New("未找到要删除的节点")
		}
		next.Nodes = filtered
		if _, ok := wanted[next.CurrentNodeID]; ok {
			next.CurrentNodeID = ""
			if len(next.Nodes) > 0 {
				next.CurrentNodeID = next.Nodes[0].ID
			}
		}
		return nil
	}, true)
}

// UpdateNode replaces editable fields while preserving the node ID.
func (a *App) UpdateNode(id string, node config.Node) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if id == "" || node.Server == "" || node.Port == 0 || node.Protocol == "" {
		return errors.New("节点信息不完整")
	}
	return a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		for i := range next.Nodes {
			if next.Nodes[i].ID != id {
				continue
			}
			node.ID = id
			next.Nodes[i] = node
			return nil
		}
		return fmt.Errorf("节点不存在: %s", id)
	}, true)
}

// UpdateNodeFromLink replaces all node parameters parsed from a share link.
func (a *App) UpdateNodeFromLink(id, link string) error {
	nodes, err := parseSubscription(link)
	if err != nil {
		return err
	}
	if len(nodes) != 1 {
		return errors.New("编辑节点时必须提供单个节点链接")
	}
	nodes[0].ID = id
	return a.UpdateNode(id, nodes[0])
}

// TestNodes checks nodes sequentially to keep resource usage bounded.
func (a *App) TestNodes(ids []string) []NodeHealthDTO {
	results := make([]NodeHealthDTO, 0, len(ids))
	for _, id := range ids {
		results = append(results, a.TestNode(id))
	}
	return results
}

func (a *App) reloadIfRunningLocked() error {
	if a.runtime == nil || a.runtime.State == nil || !a.runtime.State.IsRunning() {
		return nil
	}
	if a.manager == nil {
		return errors.New("配置管理器未初始化")
	}
	cfg, err := a.manager.Load()
	if err != nil {
		return err
	}
	if err := a.runtime.Reload(a.ctx, cfg); err != nil {
		return err
	}
	a.startSelectorSync(cfg.CurrentNodeID)
	return nil
}

func (a *App) SelectNode(id string) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.selectNodeLocked(id)
}

func (a *App) selectNodeLocked(id string) error {
	if a.manager == nil {
		return errors.New("配置管理器未初始化")
	}
	if a.runtime == nil || a.runtime.State == nil {
		return errors.New("运行时未初始化")
	}
	cfg, err := a.manager.Load()
	if err != nil {
		return err
	}
	found := false
	for _, n := range cfg.Nodes {
		if n.ID == id {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("节点不存在: %s", id)
	}
	previousID := cfg.CurrentNodeID
	running := a.runtime.State.IsRunning()
	// CLI invocations create a fresh App, so its in-memory runtime state is
	// stopped even when another sbtun process is serving Clash API requests.
	apiRunning := false
	if !running {
		_, apiErr := currentSelector()
		apiRunning = apiErr == nil
	}
	selectorActive := running || apiRunning
	switchedSelector := false
	if selectorActive && previousID != id {
		if running {
			a.stopSelectorSync()
		}
		if err := switchSelector(id); err != nil {
			if running {
				a.startSelectorSync(previousID)
			}
			return err
		}
		switchedSelector = true
	}
	before, after, err := a.manager.UpdateResult(func(before config.Config, next *config.Config) error {
		if before.CurrentNodeID != previousID {
			return errors.New("节点配置已变化，请重试")
		}
		next.CurrentNodeID = id
		return nil
	})
	if err != nil {
		if switchedSelector && previousID != "" {
			if switchErr := switchSelector(previousID); switchErr != nil {
				if running {
					a.startSelectorSync(previousID)
				}
			} else if running {
				a.setSelectorSyncStatus("synced", "")
			}
		}
		return err
	}
	if _, err := a.runtime.SyncConfig(after); err != nil {
		rollbackErrs := []error{fmt.Errorf("生成运行配置失败: %w", err)}
		if saveErr := a.manager.Save(before); saveErr != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("恢复配置失败: %w", saveErr))
		}
		if switchedSelector && previousID != "" {
			if switchErr := switchSelector(previousID); switchErr != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("恢复节点选择失败: %w", switchErr))
				if running {
					a.startSelectorSync(previousID)
				}
			} else if running {
				a.setSelectorSyncStatus("synced", "")
			}
		}
		return errors.Join(rollbackErrs...)
	}
	if running {
		a.setSelectorSyncStatus("synced", "")
	}
	return nil
}

func switchSelector(id string) error {
	return switchSelectorContext(context.Background(), id)
}

func switchSelectorContext(ctx context.Context, id string) error {
	body, err := json.Marshal(map[string]string{"name": "node-" + id})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, clashAPIBaseURL+"/proxies/proxy", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("无缝切换节点失败: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("无缝切换节点失败: HTTP %d", resp.StatusCode)
	}
	for attempt := 0; attempt < 8; attempt++ {
		current, err := currentSelectorContext(ctx)
		if err == nil && current == "node-"+id {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	return fmt.Errorf("无缝切换节点未确认生效: node-%s", id)
}

func (a *App) startSelectorSync(id string) {
	if id == "" {
		a.stopSelectorSync()
		return
	}
	baseCtx := a.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(baseCtx)
	a.selectorSyncMu.Lock()
	if a.selectorSyncEnd != nil {
		a.selectorSyncEnd()
	}
	a.selectorSyncSeq++
	seq := a.selectorSyncSeq
	a.selectorSyncEnd = cancel
	a.selectorState = "syncing"
	a.selectorMessage = ""
	a.selectorSyncMu.Unlock()
	go func() {
		defer cancel()
		a.runSelectorSync(ctx, seq, id)
	}()
}

func (a *App) runSelectorSync(ctx context.Context, seq uint64, id string) {
	delays := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond, time.Second}
	for attempt := 0; ; attempt++ {
		if ctx.Err() != nil {
			return
		}
		if a.runtime == nil || a.runtime.State == nil || !a.runtime.State.IsRunning() {
			a.updateSelectorSyncStatus(seq, "idle", "")
			return
		}
		if err := switchSelectorContext(ctx, id); err == nil {
			a.updateSelectorSyncStatus(seq, "synced", "")
			return
		} else {
			state := "syncing"
			if attempt >= 3 {
				state = "failed"
			}
			if !a.updateSelectorSyncStatus(seq, state, err.Error()) {
				return
			}
		}
		delay := delays[len(delays)-1]
		if attempt < len(delays) {
			delay = delays[attempt]
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (a *App) updateSelectorSyncStatus(seq uint64, state, message string) bool {
	a.selectorSyncMu.Lock()
	defer a.selectorSyncMu.Unlock()
	if seq != a.selectorSyncSeq {
		return false
	}
	a.selectorState = state
	a.selectorMessage = message
	return true
}

func (a *App) setSelectorSyncStatus(state, message string) {
	a.selectorSyncMu.Lock()
	defer a.selectorSyncMu.Unlock()
	if a.selectorSyncEnd != nil {
		a.selectorSyncEnd()
	}
	a.selectorSyncSeq++
	a.selectorSyncEnd = nil
	a.selectorState = state
	a.selectorMessage = message
}

func (a *App) stopSelectorSync() {
	a.setSelectorSyncStatus("idle", "")
}

func (a *App) selectorSyncStatus() (string, string) {
	a.selectorSyncMu.Lock()
	defer a.selectorSyncMu.Unlock()
	return a.selectorState, a.selectorMessage
}

func currentSelector() (string, error) {
	return currentSelectorContext(context.Background())
}

func currentSelectorContext(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clashAPIBaseURL+"/proxies/proxy", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("读取当前节点失败: HTTP %d", resp.StatusCode)
	}
	var result struct {
		Now string `json:"now"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Now == "" {
		return "", errors.New("sing-box 未返回当前节点")
	}
	return result.Now, nil
}

func (a *App) monitorNodes() {
	if a.ctx == nil {
		return
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.autoSwitchNode()
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *App) autoSwitchNode() {
	if a.runtime == nil || a.runtime.State == nil || a.manager == nil || !a.runtime.State.IsRunning() {
		return
	}
	a.failoverMu.Lock()
	if time.Now().Before(a.failoverNext) {
		a.failoverMu.Unlock()
		return
	}
	a.failoverMu.Unlock()
	cfg, err := a.manager.Load()
	if err != nil {
		return
	}
	current, ok := currentNode(cfg)
	if !ok || a.testNodeHealthy(current) {
		return
	}
	a.blockFailoverNode(current.ID)
	if candidate, ok := firstHealthyReplacementWithCooldown(cfg.Nodes, current.ID, a.isFailoverNodeBlocked, a.testNodeHealthy); ok {
		a.operationMu.Lock()
		defer a.operationMu.Unlock()
		latest, err := a.manager.Load()
		if err != nil || latest.CurrentNodeID != current.ID || !a.runtime.State.IsRunning() {
			return
		}
		if candidate, ok = firstHealthyReplacementWithCooldown(latest.Nodes, latest.CurrentNodeID, a.isFailoverNodeBlocked, a.testNodeHealthy); ok {
			if err := a.selectNodeLocked(candidate.ID); err != nil {
				fmt.Printf("自动切换节点失败（%s）: %v\n", candidate.ID, err)
				a.failoverMu.Lock()
				if a.failoverBackoff == 0 {
					a.failoverBackoff = 10 * time.Second
				} else if a.failoverBackoff < 2*time.Minute {
					a.failoverBackoff *= 2
				}
				a.failoverNext = time.Now().Add(a.failoverBackoff)
				a.failoverMu.Unlock()
			} else {
				a.failoverMu.Lock()
				a.failoverBackoff = 0
				a.failoverNext = time.Time{}
				a.failoverMu.Unlock()
			}
		}
		return
	}
}

func firstHealthyReplacement(nodes []config.Node, currentID string, check func(config.Node) bool) (config.Node, bool) {
	return firstHealthyReplacementWithCooldown(nodes, currentID, nil, check)
}

func firstHealthyReplacementWithCooldown(nodes []config.Node, currentID string, blocked func(string) bool, check func(config.Node) bool) (config.Node, bool) {
	for _, candidate := range nodes {
		if candidate.ID == currentID || (blocked != nil && blocked(candidate.ID)) || !check(candidate) {
			continue
		}
		return candidate, true
	}
	return config.Node{}, false
}

func (a *App) blockFailoverNode(id string) {
	if id == "" {
		return
	}
	a.failoverMu.Lock()
	defer a.failoverMu.Unlock()
	if a.failoverBlocked == nil {
		a.failoverBlocked = make(map[string]time.Time)
	}
	a.failoverBlocked[id] = time.Now().Add(failoverCooldown)
}

func (a *App) isFailoverNodeBlocked(id string) bool {
	a.failoverMu.Lock()
	defer a.failoverMu.Unlock()
	if a.failoverBlocked == nil {
		return false
	}
	expires, ok := a.failoverBlocked[id]
	if !ok {
		return false
	}
	if !time.Now().Before(expires) {
		delete(a.failoverBlocked, id)
		return false
	}
	return true
}

func testNodePort(node config.Node) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return testNode(ctx, node).Port.OK
}

func (a *App) testNodeHealthy(node config.Node) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	return testNodeWithBinary(ctx, node, a.binary).Healthy
}

func (a *App) SetRoutingMode(mode config.RoutingMode) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		next.RoutingMode = mode
		return nil
	}, true)
}

func (a *App) ImportSubscription(link string) (int, error) {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	nodes, err := parseSubscription(link)
	if err != nil {
		return 0, err
	}
	if len(nodes) == 0 {
		return 0, errors.New("订阅中未识别到任何节点")
	}
	added := 0
	err = a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		existing := make(map[string]struct{}, len(next.Nodes))
		for _, n := range next.Nodes {
			existing[n.ID] = struct{}{}
		}
		added = 0
		for _, n := range nodes {
			if _, ok := existing[n.ID]; ok {
				continue
			}
			next.Nodes = append(next.Nodes, n)
			existing[n.ID] = struct{}{}
			added++
		}
		if next.CurrentNodeID == "" && len(next.Nodes) > 0 {
			next.CurrentNodeID = next.Nodes[0].ID
		}
		return nil
	}, true)
	if err != nil {
		return 0, err
	}
	return added, nil
}

func (a *App) checkReady(ctx context.Context) error {
	if a.runtime == nil || a.runtime.SingBox == nil || a.runtime.TUN == nil {
		return errors.New("运行时未初始化")
	}
	if !a.runtime.SingBox.Running() {
		return errors.New("sing-box 进程未存活，请检查节点配置")
	}
	if err := a.runtime.TUN.WaitReady(8 * time.Second); err != nil {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (a *App) ListRules() []rules.RuleInfo {
	if a.rulesManager == nil {
		return nil
	}
	return a.rulesManager.List()
}

func (a *App) UpdateRule(id string) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if a.rulesManager == nil {
		return errors.New("规则管理器未初始化")
	}
	if err := a.rulesManager.Update(id); err != nil {
		return err
	}
	return a.reloadIfRunningLocked()
}

func (a *App) UpdateAllRules() []error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if a.rulesManager == nil {
		return []error{errors.New("规则管理器未初始化")}
	}
	errs := a.rulesManager.UpdateAll()
	if err := a.reloadIfRunningLocked(); err != nil {
		errs = append(errs, err)
	}
	return errs
}

func (a *App) Ping() string { return "sbtun 运行正常" }
