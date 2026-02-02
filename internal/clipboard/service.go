package clipboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/justchokingaround/greg/internal/config"
)

// Service provides clipboard operations across different platforms
type Service interface {
	// Read reads content from the system clipboard
	Read(ctx context.Context, cfg *config.Config) (string, error)

	// Write copies text to the system clipboard and returns a tea.Cmd for async operations
	Write(ctx context.Context, text string, cfg *config.Config) tea.Cmd
}

// clipboardService implements the Service interface
type clipboardService struct {
	logger Logger
}

// Logger interface for clipboard operations
type Logger interface {
	Debug(msg string, keyvals ...interface{})
	Warn(msg string, keyvals ...interface{})
	Error(msg string, keyvals ...interface{})
}

// NewService creates a new clipboard service
func NewService(logger Logger) Service {
	return &clipboardService{
		logger: logger,
	}
}

// Read reads content from the system clipboard
func (s *clipboardService) Read(ctx context.Context, cfg *config.Config) (string, error) {
	var cmd *exec.Cmd

	// First check if a custom clipboard command is configured
	if cfg != nil && cfg.Advanced.Clipboard.Command != "" {
		// Parse the command string to handle arguments properly
		parts := parseCommand(cfg.Advanced.Clipboard.Command)
		if len(parts) == 0 {
			return "", fmt.Errorf("invalid clipboard command in config: %s", cfg.Advanced.Clipboard.Command)
		}

		if len(parts) == 1 {
			cmd = exec.Command(parts[0])
		} else {
			cmd = exec.Command(parts[0], parts[1:]...)
		}
	} else {
		// Use default OS-specific commands as fallback
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("pbpaste")
		case "linux":
			// Try xclip first, then xsel, then wl-paste (Wayland)
			if _, err := exec.LookPath("xclip"); err == nil {
				cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
			} else if _, err := exec.LookPath("xsel"); err == nil {
				cmd = exec.Command("xsel", "--clipboard", "--output")
			} else if _, err := exec.LookPath("wl-paste"); err == nil {
				cmd = exec.Command("wl-paste")
			} else {
				return "", fmt.Errorf("no clipboard tool found (install xclip, xsel, or wl-clipboard)")
			}
		case "windows":
			cmd = exec.Command("powershell.exe", "-command", "Get-Clipboard")
		default:
			return "", fmt.Errorf("clipboard reading not supported on %s", runtime.GOOS)
		}
	}

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to execute clipboard command: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// Write copies text to the system clipboard and returns a tea.Cmd for async operations
func (s *clipboardService) Write(ctx context.Context, text string, cfg *config.Config) tea.Cmd {
	// Try the primary clipboard package first
	err := clipboard.WriteAll(text)
	if err != nil {
		// Log the error but try fallback method
		s.logger.Warn("failed to copy to clipboard using primary method", "error", err)

		// Get the clipboard command from config as fallback
		var clipboardCmd tea.Cmd
		if cfg != nil && cfg.Advanced.Clipboard.Command != "" {
			clipboardCmd = s.copyWithCommand(text, cfg.Advanced.Clipboard.Command)
		} else {
			// Check if we might be running in WSL and default to clip.exe
			if s.isWSL() {
				clipboardCmd = s.copyWithCommand(text, "clip.exe")
			} else {
				// Default to using xclip or other system utilities
				clipboardCmd = s.copyWithDefault(text)
			}
		}

		// Return the fallback command
		if clipboardCmd != nil {
			return clipboardCmd
		}
	} else {
		// Success with primary method, log it
		s.logger.Debug("successfully copied to clipboard using primary method")
	}

	// Return a no-op command if primary method succeeded
	return func() tea.Msg { return nil }
}

