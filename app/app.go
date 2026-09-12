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
	ctx          context.Context
	workDir      string
	binary       string
	manager      *config.Manager
	runtime      *RuntimeCoordinator
	rulesManager *rules.Manager
	trayEnd      func()
	shutdownOnce sync.Once
	operationMu  sync.Mutex
	trafficMu    sync.RWMutex
	uploadRate   uint64
	downloadRate uint64
}

func New() *App { return &App{} }

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.initPaths()
	a.runtime = NewRuntimeCoordinator(a.workDir, a.binary)
	a.runtime.ReadyCheck = a.checkReady
	exeDir := filepath.Dir(a.binary)
	if exeDir == "." || exeDir == "" {
		exeDir, _ = os.Getwd()
	}
	a.rulesManager = rules.NewManager(filepath.Join(exeDir, "rules"))
	go a.StartTray()
	go a.monitorNodes()
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
		a.binary = filepath.Join(base, "sing-box.exe")
	} else {
		a.workDir = filepath.Join(".", "runtime-data")
		a.binary = "sing-box.exe"
	}
	a.manager = config.NewManager(filepath.Join(a.workDir, "config.json"))
}

type StatusDTO struct {
	State         string `json:"state"`
	Message       string `json:"message"`
	Running       bool   `json:"running"`
	UploadBytes   uint64 `json:"upload_bytes"`
	DownloadBytes uint64 `json:"download_bytes"`
}

func (a *App) GetStatus() StatusDTO {
	state, message := a.runtime.State.Get()
	a.trafficMu.RLock()
	up, down := a.uploadRate, a.downloadRate
	a.trafficMu.RUnlock()
	return StatusDTO{State: string(state), Message: message, Running: state == core.StateRunning, UploadBytes: up, DownloadBytes: down}
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
	resp, err := client.Get("http://127.0.0.1:9090/traffic?interval=1000")
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
	cfg, err := a.manager.Load()
	if err != nil {
		return config.Default()
	}
	return cfg
}

func (a *App) SaveConfig(cfg config.Config) error { return a.manager.Save(cfg) }

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
		return errors.New("未找到 sing-box.exe，请把它放在 sbtun.exe 同目录下")
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
	if a.runtime.State.IsRunning() {
		if err := switchSelector(id); err != nil {
			return err
		}
	}
	cfg.CurrentNodeID = id
	if err := a.manager.Save(cfg); err != nil {
		if a.runtime.State.IsRunning() && previousID != "" {
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
	req, err := http.NewRequest(http.MethodPut, "http://127.0.0.1:9090/proxies/proxy", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("无缝切换节点失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("无缝切换节点失败: HTTP %d", resp.StatusCode)
	}
	return nil
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
	cfg, err := a.manager.Load()
	if err != nil {
		return
	}
	current, ok := currentNode(cfg)
	if !ok || testNodePort(current) {
		return
	}
	if candidate, ok := firstHealthyReplacement(cfg.Nodes, current.ID, testNodePort); ok {
		a.operationMu.Lock()
		if a.runtime.State.IsRunning() {
			_ = a.selectNodeLocked(candidate.ID)
		}
		a.operationMu.Unlock()
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

func (a *App) SetRoutingMode(mode config.RoutingMode) error {
	cfg, err := a.manager.Load()
	if err != nil {
		return err
	}
	cfg.RoutingMode = mode
	return a.manager.Save(cfg)
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
	return a.rulesManager.Update(id)
}

func (a *App) UpdateAllRules() []error {
	return a.rulesManager.UpdateAll()
}

func (a *App) Ping() string { return "sbtun 运行正常" }
