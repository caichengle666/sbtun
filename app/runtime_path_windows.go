//go:build windows

package app

import (
	"os"
	"path/filepath"
)

func runtimeDataDir(exeDir string) string {
	if base, err := os.UserConfigDir(); err == nil && base != "" {
		return filepath.Join(base, "sbtun")
	}
	return filepath.Join(exeDir, "runtime-data")
}
