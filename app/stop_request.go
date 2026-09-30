package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const stopRequestFile = "stop.request"

func RequestRunningInstanceStop() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	workDir := runtimeDataDir(filepath.Dir(executable))
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workDir, stopRequestFile), []byte("stop\n"), 0o600)
}

func (a *App) ClearStopRequest() error {
	if a.workDir == "" {
		return nil
	}
	err := os.Remove(filepath.Join(a.workDir, stopRequestFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (a *App) WaitForStopRequest(ctx context.Context) bool {
	if a.workDir == "" {
		return false
	}
	path := filepath.Join(a.workDir, stopRequestFile)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			_ = os.Remove(path)
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}
