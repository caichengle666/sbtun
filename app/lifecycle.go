package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core"
	"github.com/caichengle666/sbtun/core/capture"
	"github.com/caichengle666/sbtun/core/singbox"
	"github.com/caichengle666/sbtun/core/tun"
)

const (
	ErrConfigInvalid      = "CONFIG_INVALID"
	ErrSingBoxStartFailed = "SINGBOX_START_FAILED"
	ErrSingBoxReload      = "SINGBOX_RELOAD_FAILED"
	ErrTunNotReady        = "TUN_NOT_READY"
	ErrSingBoxExited      = "SINGBOX_EXITED"
	ErrCaptureStartFailed = "CAPTURE_START_FAILED"
)

type RuntimeCoordinator struct {
	mu      sync.Mutex
	State   *core.StateStore
	SingBox *singbox.Manager
	TUN     *tun.Manager
	Capture *capture.Manager
	WorkDir string
	Binary  string
	// ResourceDir 放规则文件的目录，默认是 sing-box 所在目录（安卓上内核目录只读，单独指定）。
	ResourceDir     string
	ReadyCheck      func(context.Context) error
	activeConfig    config.Config
	activeConfigSet bool
	activeData      []byte // 正在运行的 sing-box 配置内容，用来判断重载是否真有必要
	// activeResolved 是生成 activeData 时节点域名的解析结果。判断配置是否变化时沿用它，
	// DNS 轮换或缓存过期换了 IP 不会被当成配置变化而重启、断开所有连接。
	activeResolved []netip.Addr
	// LocalAuth protects the clash API and capture proxy ports.
	LocalAuth singbox.LocalAuth
	resolver  nodeResolver
}

func NewRuntimeCoordinator(workDir, binary string) *RuntimeCoordinator {
	r := &RuntimeCoordinator{
		State: core.NewStateStore(), SingBox: singbox.NewManager(), TUN: tun.NewManager(),
		Capture: capture.NewManager(filepath.Join(workDir, "capture"), "127.0.0.1:9081", "127.0.0.1:9082"),
		WorkDir: workDir, Binary: binary,
	}
	if auth, err := loadOrCreateLocalAuth(workDir); err == nil {
		r.LocalAuth = auth
	} else if auth, genErr := newLocalAuth(); genErr == nil {
		// Still protect the ports for this process even if the shared file
		// cannot be written; the CLI then cannot reach this instance's API.
		fmt.Fprintf(os.Stderr, "本地认证信息无法保存，仅本进程有效: %v\n", err)
		r.LocalAuth = auth
	}
	setClashAPISecret(r.LocalAuth.ClashSecret)
	r.Capture.SetAuth(&capture.ProxyAuth{Username: r.LocalAuth.ProxyUsername, Password: r.LocalAuth.ProxyPassword})
	go r.watchSingBoxExit()
	return r
}

func (r *RuntimeCoordinator) watchSingBoxExit() {
	if r.SingBox == nil || r.State == nil {
		return
	}
	for event := range r.SingBox.Exited() {
		if event.Expected {
			continue
		}
		r.mu.Lock()
		// A crash can be queued while Start/Reload holds the lock and has
		// already launched a replacement process. That stale event must not
		// mark the new process as failed or tear down its routes.
		if current := r.SingBox.Generation(); current != 0 && current != event.Generation {
			r.mu.Unlock()
			continue
		}
		state, _ := r.State.Get()
		if state == core.StateStopping || state == core.StateStopped || state == core.StateError {
			r.mu.Unlock()
			continue
		}
		if r.TUN != nil {
			r.TUN.MarkStopped()
		}
		message := ErrSingBoxExited
		if event.Err != nil {
			message += ": " + event.Err.Error()
		}
		// “exit status 1”本身看不出原因，带上 sing-box 日志里最后一条错误。
		if reason := r.SingBox.LastError(); reason != "" {
			message += "：" + reason
		}
		r.State.Set(core.StateError, message)
		if r.Capture != nil {
			_ = r.Capture.Stop()
		}
		_ = r.cleanupTUN()
		r.mu.Unlock()
	}
}

