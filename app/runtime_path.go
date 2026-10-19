package app

import "path/filepath"

// runtimeDataDir holds config.json, runtime.json and other mutable state.
func runtimeDataDir(baseDir string) string {
	return filepath.Join(baseDir, "runtime-data")
}
