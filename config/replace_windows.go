//go:build windows

package config

import (
	"errors"
	"os"
)

func replaceConfigFile(tempPath, targetPath string) error {
	// Windows does not permit os.Rename to overwrite an existing file.
	// Remove the old file only after the replacement has been fully written
	// and closed; then rename the durable temporary file into place.
	if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tempPath, targetPath)
}