func (r *RuntimeCoordinator) Start(ctx context.Context, cfg config.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.State == nil || r.SingBox == nil || r.TUN == nil {
		return errors.New("runtime 未完整初始化")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state, _ := r.State.Get()
	if state == core.StateStarting || state == core.StateRunning || state == core.StateStopping {
		return fmt.Errorf("runtime already active: %s", state)
	}
	_ = r.TUN.RecoverStale()
	return r.startConfigLocked(ctx, cfg)
}

// Reload restarts sing-box without disabling the existing TUN adapter. This
// keeps rule and routing changes fast while still replacing the running config.
func (r *RuntimeCoordinator) Reload(ctx context.Context, cfg config.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.State == nil || r.SingBox == nil || r.TUN == nil {
		return errors.New("runtime 未完整初始化")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state, _ := r.State.Get()
	if state == core.StateStopped {
		// 已停止就只写配置，绝不自行启动（退出过程中被重载会留下孤儿进程）。
		_, err := r.SyncConfig(cfg)
		return err
	}
	if state != core.StateRunning && state != core.StateError {
		return fmt.Errorf("runtime cannot reload from state: %s", state)
	}
	// 先校验通过再替换 runtime.json；校验失败时旧配置原样保留，旧进程继续运行。
	built, err := r.writeValidatedConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrConfigInvalid, err)
	}
	captureRestart := !r.activeConfigSet || captureConfigChanged(r.activeConfig, cfg)
	r.State.Set(core.StateStopping, "")
	if err := r.SingBox.Stop(); err != nil {
		return r.fail(ErrSingBoxReload, fmt.Errorf("停止旧 sing-box 失败: %w", err))
	}
	if captureRestart && r.Capture != nil {
		if err := r.Capture.Stop(); err != nil {
			return r.fail(ErrSingBoxReload, fmt.Errorf("停止流量分析器失败: %w", err))
		}
	}
	if err := r.TUN.CleanupRoutes(); err != nil {
		return r.fail(ErrSingBoxReload, fmt.Errorf("清理旧 TUN 路由失败: %w", err))
	}
	return r.launchLocked(ctx, cfg, built)
}

// SyncIfUnchanged 在运行中且新配置生成的 sing-box 配置与正在运行的完全一致时，只写回 runtime.json、不重启，
// 返回 true。订阅定时更新等只改了元数据（如 UpdatedAt）的场景靠它避免断开所有连接。
func (r *RuntimeCoordinator) SyncIfUnchanged(cfg config.Config) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.State == nil || !r.State.IsRunning() || !r.activeConfigSet || captureConfigChanged(r.activeConfig, cfg) ||
		!slices.Equal(singbox.NodeDomains(cfg.Nodes), singbox.NodeDomains(r.activeConfig.Nodes)) {
		return false, nil
	}
	data, err := r.buildConfig(cfg, r.activeResolved)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(data, r.activeData) && !r.onlySelectionChanged(cfg) {
		return false, nil
	}
	if err := atomicWrite(r.configPath(), data, nil); err != nil {
		return false, fmt.Errorf("写入运行配置失败: %w", err)
	}
	r.activeConfig = cfg
	r.activeData = data
	return true, nil
}

// onlySelectionChanged 判断新配置和运行中的配置是否只差当前节点：切节点只改了 selector 的默认值，
// 运行中靠 Clash API 无缝切换，不需要重启 sing-box。
func (r *RuntimeCoordinator) onlySelectionChanged(cfg config.Config) bool {
	if cfg.CurrentNodeID == r.activeConfig.CurrentNodeID {
		return false
	}
	cfg.CurrentNodeID = r.activeConfig.CurrentNodeID
	data, err := r.buildConfig(cfg, r.activeResolved)
	return err == nil && bytes.Equal(data, r.activeData)
}