// copyWithDefault copies text using default system clipboard utilities
func (s *clipboardService) copyWithDefault(text string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd

		switch runtime.GOOS {
		case "windows":
			// For Windows, use clip.exe
			cmd = exec.Command("clip.exe")
			s.logger.Debug("using Windows clipboard", "command", "clip.exe")

		case "darwin":
			// For macOS, use pbcopy
			cmd = exec.Command("pbcopy")
			s.logger.Debug("using macOS clipboard", "command", "pbcopy")

		case "linux":
			// Check if we're in WSL first
			if s.isWSL() {
				// On WSL, use Windows clip.exe to interface with the Windows clipboard
				cmd = exec.Command("clip.exe")
				s.logger.Debug("using WSL clipboard", "command", "clip.exe")
			} else {
				// For standard Linux systems, try different clipboard utilities in order of preference
				if s.commandExists("wl-copy") {
					// For Wayland
					cmd = exec.Command("wl-copy")
					s.logger.Debug("using Wayland clipboard", "command", "wl-copy")
				} else if s.commandExists("xclip") {
					// For X11
					cmd = exec.Command("xclip", "-selection", "clipboard")
					s.logger.Debug("using X11 clipboard", "command", "xclip")
				} else if s.commandExists("xsel") {
					// Alternative for X11
					cmd = exec.Command("xsel", "--clipboard", "--input")
					s.logger.Debug("using X11 clipboard (xsel)", "command", "xsel")
				} else {
					// Fallback - try xclip anyway
					cmd = exec.Command("xclip", "-selection", "clipboard")
					s.logger.Debug("using fallback X11 clipboard", "command", "xclip")
				}
			}
		default:
			// For unsupported OS, just return
			s.logger.Warn("unsupported OS for clipboard", "os", runtime.GOOS)
			return nil
		}

		// Set the text as input to the command via stdin
		cmd.Stdin = strings.NewReader(text)

		// Run the command to copy to clipboard - add more error handling
		err := cmd.Run()
		if err != nil {
			// Log detailed error for debugging - include text being copied
			s.logger.Error("failed to copy to clipboard",
				"error", err,
				"os", runtime.GOOS,
				"command", cmd.Path,
				"text_length", len(text),
				"text_sample", text[:minInt(len(text), 50)]) // Log first 50 chars of text for debugging
		} else {
			// On success, log it
			s.logger.Debug("successfully copied to clipboard",
				"os", runtime.GOOS,
				"command", cmd.Path,
				"text_length", len(text))
		}

		return nil
	}
}

// copyWithCommand copies text using a specified command
func (s *clipboardService) copyWithCommand(text, command string) tea.Cmd {
	return func() tea.Msg {
		// More robust command parsing to handle quoted arguments
		parts := parseCommand(command)
		if len(parts) == 0 {
			s.logger.Error("empty clipboard command provided")
			return nil
		}

		var cmd *exec.Cmd
		if len(parts) == 1 {
			cmd = exec.Command(parts[0])
		} else {
			cmd = exec.Command(parts[0], parts[1:]...)
		}

		cmd.Stdin = strings.NewReader(text)

		s.logger.Debug("attempting clipboard command", "command_parts", parts, "text_length", len(text))

		// Use Run() instead of CombinedOutput() as some clipboard tools don't need to capture output
		err := cmd.Run()
		if err != nil {
			s.logger.Error("failed to copy to clipboard with custom command", "error", err, "command", command, "parts", parts)
			// Also try alternative approach for Windows systems that might need different handling
			if strings.ToLower(parts[0]) == "clip.exe" || strings.Contains(strings.ToLower(runtime.GOOS), "windows") {
				// For Windows clip.exe, try using a shell command as alternative
				shellCmd := exec.Command("cmd", "/c", fmt.Sprintf("echo %s | clip", strings.ReplaceAll(text, "\"", "")))
				shellErr := shellCmd.Run()
				if shellErr != nil {
					s.logger.Error("failed to copy to clipboard using shell approach", "error", shellErr, "command", fmt.Sprintf("echo %s | clip", text))
				} else {
					s.logger.Debug("successfully copied to clipboard using shell approach")
				}
			}
		} else {
			s.logger.Debug("custom clipboard command succeeded", "command", command)
		}

		return nil
	}
}

// parseCommand parses a command string into executable parts, respecting quotes
func parseCommand(command string) []string {
	var parts []string
	var currentPart string
	var inQuotes bool
	var quoteChar rune

	for _, char := range command {
		switch {
		case char == '\'' || char == '"':
			if !inQuotes {
				inQuotes = true
				quoteChar = char
			} else if char == quoteChar {
				inQuotes = false
			} else {
				currentPart += string(char)
			}
		case char == ' ' && !inQuotes:
			if currentPart != "" {
				parts = append(parts, currentPart)
				currentPart = ""
			}
		default:
			currentPart += string(char)
		}
	}

	if currentPart != "" {
		parts = append(parts, currentPart)
	}

	return parts
}

// isWSL checks if the application is running in Windows Subsystem for Linux
func (s *clipboardService) isWSL() bool {
	// Check if we're running in WSL by checking the kernel version
	// WSL systems typically have "microsoft" or "WSL" in the uname output
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		// If uname fails, try alternative method - check for WSL-specific files
		_, err := os.Stat("/proc/version")
		if err == nil {
			versionBytes, err := os.ReadFile("/proc/version")
			if err == nil {
				version := strings.ToLower(string(versionBytes))
				return strings.Contains(version, "microsoft") || strings.Contains(version, "wsl")
			}
		}
		return false
	}

	output := strings.ToLower(string(out))
	return strings.Contains(output, "microsoft") || strings.Contains(output, "wsl")
}

// commandExists checks if a command exists on the system
func (s *clipboardService) commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// minInt returns the minimum of two integers
// Helper function to avoid importing math package just for min
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
