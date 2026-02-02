package mpv

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Platform represents the operating system platform
type Platform int

const (
	PlatformLinux Platform = iota
	PlatformWindows
	PlatformWSL
	PlatformMac
)

// IPCConfig holds IPC connection configuration
type IPCConfig struct {
	Type     IPCType
	Address  string
	IsSocket bool // true for Unix sockets, false for TCP
}

// IPCType represents the IPC connection type
type IPCType int

const (
	IPCUnixSocket IPCType = iota
	IPCNamedPipe
	IPCTCP
)

// DetectPlatform detects the current platform
func DetectPlatform() Platform {
	switch runtime.GOOS {
	case "windows":
		return PlatformWindows
	case "darwin":
		return PlatformMac
	case "linux":
		// Check if running under WSL
		if isWSL() {
			return PlatformWSL
		}
		return PlatformLinux
	default:
		return PlatformLinux
	}
}

// isWSL checks if running under Windows Subsystem for Linux
func isWSL() bool {
	// Check /proc/version for "microsoft" or "WSL"
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}

	version := strings.ToLower(string(data))
	return strings.Contains(version, "microsoft") || strings.Contains(version, "wsl")
}

// GetMPVExecutable returns the mpv executable name for the platform
func GetMPVExecutable(platform Platform) string {
	switch platform {
	case PlatformWindows:
		return "mpv.exe"
	case PlatformWSL:
		// For WSL, prefer Linux mpv (Unix sockets work, Windows named pipes don't from WSL)
		// The gopv library cannot connect to Windows named pipes from WSL
		// Using Linux mpv provides better compatibility
		return "mpv"
	case PlatformLinux, PlatformMac:
		return "mpv"
	default:
		return "mpv"
	}
}

// FindMPVExecutable attempts to find the mpv executable path
func FindMPVExecutable(platform Platform) (string, error) {
	executable := GetMPVExecutable(platform)

	// Try to find in PATH
	path, err := exec.LookPath(executable)
	if err == nil {
		return path, nil
	}

	// If WSL and not found, provide helpful error
	if platform == PlatformWSL {
		return "", fmt.Errorf("mpv.exe not found in PATH. Please install mpv on Windows and ensure it's in your Windows PATH")
	}

	return "", fmt.Errorf("%s not found in PATH. Please install mpv", executable)
}

// GetIPCConfig generates an IPC configuration for the platform
func GetIPCConfig(platform Platform) (*IPCConfig, error) {
	switch platform {
	case PlatformLinux, PlatformMac:
		// Use Unix socket
		socketPath, err := generateUnixSocketPath()
		if err != nil {
			return nil, err
		}
		return &IPCConfig{
			Type:     IPCUnixSocket,
			Address:  socketPath,
			IsSocket: true,
		}, nil

	case PlatformWSL:
		// Use Unix socket (Linux mpv in WSL)
		// Windows named pipes are not accessible from gopv when running in WSL
		socketPath, err := generateUnixSocketPath()
		if err != nil {
			return nil, err
		}
		return &IPCConfig{
			Type:     IPCUnixSocket,
			Address:  socketPath,
			IsSocket: true,
		}, nil

	case PlatformWindows:
		// Use named pipe (native Windows IPC)
		pipeName, err := generateNamedPipePath()
		if err != nil {
			return nil, err
		}
		return &IPCConfig{
			Type:     IPCNamedPipe,
			Address:  pipeName,
			IsSocket: false,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported platform")
	}
}

// generateUnixSocketPath generates a Unix socket path
func generateUnixSocketPath() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	suffix := hex.EncodeToString(b)

	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("greg-mpv-%s.sock", suffix))
	return socketPath, nil
}

// generateNamedPipePath generates a Windows named pipe path
func generateNamedPipePath() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	suffix := hex.EncodeToString(b)

	// Windows named pipe format: \\.\pipe\name
	pipeName := fmt.Sprintf(`\\.\pipe\greg-mpv-%s`, suffix)
	return pipeName, nil
}

// GetMPVIPCArgument returns the mpv command-line argument for IPC
func GetMPVIPCArgument(config *IPCConfig) string {
	return fmt.Sprintf("--input-ipc-server=%s", config.Address)
}

// GetGopvConnectionString returns the connection string for gopv library
func GetGopvConnectionString(config *IPCConfig) string {
	switch config.Type {
	case IPCTCP:
		// gopv expects "tcp://host:port" for TCP connections
		return fmt.Sprintf("tcp://%s", config.Address)
	case IPCNamedPipe:
		// gopv should support named pipes directly (verify with testing)
		return config.Address
	case IPCUnixSocket:
		// Unix sockets use the path directly
		return config.Address
	default:
		return config.Address
	}
}
