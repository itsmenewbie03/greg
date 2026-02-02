//go:build !windows

package mpv

// isPipeReady is a no-op on Unix systems (sockets are checked differently)
func isPipeReady(pipePath string) bool {
	return false
}
