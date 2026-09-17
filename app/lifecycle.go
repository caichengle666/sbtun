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
	ErrSingBoxExited      = "SINGBOX_EXITED"
)

type RuntimeCoordinator struct {
	State      *core.StateStore
	SingBox    *singbox.Manager
	TUN        *tun.Manager
	WorkDir    string
	Binary     string
	ReadyCheck func(context.Context) error
}

func NewRuntimeCoordinator(workDir, binary string) *RuntimeCoordinator {
	r := &RuntimeCoordinator{State: core.NewStateStore(), SingBox: singbox.NewManager(), TUN: tun.NewManager(), WorkDir: workDir, Binary: binary}
	go r.watchSingBoxExit()
	return r
}

func (r *RuntimeCoordinator) watchSingBoxExit() {
	for event := range r.SingBox.Exited() {
		if event.Expected {
			continue
		}
		state, _ := r.State.Get()
		if state == core.StateStopping || state == core.StateStopped {
			continue
		}
		r.TUN.MarkStopped()
		message := ErrSingBoxExited
		if event.Err != nil {
			message += ": " + event.Err.Error()
		}
		r.State.Set(core.StateError, message)
		r.cleanupTUN()
	}
}

func (r *RuntimeCoordinator) Start(ctx context.Context, cfg config.Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.State.Set(core.StateStarting, "")
	if err := config.Validate(cfg); err != nil {
		return r.fail(ErrConfigInvalid, err)
	}
	configPath, err := r.SyncConfig(cfg)
	if err != nil {
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
			stopErr := r.SingBox.Stop()
			r.cleanupTUN()
			if stopErr != nil {
				err = fmt.Errorf("%w; 停止失败: %v", err, stopErr)
			}
			return r.fail(ErrTunNotReady, err)
		}
	}
	r.TUN.MarkRunning()
	r.State.Set(core.StateRunning, "")
	return nil
}

func (r *RuntimeCoordinator) SyncConfig(cfg config.Config) (string, error) {
	exeDir := filepath.Dir(r.Binary)
	if exeDir == "." || exeDir == "" {
		exeDir, _ = os.Getwd()
	}
	data, err := singbox.BuildConfig(cfg, exeDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(r.WorkDir, 0o755); err != nil {
		return "", fmt.Errorf("创建运行目录失败: %w", err)
	}
	configPath := filepath.Join(r.WorkDir, "runtime.json")
	if err := atomicWrite(configPath, data); err != nil {
		return "", fmt.Errorf("写入运行配置失败: %w", err)
	}
	return configPath, nil
}

func (r *RuntimeCoordinator) cleanupTUN() {
	if err := r.TUN.Cleanup(); err != nil {
		fmt.Printf("清理 TUN 路由失败: %v\n", err)
	}
}

func (r *RuntimeCoordinator) Stop() error {
	state, _ := r.State.Get()
	if state == core.StateStopped {
		r.TUN.MarkStopped()
		r.cleanupTUN()
		return nil
	}
	r.State.Set(core.StateStopping, "")
	err := r.SingBox.Stop()
	r.cleanupTUN()
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
	r.cleanupTUN()
	r.State.Set(core.StateError, code+": "+err.Error())
	return fmt.Errorf("%s: %w", code, err)
}

func atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".runtime-*.tmp")
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
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除旧运行配置失败: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("替换运行配置失败: %w", err)
	}
	return nil
}
