//go:build !darwin

package app

// appBaseDir is the portable install directory itself on Windows and Linux.
func appBaseDir(exeDir string) string { return exeDir }
