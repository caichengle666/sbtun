package config

// WithFileLock serializes operations that touch a shared file across
// processes using the platform-specific lock implementation.
func WithFileLock(path string, action func() error) error {
	return withConfigLock(path, action)
}
