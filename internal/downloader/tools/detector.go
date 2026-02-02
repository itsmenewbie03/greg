// Package tools provides utilities for detecting and managing external download tools
package tools

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ToolType represents the type of download tool
type ToolType int

const (
	// ToolYTDLP represents yt-dlp download tool
	ToolYTDLP ToolType = iota
	// ToolFFmpeg represents ffmpeg download/processing tool
	ToolFFmpeg
)

// String returns the string representation of ToolType
func (t ToolType) String() string {
	switch t {
	case ToolYTDLP:
		return "yt-dlp"
	case ToolFFmpeg:
		return "ffmpeg"
	default:
		return "unknown"
	}
}

// ToolInfo contains information about an external tool
type ToolInfo struct {
	Type      ToolType // Type of tool
	Binary    string   // Full path to binary
	Version   string   // Version string
	Available bool     // Whether tool is available on system
}

// DetectTools detects available download tools on the system
// Returns info for both yt-dlp and ffmpeg, along with any error
func DetectTools() (ytdlp *ToolInfo, ffmpeg *ToolInfo, err error) {
	// Detect yt-dlp
	ytdlp = &ToolInfo{Type: ToolYTDLP}
	ytdlpPath, ytdlpErr := FindTool("yt-dlp")
	if ytdlpErr == nil {
		ytdlp.Binary = ytdlpPath
		ytdlp.Available = true
		ytdlp.Version, _ = GetVersion(ytdlpPath)
	}

	// Detect ffmpeg
	ffmpeg = &ToolInfo{Type: ToolFFmpeg}
	ffmpegPath, ffmpegErr := FindTool("ffmpeg")
	if ffmpegErr == nil {
		ffmpeg.Binary = ffmpegPath
		ffmpeg.Available = true
		ffmpeg.Version, _ = GetVersion(ffmpegPath)
	}

	// At least one tool must be available
	if !ytdlp.Available && !ffmpeg.Available {
		return ytdlp, ffmpeg, fmt.Errorf("neither yt-dlp nor ffmpeg found in PATH. Please install at least one: yt-dlp (preferred) or ffmpeg")
	}

	return ytdlp, ffmpeg, nil
}

// FindTool searches for a tool in the system PATH
// Returns the full path to the binary or an error if not found
func FindTool(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found in PATH: %w", name, err)
	}
	return path, nil
}

// GetVersion attempts to get the version of a tool
// Returns the version string or empty string if unable to determine
func GetVersion(toolPath string) (string, error) {
	// Try --version flag (works for both yt-dlp and ffmpeg)
	cmd := exec.Command(toolPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get version for %s: %w", toolPath, err)
	}

	// Parse version from output
	versionStr := string(output)
	version := parseVersion(versionStr)
	if version == "" {
		return "", fmt.Errorf("failed to parse version from output: %s", versionStr)
	}

	return version, nil
}

// parseVersion extracts version string from tool output
// Handles both yt-dlp and ffmpeg version formats
func parseVersion(output string) string {
	output = strings.TrimSpace(output)
	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return ""
	}

	firstLine := lines[0]

	// yt-dlp format: "2024.08.06" or "yt-dlp 2024.08.06"
	// Look for date-like version (YYYY.MM.DD)
	datePattern := regexp.MustCompile(`(\d{4}\.\d{2}\.\d{2})`)
	if matches := datePattern.FindStringSubmatch(firstLine); len(matches) > 1 {
		return matches[1]
	}

	// ffmpeg format: "ffmpeg version 6.0" or "ffmpeg version N-112345-g1234567"
	// Look for "version X.Y" or "version X.Y.Z"
	versionPattern := regexp.MustCompile(`version\s+([^\s,]+)`)
	if matches := versionPattern.FindStringSubmatch(firstLine); len(matches) > 1 {
		return matches[1]
	}

	// Generic version pattern: X.Y or X.Y.Z
	genericPattern := regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`)
	if matches := genericPattern.FindStringSubmatch(firstLine); len(matches) > 1 {
		return matches[1]
	}

	// Return first line if we can't parse a specific version
	if len(firstLine) > 0 && len(firstLine) < 100 {
		return firstLine
	}

	return ""
}

// ValidateTool checks if a tool is available and functional
func ValidateTool(toolPath string) error {
	// Check if file exists and is executable
	if _, err := exec.LookPath(toolPath); err != nil {
		return fmt.Errorf("tool not found or not executable: %s: %w", toolPath, err)
	}

	// Try to run --version to verify it works
	cmd := exec.Command(toolPath, "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tool exists but failed to run: %s: %w", toolPath, err)
	}

	return nil
}
