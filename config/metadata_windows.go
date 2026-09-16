//go:build windows

package config

import "os"

func preserveFileMetadata(path string, info os.FileInfo) error {
	return os.Chmod(path, info.Mode().Perm())
}

func applyInvokerOwnership(path string) error {
	return nil
}
