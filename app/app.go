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
	failoverNext    time.Time
	failoverBackoff time.Duration
	uploadRate      uint64
	downloadRate    uint64
}

const clashAPIBaseURL = "http://127.0.0.1:9090"

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
		a.workDir = filepath.Join(base, "runtime-data")
		a.binary = filepath.Join(base, singBoxBinaryName())
	} else {
		a.workDir = filepath.Join(".", "runtime-data")
		a.binary = singBoxBinaryName()
	}
	a.manager = config.NewManager(filepath.Join(a.workDir, "config.json"))
}

type StatusDTO struct {
	State         string `json:"state"`
	Message       string `json:"message"`
	Running       bool   `json:"running"`
	UploadBytes   uint64 `json:"upload_bytes"`
	DownloadBytes uint64 `json:"download_bytes"`
	Selector      string `json:"selector,omitempty"`
	CurrentNodeID string `json:"current_node_id,omitempty"`
}

type DiagnosticsDTO struct {
	Enabled       bool   `json:"enabled"`
	Selector      string `json:"selector"`
	CurrentNodeID string `json:"current_node_id"`
	Message       string `json:"message"`
}

func (a *App) GetDiagnostics() DiagnosticsDTO {
	cfg := a.GetConfig()
	result := DiagnosticsDTO{Enabled: cfg.DiagnosticsEnabled, CurrentNodeID: cfg.CurrentNodeID}
	if !cfg.DiagnosticsEnabled {
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

func (a *App) GetStatus() StatusDTO {
	state, message := a.runtime.State.Get()
	a.trafficMu.RLock()
	up, down := a.uploadRate, a.downloadRate
	a.trafficMu.RUnlock()
	result := StatusDTO{State: string(state), Message: message, Running: state == core.StateRunning, UploadBytes: up, DownloadBytes: down}
	if result.Running {
		if cfg, err := a.manager.Load(); err == nil {
			if selector, selectorErr := currentSelector(); selectorErr == nil {
				result.Selector = selector
				if id := nodeIDFromSelector(cfg, selector); id != "" {
					result.CurrentNodeID = id
					if id != cfg.CurrentNodeID {
						a.operationMu.Lock()
						latest, loadErr := a.manager.Load()
						if loadErr == nil && nodeIDFromSelector(latest, selector) == id && latest.CurrentNodeID != id {
							latest.CurrentNodeID = id
							_ = a.manager.Save(latest)
						}
						a.operationMu.Unlock()
					}
				}
			}
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
	if a.runtime == nil || !a.runtime.SingBox.Running() {
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
	if err := a.manager.Save(cfg); err != nil {
		return err
	}
	return a.reloadIfRunningLocked()
}

func (a *App) Start() error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.startLocked()
}

func (a *App) startLocked() error {
	cfg, err := a.manager.Load()
	if err != nil {
		return err
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
	return a.runtime.Start(a.ctx, cfg)
}

func (a *App) TestNode(id string) NodeHealthDTO {
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

func (a *App) stopLocked() error { return a.runtime.Stop() }

func (a *App) AddNode(node config.Node) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	cfg, err := a.manager.Load()
	if err != nil {
		return err
	}
	for i := range cfg.Nodes {
		if cfg.Nodes[i].ID == node.ID {
			cfg.Nodes[i] = node
			if err := a.manager.Save(cfg); err != nil {
				return err
			}
			if a.runtime != nil {
				if _, err := a.runtime.SyncConfig(cfg); err != nil {
					return fmt.Errorf("节点已写入配置，但生成运行配置失败: %w", err)
				}
			}
			return a.reloadIfRunningLocked()
		}
	}
	cfg.Nodes = append(cfg.Nodes, node)
	if cfg.CurrentNodeID == "" {
		cfg.CurrentNodeID = node.ID
	}
	if err := a.manager.Save(cfg); err != nil {
		return err
	}
	if a.runtime != nil {
		if _, err := a.runtime.SyncConfig(cfg); err != nil {
			return fmt.Errorf("节点已写入配置，但生成运行配置失败: %w", err)
		}
	}
	return a.reloadIfRunningLocked()
}

func (a *App) RemoveNode(id string) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	cfg, err := a.manager.Load()
	if err != nil {
		return err
	}
	found := false
	filtered := make([]config.Node, 0, len(cfg.Nodes))
	for _, n := range cfg.Nodes {
		if n.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, n)
	}
	if !found {
		return fmt.Errorf("节点不存在: %s", id)
	}
	cfg.Nodes = filtered
	if cfg.CurrentNodeID == id {
		cfg.CurrentNodeID = ""
	}
	if err := a.manager.Save(cfg); err != nil {
		return err
	}
	return a.reloadIfRunningLocked()
}

// RemoveNodes removes several nodes atomically by ID.
func (a *App) RemoveNodes(ids []string) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	cfg, err := a.manager.Load()
	if err != nil {
		return err
	}
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			wanted[id] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return errors.New("未选择节点")
	}
	filtered := make([]config.Node, 0, len(cfg.Nodes))
	removed := 0
	for _, node := range cfg.Nodes {
		if _, ok := wanted[node.ID]; ok {
			removed++
			continue
		}
		filtered = append(filtered, node)
	}
	if removed == 0 {
		return errors.New("未找到要删除的节点")
	}
	cfg.Nodes = filtered
	if _, ok := wanted[cfg.CurrentNodeID]; ok {
		cfg.CurrentNodeID = ""
		if len(cfg.Nodes) > 0 {
			cfg.CurrentNodeID = cfg.Nodes[0].ID
		}
	}
	if err := a.manager.Save(cfg); err != nil {
		return err
	}
	if len(cfg.Nodes) == 0 {
		if a.runtime != nil && a.runtime.State.IsRunning() {
			return a.stopLocked()
		}
		return nil
	}
	if a.runtime != nil {
		if _, err := a.runtime.SyncConfig(cfg); err != nil {
			return fmt.Errorf("节点已删除，但生成运行配置失败: %w", err)
		}
	}
	return a.reloadIfRunningLocked()
}

// UpdateNode replaces editable fields while preserving the node ID.
func (a *App) UpdateNode(id string, node config.Node) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if id == "" || node.Server == "" || node.Port == 0 || node.Protocol == "" {
		return errors.New("节点信息不完整")
	}
	cfg, err := a.manager.Load()
	if err != nil {
		return err
	}
	for i := range cfg.Nodes {
		if cfg.Nodes[i].ID != id {
			continue
		}
		node.ID = id
		cfg.Nodes[i] = node
		if err := a.manager.Save(cfg); err != nil {
			return err
		}
		if a.runtime != nil {
			if _, err := a.runtime.SyncConfig(cfg); err != nil {
				return fmt.Errorf("节点已保存，但生成运行配置失败: %w", err)
			}
		}
		return a.reloadIfRunningLocked()
	}
	return fmt.Errorf("节点不存在: %s", id)
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
	if a.runtime == nil || !a.runtime.State.IsRunning() {
		return nil
	}
	if err := a.stopLocked(); err != nil {
		return err
	}
	return a.startLocked()
}

