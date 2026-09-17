package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	mu         sync.Mutex
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
	if r.SingBox == nil || r.State == nil {
		return
	}
	for event := range r.SingBox.Exited() {
		if event.Expected {
			continue
		}
		r.mu.Lock()
		state, _ := r.State.Get()
		if state == core.StateStopping || state == core.StateStopped {
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
		r.State.Set(core.StateError, message)
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
			cleanupErr := r.cleanupTUN()
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
	return nil
}

func (r *RuntimeCoordinator) fail(code string, err error) error {
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
	if err := config.ReplaceConfigFile(name, path); err != nil {
		return fmt.Errorf("替换运行配置失败: %w", err)
	}
	return nil
}
