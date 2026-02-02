//go:build !windows

package mpv

import "os/exec"

// setupProcessAttributes is a no-op on non-Windows platforms
func setupProcessAttributes(cmd *exec.Cmd) {
	// No special setup needed on Unix
}