func (r *RuntimeCoordinator) startConfigLocked(ctx context.Context, cfg config.Config) error {
	r.State.Set(core.StateStarting, "")
	built, err := r.writeValidatedConfig(ctx, cfg)
	if err != nil {
		return r.fail(ErrConfigInvalid, err)
	}
	return r.launchLocked(ctx, cfg, built)
}

// launchLocked 用已校验并写好的配置启动 sing-box，并按需启停流量分析器。
func (r *RuntimeCoordinator) launchLocked(ctx context.Context, cfg config.Config, built builtConfig) error {
	r.State.Set(core.StateStarting, "")
	if r.Capture != nil {
		if cfg.CaptureEnabled && len(cfg.CaptureDomains) > 0 {
			if err := r.Capture.Start(ctx, cfg.CaptureDomains); err != nil {
				return r.fail(ErrCaptureStartFailed, err)
			}
		} else if err := r.Capture.Stop(); err != nil {
			return r.fail(ErrCaptureStartFailed, err)
		}
	}
	if err := r.SingBox.Start(ctx, r.Binary, r.configPath()); err != nil {
		return r.fail(ErrSingBoxStartFailed, err)
	}
	if r.ReadyCheck != nil {
		readyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := r.ReadyCheck(readyCtx)
		cancel()
		if err != nil {
			stopErr := r.SingBox.Stop()
			cleanupErr := r.cleanupTUN()
			if r.Capture != nil {
				_ = r.Capture.Stop()
			}
			if stopErr != nil {
				err = fmt.Errorf("%w; 停止失败: %v", err, stopErr)
			}
			if cleanupErr != nil {
				err = fmt.Errorf("%w; 清理失败: %v", err, cleanupErr)
			}
			return r.fail(ErrTunNotReady, err)
		}
	}
	r.TUN.MarkRunning()
	r.State.Set(core.StateRunning, "")
	r.activeConfig = cfg
	r.activeConfigSet = true
	r.activeData = built.data
	r.activeResolved = built.resolved
	return nil
}

func captureConfigChanged(before, after config.Config) bool {
	if before.CaptureEnabled != after.CaptureEnabled {
		return true
	}
	left := append([]string(nil), before.CaptureDomains...)
	right := append([]string(nil), after.CaptureDomains...)
	normalize := func(values []string) []string {
		seen := make(map[string]struct{}, len(values))
		result := make([]string, 0, len(values))
		for _, value := range values {
			value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), ".")
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
		sort.Strings(result)
		return result
	}
	left = normalize(left)
	right = normalize(right)
	if len(left) != len(right) {
		return true
	}
	for i := range left {
		if left[i] != right[i] {
			return true
		}
	}
	return false
}

// builtConfig 是一份生成好的 sing-box 配置和生成时用到的节点解析结果。
type builtConfig struct {
	data     []byte
	resolved []netip.Addr
}

func (r *RuntimeCoordinator) buildConfig(cfg config.Config, resolved []netip.Addr) ([]byte, error) {
	return singbox.BuildConfigWithAuth(cfg, r.resourceDir(), r.LocalAuth, resolved)
}

func (r *RuntimeCoordinator) configPath() string { return filepath.Join(r.WorkDir, "runtime.json") }

// SyncConfig 生成并原子写入 runtime.json，不做 sing-box 校验（未运行时保存配置用）。
func (r *RuntimeCoordinator) SyncConfig(cfg config.Config) (string, error) {
	data, err := r.buildConfig(cfg, r.resolver.resolve(cfg.Nodes))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(r.WorkDir, 0o755); err != nil {
		return "", fmt.Errorf("创建运行目录失败: %w", err)
	}
	if err := atomicWrite(r.configPath(), data, nil); err != nil {
		return "", fmt.Errorf("写入运行配置失败: %w", err)
	}
	return r.configPath(), nil
}

