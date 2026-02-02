package mpv

import (
	"os"
	"runtime"
	"testing"
)

func TestDetectPlatform(t *testing.T) {
	platform := DetectPlatform()

	switch runtime.GOOS {
	case "windows":
		if platform != PlatformWindows {
			t.Errorf("Expected PlatformWindows, got %v", platform)
		}
	case "darwin":
		if platform != PlatformMac {
			t.Errorf("Expected PlatformMac, got %v", platform)
		}
	case "linux":
		// Check if WSL
		if isWSL() {
			if platform != PlatformWSL {
				t.Errorf("Expected PlatformWSL, got %v", platform)
			}
		} else {
			if platform != PlatformLinux {
				t.Errorf("Expected PlatformLinux, got %v", platform)
			}
		}
	}
}

func TestGetMPVExecutable(t *testing.T) {
	tests := []struct {
		platform Platform
		expected string
	}{
		{PlatformLinux, "mpv"},
		{PlatformMac, "mpv"},
		{PlatformWindows, "mpv.exe"},
		{PlatformWSL, "mpv"}, // WSL uses Linux mpv
	}

	for _, tt := range tests {
		result := GetMPVExecutable(tt.platform)
		if result != tt.expected {
			t.Errorf("GetMPVExecutable(%v) = %v, want %v", tt.platform, result, tt.expected)
		}
	}
}

func TestGetIPCConfig(t *testing.T) {
	tests := []struct {
		platform     Platform
		expectedType IPCType
		isSocket     bool
	}{
		{PlatformLinux, IPCUnixSocket, true},
		{PlatformMac, IPCUnixSocket, true},
		{PlatformWSL, IPCUnixSocket, true}, // WSL uses Linux mpv with Unix sockets
		{PlatformWindows, IPCNamedPipe, false},
	}

	for _, tt := range tests {
		config, err := GetIPCConfig(tt.platform)
		if err != nil {
			t.Errorf("GetIPCConfig(%v) returned error: %v", tt.platform, err)
			continue
		}

		if config.Type != tt.expectedType {
			t.Errorf("GetIPCConfig(%v).Type = %v, want %v", tt.platform, config.Type, tt.expectedType)
		}

		if config.IsSocket != tt.isSocket {
			t.Errorf("GetIPCConfig(%v).IsSocket = %v, want %v", tt.platform, config.IsSocket, tt.isSocket)
		}

		if config.Address == "" {
			t.Errorf("GetIPCConfig(%v).Address is empty", tt.platform)
		}
	}
}

func TestIsWSL(t *testing.T) {
	result := isWSL()

	// Only verify on Linux systems
	if runtime.GOOS == "linux" {
		// Read /proc/version
		data, err := os.ReadFile("/proc/version")
		if err != nil {
			t.Skip("Cannot read /proc/version")
		}

		versionStr := string(data)
		expectedWSL := false
		if contains(versionStr, "microsoft") || contains(versionStr, "WSL") {
			expectedWSL = true
		}

		if result != expectedWSL {
			t.Errorf("isWSL() = %v, expected %v (version: %s)", result, expectedWSL, versionStr)
		}
	}
}

// Helper function for case-insensitive substring check
func contains(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	return len(s) >= len(substr) && indexSubstring(s, substr) >= 0
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func indexSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
