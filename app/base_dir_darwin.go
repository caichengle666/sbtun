//go:build darwin

package app

import (
	"io"
	"os"
	"path/filepath"
)

// macOS may run a downloaded .app from a read-only translocated path, and the
// bundle is replaced on every update, so sing-box, rules and runtime data live
// in a fixed system directory shared by the GUI and the CLI (both run as root).
const darwinDataDir = "/Library/Application Support/sbtun"

func appBaseDir(exeDir string) string {
	seedDarwinDataDir(exeDir)
	return darwinDataDir
}

// seedDarwinDataDir copies the bundled sing-box and rule sets on first run.
// Errors are ignored: an unprivileged CLI call cannot write here, and the
// elevated run that follows seeds the directory.
func seedDarwinDataDir(exeDir string) {
	if err := os.MkdirAll(filepath.Join(darwinDataDir, "rules"), 0o755); err != nil {
		return
	}
	bundled := filepath.Join(exeDir, singBoxBinaryName())
	target := filepath.Join(darwinDataDir, singBoxBinaryName())
	if newerThan(bundled, target) {
		_ = copyFile(bundled, target, 0o755)
	}
	for _, dir := range []string{filepath.Join(exeDir, "rules"), filepath.Join(exeDir, "..", "Resources", "rules")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			dst := filepath.Join(darwinDataDir, "rules", entry.Name())
			if _, err := os.Stat(dst); os.IsNotExist(err) {
				_ = copyFile(filepath.Join(dir, entry.Name()), dst, 0o644)
			}
		}
		break
	}
}

// newerThan reports whether src exists and dst is missing or older, so an app
// update ships its bundled sing-box while an in-app sing-box update is kept.
func newerThan(src, dst string) bool {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return false
	}
	dstInfo, err := os.Stat(dst)
	return err != nil || srcInfo.ModTime().After(dstInfo.ModTime())
}

// copyFile writes a fresh file, which also drops the quarantine attribute
// that would make Gatekeeper block the bundled sing-box when executed.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
