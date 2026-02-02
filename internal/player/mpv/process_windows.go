//go:build windows

package mpv

import (
	"os/exec"
	"syscall"
)

// setupProcessAttributes configures the process to run detached from the console
// This is critical on Windows to prevent mpv from interfering with TUI keyboard input
func setupProcessAttributes(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// CREATE_NEW_PROCESS_GROUP: Creates a new process group
		// This detaches mpv from the console's Ctrl+C handler
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
