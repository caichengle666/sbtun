//go:build windows

package app

import "path/filepath"

func runtimeDataDir(exeDir string) string {
	return filepath.Join(exeDir, "runtime-data")
}
