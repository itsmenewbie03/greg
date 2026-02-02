package tui

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justchokingaround/greg/internal/providers"
	"gorm.io/gorm"
)

// DebugInfo holds the debug information for media
type DebugInfo struct {
	MediaTitle    string
	EpisodeTitle  string
	EpisodeNumber int
	StreamURL     string
	Quality       providers.Quality
	Type          providers.StreamType
	Referer       string
	Headers       map[string]string
	Subtitles     []providers.Subtitle
	Error         error
}

// Start is the entry point for the TUI.
// Returns debug information if in debug mode, otherwise nil.
func Start(providers map[providers.MediaType]providers.Provider, trackerMgr interface{}, db *gorm.DB, cfg interface{}, logger *slog.Logger, audioPreference string) *DebugInfo {
	m := NewApp(providers, db, cfg, logger, audioPreference)
	m.trackerMgr = trackerMgr
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}

	// Return debug info if in debug mode
	return m.debugInfo
}

// StartDebugLinks is the entry point for the TUI in debug links mode.
// Returns debug information that should be printed after TUI exit.
func StartDebugLinks(providers map[providers.MediaType]providers.Provider, trackerMgr interface{}, db *gorm.DB, cfg interface{}, logger *slog.Logger, audioPreference string) *DebugInfo {
	m := NewApp(providers, db, cfg, logger, audioPreference)
	m.trackerMgr = trackerMgr
	m.inDebugLinksMode = true
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}

	// Return the stored debug info from the app model
	return m.debugInfo
}

// PrintDebugJSON formats and prints DebugInfo as JSON to stdout.
// Used by the debug command to output debug information.
func PrintDebugJSON(debugInfo *DebugInfo) {
	// Extract referer from headers if not set directly
	referer := debugInfo.Referer
	if referer == "" && debugInfo.Headers != nil {
		if r, ok := debugInfo.Headers["Referer"]; ok {
			referer = r
		}
	}

	output := map[string]interface{}{
		"media_title":    debugInfo.MediaTitle,
		"episode_title":  debugInfo.EpisodeTitle,
		"episode_number": debugInfo.EpisodeNumber,
		"stream_url":     debugInfo.StreamURL,
		"quality":        debugInfo.Quality,
		"type":           debugInfo.Type,
		"referer":        referer,
		"subtitles":      debugInfo.Subtitles,
	}

	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling debug info to JSON: %v\n", err)
		return
	}

	fmt.Println(string(jsonBytes))
}