// writeValidatedConfig 把配置写到临时文件，sing-box check 通过后才替换 runtime.json；
// 任何一步失败都保留旧的 runtime.json，避免之后每次启动和内核升级校验都被坏配置卡住。
func (r *RuntimeCoordinator) writeValidatedConfig(ctx context.Context, cfg config.Config) (builtConfig, error) {
	if err := config.Validate(cfg); err != nil {
		return builtConfig{}, err
	}
	built := builtConfig{resolved: r.resolver.resolve(cfg.Nodes)}
	var err error
	if built.data, err = r.buildConfig(cfg, built.resolved); err != nil {
		return builtConfig{}, err
	}
	if err := os.MkdirAll(r.WorkDir, 0o755); err != nil {
		return builtConfig{}, fmt.Errorf("创建运行目录失败: %w", err)
	}
	err = atomicWrite(r.configPath(), built.data, func(tmp string) error {
		return singbox.ValidateConfig(ctx, r.Binary, tmp)
	})
	if err != nil {
		return builtConfig{}, err
	}
	return built, nil
}

func (r *RuntimeCoordinator) resourceDir() string {
	if r.ResourceDir != "" {
		return r.ResourceDir
	}
	dir := filepath.Dir(r.Binary)
	if dir == "." || dir == "" {
		dir, _ = os.Getwd()
	}
	return dir
}

func (r *RuntimeCoordinator) cleanupTUN() error {
	if r.TUN == nil {
		return nil
	}
	return r.TUN.Cleanup()
}

func (r *RuntimeCoordinator) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.State == nil {
		return errors.New("runtime 状态未初始化")
	}
	state, _ := r.State.Get()
	if state == core.StateStopped {
		if r.TUN == nil {
			return nil
		}
		r.TUN.MarkStopped()
		return r.cleanupTUN()
	}
	r.State.Set(core.StateStopping, "")
	var stopErr error
	if r.SingBox != nil {
		stopErr = r.SingBox.Stop()
	}
	if r.Capture != nil {
		if err := r.Capture.Stop(); stopErr == nil {
			stopErr = err
		}
	}
	cleanupErr := r.cleanupTUN()
	if r.TUN != nil {
		r.TUN.MarkStopped()
	}
	if stopErr != nil || cleanupErr != nil {
		message := ""
		if stopErr != nil {
			message = stopErr.Error()
		}
		if cleanupErr != nil {
			if message != "" {
				message += "; "
			}
			message += "清理失败: " + cleanupErr.Error()
		}
		r.State.Set(core.StateError, message)
		if stopErr != nil {
			return stopErr
		}
		return cleanupErr
	}
	r.State.Set(core.StateStopped, "")
	r.activeConfig = config.Config{}
	r.activeConfigSet = false
	r.activeData = nil
	r.activeResolved = nil
	return nil
}

func (r *RuntimeCoordinator) fail(code string, err error) error {
	if r.Capture != nil {
		_ = r.Capture.Stop()
	}
	if r.TUN != nil {
		r.TUN.MarkStopped()
	}
	cleanupErr := r.cleanupTUN()
	if cleanupErr != nil {
		if err == nil {
			err = cleanupErr
		} else {
			err = fmt.Errorf("%w; 清理失败: %v", err, cleanupErr)
		}
	}
	if err == nil {
		err = errors.New("未知运行时错误")
	}
	if r.State != nil {
		r.State.Set(core.StateError, code+": "+err.Error())
	}
	return fmt.Errorf("%s: %w", code, err)
}

// atomicWrite 先写同目录临时文件；check 不为空时校验临时文件，通过后才替换 path。
func atomicWrite(path string, data []byte, check func(tmp string) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".runtime-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
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
	if check != nil {
		if err := check(name); err != nil {
			return err
		}
	}
	// 含本地认证密钥，只有本人可读；sudo/pkexec 下交还调用者（内核更新校验要读它）。
	if err := config.ShareWithInvoker(name, 0o600); err != nil {
		return err
	}
	if err := config.ReplaceConfigFile(name, path); err != nil {
		return fmt.Errorf("替换运行配置失败: %w", err)
	}
	return nil
}
