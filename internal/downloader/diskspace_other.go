//go:build !(linux || darwin || windows)

package downloader

// checkDiskSpace checks if there's enough free space available based on config
func (m *Manager) checkDiskSpace() error {
	// For other platforms where disk space checking might not be supported
	// we'll just return nil to allow the operation to continue
	// This is a fallback implementation for unsupported platforms
	return nil
}
