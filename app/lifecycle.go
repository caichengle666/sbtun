package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core"
	"github.com/caichengle666/sbtun/core/singbox"
	"github.com/caichengle666/sbtun/core/tun"
)

const (
	ErrConfigInvalid      = "CONFIG_INVALID"
	ErrSingBoxStartFailed = "SINGBOX_START_FAILED"
	ErrTunNotReady        = "TUN_NOT_READY"
)

// RuntimeCoordinator 串起“生成配置 -> 校验 -> 启动 -> 验证 TUN -> 运行”的完整状态机。
// readiness 检查失败时绝不会把状态标记为 Running。
type RuntimeCoordinator struct {
	State    *core.StateStore
	SingBox  *singbox.Manager
	TUN      *tun.Manager
	WorkDir  string
	Binary   string
	ReadyCheck func(context.Context) error
}

func NewRuntimeCoordinator(workDir, binary string) *RuntimeCoordinator {
	return &RuntimeCoordinator{
		State: core.NewStateStore(),
		SingBox: singbox.NewManager(),
		TUN: tun.NewManager(),
		WorkDir: workDir,
		Binary: binary,
	}
}

func (r *RuntimeCoordinator) Start(ctx context.Context, cfg config.Config) error {
	r.State.Set(core.StateStarting, "")
	if err := config.Validate(cfg); err != nil {
		return r.fail(ErrConfigInvalid, err)
	}
	data, err := singbox.BuildConfig(cfg)
	if err != nil {
		return r.fail(ErrConfigInvalid, err)
	}
	if err := os.MkdirAll(r.WorkDir, 0o755); err != nil {
		return r.fail(ErrConfigInvalid, fmt.Errorf("创建运行目录失败: %w", err))
	}
	configPath := filepath.Join(r.WorkDir, "runtime.json")
	if err := atomicWrite(configPath, data); err != nil {
		return r.fail(ErrConfigInvalid, err)
	}
	if err := singbox.ValidateConfig(ctx, r.Binary, configPath); err != nil {
		return r.fail(ErrConfigInvalid, err)
	}
	if err := r.SingBox.Start(ctx, r.Binary, configPath); err != nil {
		return r.fail(ErrSingBoxStartFailed, err)
	}
	if r.ReadyCheck != nil {
		readyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = r.ReadyCheck(readyCtx)
		cancel()
		if err != nil {
			_ = r.SingBox.Stop()
			return r.fail(ErrTunNotReady, err)
		}
	}
	r.TUN.MarkRunning()
	r.State.Set(core.StateRunning, "")
	return nil
}

func (r *RuntimeCoordinator) Stop() error {
	r.State.Set(core.StateStopping, "")
	err := r.SingBox.Stop()
	r.TUN.MarkStopped()
	if err != nil {
		r.State.Set(core.StateError, err.Error())
		return err
	}
	r.State.Set(core.StateStopped, "")
	return nil
}

func (r *RuntimeCoordinator) fail(code string, err error) error {
	r.TUN.MarkStopped()
	r.State.Set(core.StateError, code+": "+err.Error())
	return fmt.Errorf("%s: %w", code, err)
}

func atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".runtime-*.tmp")
	if err != nil { return err }
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil { tmp.Close(); return err }
	if err := tmp.Sync(); err != nil { tmp.Close(); return err }
	if err := tmp.Close(); err != nil { return err }
	return os.Rename(name, path)
}
