//go:build unix && !linux && !darwin

package downloader

import (
	"fmt"
	"syscall"
)

// checkDiskSpace checks if there's enough free space available based on config
func (m *Manager) checkDiskSpace() error {
	if m.config.Path == "" {
		return nil // Skip check if no download path is set
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(m.config.Path, &stat); err != nil {
		return fmt.Errorf("failed to get disk stats: %w", err)
	}

	// Available space in bytes
	availableSpace := stat.Bavail * uint64(stat.Bsize)
	requiredSpace := uint64(m.config.MinFreeSpace)

	if requiredSpace > 0 && availableSpace < requiredSpace {
		return fmt.Errorf("insufficient disk space: need %d bytes, available %d bytes", requiredSpace, availableSpace)
	}

	return nil
}
