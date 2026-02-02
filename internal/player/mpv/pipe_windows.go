//go:build windows

package mpv

import (
	"time"

	"github.com/Microsoft/go-winio"
)

// isPipeReady checks if a Windows named pipe is ready for connection
func isPipeReady(pipePath string) bool {
	// Try to connect to the named pipe with a short timeout
	timeout := 200 * time.Millisecond
	conn, err := winio.DialPipe(pipePath, &timeout)
	if err != nil {
		return false
	}

	// Pipe exists and is accessible
	conn.Close()
	return true
}
