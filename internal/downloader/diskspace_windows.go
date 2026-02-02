//go:build windows

package downloader

import (
	"fmt"
	"syscall"
	"unsafe"
)

// checkDiskSpace checks if there's enough free disk space (Windows)
func (m *Manager) checkDiskSpace() error {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytes uint64
	var totalBytes uint64
	var availBytes uint64

	// Convert path to UTF16
	pathPtr, err := syscall.UTF16PtrFromString(m.config.Path)
	if err != nil {
		return fmt.Errorf("failed to convert path: %w", err)
	}

	ret, _, err := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&availBytes)),
	)

	if ret == 0 {
		return fmt.Errorf("failed to check disk space: %w", err)
	}

	// Calculate free space in GB
	freeSpaceGB := freeBytes / (1024 * 1024 * 1024)

	if int(freeSpaceGB) < m.config.MinFreeSpace {
		return fmt.Errorf("insufficient disk space: %d GB free, %d GB required",
			freeSpaceGB, m.config.MinFreeSpace)
	}

	return nil
}