func (a *App) SelectNode(id string) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.selectNodeLocked(id)
}

func (a *App) selectNodeLocked(id string) error {
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
	running := a.runtime != nil && a.runtime.State.IsRunning()
	// CLI invocations create a fresh App, so its in-memory runtime state is
	// stopped even when another sbtun process is serving Clash API requests.
	apiRunning := false
	if !running {
		_, apiErr := currentSelector()
		apiRunning = apiErr == nil
	}
	selectorActive := running || apiRunning
	if selectorActive && previousID != id {
		if err := switchSelector(id); err != nil {
			if !running {
				return err
			}
			if reloadErr := a.reloadIfRunningLocked(); reloadErr != nil {
				return fmt.Errorf("节点切换失败（无缝切换和自动重启均失败）: %w", reloadErr)
			}
		}
	}
	cfg.CurrentNodeID = id
	if err := a.manager.Save(cfg); err != nil {
		if selectorActive && previousID != "" && previousID != id {
			_ = switchSelector(previousID)
		}
		return err
	}
	return nil
}

func switchSelector(id string) error {
	body, err := json.Marshal(map[string]string{"name": "node-" + id})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, clashAPIBaseURL+"/proxies/proxy", bytes.NewReader(body))
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
		current, err := currentSelector()
		if err == nil && current == "node-"+id {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("无缝切换节点未确认生效: node-%s", id)
}

func currentSelector() (string, error) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(clashAPIBaseURL + "/proxies/proxy")
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
	if a.runtime == nil || !a.runtime.State.IsRunning() {
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
	if candidate, ok := firstHealthyReplacement(cfg.Nodes, current.ID, a.testNodeHealthy); ok {
		a.operationMu.Lock()
		defer a.operationMu.Unlock()
		latest, err := a.manager.Load()
		if err != nil || latest.CurrentNodeID != current.ID || !a.runtime.State.IsRunning() {
			return
		}
		if candidate, ok = firstHealthyReplacement(latest.Nodes, latest.CurrentNodeID, a.testNodeHealthy); ok {
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
	for _, candidate := range nodes {
		if candidate.ID == currentID || !check(candidate) {
			continue
		}
		return candidate, true
	}
	return config.Node{}, false
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
	cfg, err := a.manager.Load()
	if err != nil {
		return err
	}
	cfg.RoutingMode = mode
	if err := a.manager.Save(cfg); err != nil {
		return err
	}
	return a.reloadIfRunningLocked()
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
	cfg, err := a.manager.Load()
	if err != nil {
		return 0, err
	}
	existing := make(map[string]struct{}, len(cfg.Nodes))
	for _, n := range cfg.Nodes {
		existing[n.ID] = struct{}{}
	}
	added := 0
	for _, n := range nodes {
		if _, ok := existing[n.ID]; ok {
			continue
		}
		cfg.Nodes = append(cfg.Nodes, n)
		existing[n.ID] = struct{}{}
		added++
	}
	if cfg.CurrentNodeID == "" && len(cfg.Nodes) > 0 {
		cfg.CurrentNodeID = cfg.Nodes[0].ID
	}
	if err := a.manager.Save(cfg); err != nil {
		return 0, err
	}
	if a.runtime != nil {
		if _, err := a.runtime.SyncConfig(cfg); err != nil {
			return added, fmt.Errorf("节点已写入配置，但生成运行配置失败: %w", err)
		}
	}
	if err := a.reloadIfRunningLocked(); err != nil {
		return added, err
	}
	return added, nil
}

func (a *App) checkReady(ctx context.Context) error {
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
	return a.rulesManager.List()
}

func (a *App) UpdateRule(id string) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if err := a.rulesManager.Update(id); err != nil {
		return err
	}
	return a.reloadIfRunningLocked()
}

func (a *App) UpdateAllRules() []error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	errs := a.rulesManager.UpdateAll()
	if err := a.reloadIfRunningLocked(); err != nil {
		errs = append(errs, err)
	}
	return errs
}

func (a *App) Ping() string { return "sbtun 运行正常" }
