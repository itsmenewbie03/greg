//go:build linux || darwin

package downloader

import (
	"fmt"
	"syscall"
)

// checkDiskSpace checks if there's enough free disk space (Linux/macOS)
func (m *Manager) checkDiskSpace() error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(m.config.Path, &stat); err != nil {
		return fmt.Errorf("failed to check disk space: %w", err)
	}

	// Calculate free space in GB
	freeSpaceGB := (stat.Bavail * uint64(stat.Bsize)) / (1024 * 1024 * 1024)

	if int(freeSpaceGB) < m.config.MinFreeSpace {
		return fmt.Errorf("insufficient disk space: %d GB free, %d GB required",
			freeSpaceGB, m.config.MinFreeSpace)
	}

	return nil
}
