package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

// watchSingBoxExit 将 sing-box 的非预期退出同步到 sbtun 状态，避免界面继续显示"运行中"。
func (r *RuntimeCoordinator) watchSingBoxExit() {
	for event := range r.SingBox.Exited() {
		if event.Expected {
			continue
		}
		// Windows reports externally terminated GUI child processes as 0xffffffff.
		// This is not a useful configuration error and should not poison the UI state.
		if exitErr, ok := event.Err.(*exec.ExitError); ok && exitErr.ExitCode() == -1 {
			r.TUN.MarkStopped()
			r.State.Set(core.StateStopped, "")
			r.cleanupTUN()
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
	r.State.Set(core.StateStarting, "")
	if err := config.Validate(cfg); err != nil {
		return r.fail(ErrConfigInvalid, err)
	}
	exeDir := filepath.Dir(r.Binary)
	if exeDir == "." || exeDir == "" {
		exeDir, _ = os.Getwd()
	}
	data, err := singbox.BuildConfig(cfg, exeDir)
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
			r.cleanupTUN()
			return r.fail(ErrTunNotReady, err)
		}
	}
	r.TUN.MarkRunning()
	r.State.Set(core.StateRunning, "")
	return nil
}

func (r *RuntimeCoordinator) cleanupTUN() {
	if err := r.TUN.Cleanup(); err != nil {
		fmt.Printf("清理 TUN 路由失败: %v\n", err)
	}
}

func (r *RuntimeCoordinator) Stop() error {
	r.State.Set(core.StateStopping, "")
	err := r.SingBox.Stop()
	// sing-box 被 kill 后不会自己清理路由和 TUN 网卡，必须由我们来做
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
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
