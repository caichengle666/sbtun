package config

// ReplaceConfigFile atomically replaces targetPath with tempPath using the
// platform-specific replacement implementation.
func ReplaceConfigFile(tempPath, targetPath string) error {
	return replaceConfigFile(tempPath, targetPath)
}
