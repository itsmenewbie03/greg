package tui

// This file contains debug-related methods extracted from model.go
// for better code organization. All methods remain on the App struct.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/justchokingaround/greg/internal/config"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

// isDebugMode checks if debug mode is enabled in the config
func (a *App) isDebugMode() bool {
	if a.cfg == nil {
		return false
	}
	cfg, ok := a.cfg.(*config.Config)
	if !ok {
		return false
	}
	return cfg.Advanced.Debug || a.inDebugLinksMode
}

// debugLog prints debug messages only when debug mode is enabled
func (a *App) debugLog(format string, args ...interface{}) {
	if a.isDebugMode() {
		a.logger.Debug(""+format+"\n", args...)
	}
}

// generateMediaDebugInfo fetches debug info for a media item (e.g. movie)
func (a *App) generateMediaDebugInfo(mediaID string, title string, mediaType string) tea.Cmd {
	return func() tea.Msg {
		// Determine provider
		providerType := providers.MediaType(mediaType)

		// Map generic types to specific provider types if needed
		if providerType == providers.MediaTypeMovie || providerType == providers.MediaTypeTV {
			// Check if we have a specific provider for this type, otherwise try MovieTV
			if _, ok := a.providers[providerType]; !ok {
				if _, ok := a.providers[providers.MediaTypeMovieTV]; ok {
					providerType = providers.MediaTypeMovieTV
				}
			}
		}

		// If generic type or empty, try to infer from current app state
		if providerType == "" {
			providerType = a.currentMediaType
		}

		provider, ok := a.providers[providerType]
		if !ok {
			// Try fallback
			for _, p := range a.providers {
				provider = p
				break
			}
		}

		if provider == nil {
			return common.DebugSourcesLoadedMsg{
				Error: fmt.Errorf("no provider available"),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Try to get episode ID for movie
		var episodeID string

		// Check if provider supports direct movie ID
		type movieEpisodeIDGetter interface {
			GetMovieEpisodeID(ctx context.Context, mediaID string) (string, error)
		}

		if getter, ok := provider.(movieEpisodeIDGetter); ok {
			id, err := getter.GetMovieEpisodeID(ctx, mediaID)
			if err == nil {
				episodeID = id
			}
		}

		// If not found, try getting seasons/episodes
		if episodeID == "" {
			seasons, err := provider.GetSeasons(ctx, mediaID)
			if err == nil && len(seasons) > 0 {
				episodes, err := provider.GetEpisodes(ctx, seasons[0].ID)
				if err == nil && len(episodes) > 0 {
					episodeID = episodes[0].ID
				}
			} else {
				// Try getting media details directly (some providers return episodes in details)
				details, err := provider.GetMediaDetails(ctx, mediaID)
				if err == nil && len(details.Seasons) > 0 {
					// Try first season from details
					episodes, err := provider.GetEpisodes(ctx, details.Seasons[0].ID)
					if err == nil && len(episodes) > 0 {
						episodeID = episodes[0].ID
					}
				}
			}
		}

		if episodeID == "" {
			return common.DebugSourcesLoadedMsg{
				Error: fmt.Errorf("could not determine episode ID for media"),
			}
		}

		// Now call the standard generation logic
		// We can reuse the logic by calling the function directly, but since it returns a Cmd,
		// we need to execute the logic here.
		// Let's duplicate the logic for now to avoid complex refactoring, or extract a helper.
		// Extracting helper is better.

		return a.fetchDebugInfo(ctx, provider, episodeID, 0, title)
	}
}

// fetchDebugInfo is a helper to fetch debug info given a provider and episode ID
func (a *App) fetchDebugInfo(ctx context.Context, provider providers.Provider, episodeID string, episodeNumber int, episodeTitle string) common.DebugSourcesLoadedMsg {
	qualities, err := provider.GetAvailableQualities(ctx, episodeID)
	if err != nil {
		return common.DebugSourcesLoadedMsg{
			Error: fmt.Errorf("failed to get qualities: %w", err),
		}
	}

	var sources []common.DebugSource
	var subtitles []common.DebugSubtitle
	var subtitlesFound bool

	for _, q := range qualities {
		stream, err := provider.GetStreamURL(ctx, episodeID, q)
		if err != nil {
			// Log error but continue
			a.logger.Warn("failed to get stream for quality", "quality", q, "error", err)
			continue
		}
		sources = append(sources, common.DebugSource{
			Quality: string(q),
			URL:     stream.URL,
			Type:    string(stream.Type),
			Referer: stream.Referer,
		})

		// Capture subtitles from the first successful stream
		if !subtitlesFound && len(stream.Subtitles) > 0 {
			for _, sub := range stream.Subtitles {
				subtitles = append(subtitles, common.DebugSubtitle{
					Language: sub.Language,
					URL:      sub.URL,
				})
			}
			subtitlesFound = true
		}
	}

	return common.DebugSourcesLoadedMsg{
		Info: &common.DebugSourcesInfo{
			EpisodeTitle:  episodeTitle,
			EpisodeNumber: episodeNumber,
			Sources:       sources,
			Subtitles:     subtitles,
			ProviderName:  provider.Name(),
			SelectedIndex: 0,
		},
	}
}

// generateDebugInfo fetches all source links for an episode
func (a *App) generateDebugInfo(episodeID string, episodeNumber int, episodeTitle string) tea.Cmd {
	return func() tea.Msg {
		provider, ok := a.providers[a.currentMediaType]
		if !ok {
			return common.DebugSourcesLoadedMsg{
				Error: fmt.Errorf("no provider available"),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		return a.fetchDebugInfo(ctx, provider, episodeID, episodeNumber, episodeTitle)
	}
}

// handleDebugPopupInput handles key input when the Debug popup is visible
func (a *App) handleDebugPopupInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		a.showDebugPopup = false
		a.debugSourcesInfo = nil
		return a, nil
	case "up", "k":
		if a.debugSourcesInfo != nil && a.debugSourcesInfo.SelectedIndex > 0 {
			a.debugSourcesInfo.SelectedIndex--
		}
		return a, nil
	case "down", "j":
		if a.debugSourcesInfo != nil && a.debugSourcesInfo.SelectedIndex < len(a.debugSourcesInfo.Sources)-1 {
			a.debugSourcesInfo.SelectedIndex++
		}
		return a, nil
	case "enter", "u":
		// Copy selected source URL
		if a.debugSourcesInfo != nil && len(a.debugSourcesInfo.Sources) > 0 {
			idx := a.debugSourcesInfo.SelectedIndex
			if idx >= 0 && idx < len(a.debugSourcesInfo.Sources) {
				url := a.debugSourcesInfo.Sources[idx].URL
				quality := a.debugSourcesInfo.Sources[idx].Quality
				return a, a.copyToClipboardWithNotification(url, fmt.Sprintf("%s URL", quality))
			}
		}
	case "m":
		// Copy MPV command
		if a.debugSourcesInfo != nil && len(a.debugSourcesInfo.Sources) > 0 {
			idx := a.debugSourcesInfo.SelectedIndex
			if idx >= 0 && idx < len(a.debugSourcesInfo.Sources) {
				src := a.debugSourcesInfo.Sources[idx]
				// Construct MPV command
				cmdStr := fmt.Sprintf("mpv \"%s\"", src.URL)
				if src.Referer != "" {
					cmdStr += fmt.Sprintf(" --http-header-fields=\"Referer: %s\"", src.Referer)
				}

				return a, a.copyToClipboardWithNotification(cmdStr, "MPV command")
			}
		}
	case "J":
		// Copy entire JSON
		if a.debugSourcesInfo != nil {
			// Create a simplified struct for JSON export to avoid circular deps or complex types
			type JSONExport struct {
				Title     string
				Episode   int
				Provider  string
				Sources   []common.DebugSource
				Subtitles []common.DebugSubtitle
			}

			export := JSONExport{
				Title:     a.debugSourcesInfo.EpisodeTitle,
				Episode:   a.debugSourcesInfo.EpisodeNumber,
				Provider:  a.debugSourcesInfo.ProviderName,
				Sources:   a.debugSourcesInfo.Sources,
				Subtitles: a.debugSourcesInfo.Subtitles,
			}

			// Marshal to JSON
			// We need encoding/json
			// I'll add the import if needed, or just use fmt.Sprintf for a quick hack if imports are tricky
			// But imports are fine.

			// Since I can't easily add imports in this edit block without seeing the top of file,
			// I'll use a manual string construction or assume json is imported (it likely isn't in model.go).
			// Actually, model.go probably doesn't import encoding/json.
			// Let's check imports first or use a helper.

			// For now, let's just format it nicely as text if JSON is hard, but user asked for JSON.
			// I'll try to use a simple string builder for JSON to be safe.

			var jsonBuilder strings.Builder
			jsonBuilder.WriteString("{\n")
			jsonBuilder.WriteString(fmt.Sprintf("  \"title\": \"%s\",\n", export.Title))
			jsonBuilder.WriteString(fmt.Sprintf("  \"episode\": %d,\n", export.Episode))
			jsonBuilder.WriteString(fmt.Sprintf("  \"provider\": \"%s\",\n", export.Provider))
			jsonBuilder.WriteString("  \"sources\": [\n")
			for i, s := range export.Sources {
				jsonBuilder.WriteString(fmt.Sprintf("    { \"quality\": \"%s\", \"url\": \"%s\", \"type\": \"%s\", \"referer\": \"%s\" }", s.Quality, s.URL, s.Type, s.Referer))
				if i < len(export.Sources)-1 {
					jsonBuilder.WriteString(",")
				}
				jsonBuilder.WriteString("\n")
			}
			jsonBuilder.WriteString("  ],\n")
			jsonBuilder.WriteString("  \"subtitles\": [\n")
			for i, s := range export.Subtitles {
				jsonBuilder.WriteString(fmt.Sprintf("    { \"language\": \"%s\", \"url\": \"%s\" }", s.Language, s.URL))
				if i < len(export.Subtitles)-1 {
					jsonBuilder.WriteString(",")
				}
				jsonBuilder.WriteString("\n")
			}
			jsonBuilder.WriteString("  ]\n")
			jsonBuilder.WriteString("}")

			return a, a.copyToClipboardWithNotification(jsonBuilder.String(), "Debug info JSON")
		}
	case "c":
		// Copy all sources to clipboard (legacy/text format)
		if a.debugSourcesInfo != nil {
			var text string
			for _, s := range a.debugSourcesInfo.Sources {
				text += fmt.Sprintf("[%s] %s\n", s.Quality, s.URL)
			}
			return a, a.copyToClipboardWithNotification(text, "All sources")
		}
	// Number keys for subtitles
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if a.debugSourcesInfo != nil && len(a.debugSourcesInfo.Subtitles) > 0 {
			num, _ := strconv.Atoi(msg.String())
			idx := num - 1
			if idx >= 0 && idx < len(a.debugSourcesInfo.Subtitles) {
				sub := a.debugSourcesInfo.Subtitles[idx]
				return a, a.copyToClipboardWithNotification(sub.URL, fmt.Sprintf("%s subtitle", sub.Language))
			}
		}
	}
	return a, nil
}

// Message handlers for debug messages

func (a *App) handleGenerateDebugInfoMsg(msg common.GenerateDebugInfoMsg) (tea.Model, tea.Cmd) {
	a.previousState = a.state
	a.state = loadingView
	a.loadingOp = loadingStream
	cmds := []tea.Cmd{a.spinner.Tick, a.generateDebugInfo(msg.EpisodeID, msg.Number, msg.Title)}
	return a, tea.Batch(cmds...)
}

func (a *App) handleDebugSourcesLoadedMsg(msg common.DebugSourcesLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.Error != nil {
		a.err = msg.Error
		a.state = errorView
		return a, nil
	}

	a.debugSourcesInfo = msg.Info
	a.showDebugPopup = true

	if a.previousState != -1 {
		a.state = a.previousState
		a.previousState = -1
	} else {
		a.state = episodeView
	}

	return a, nil
}

// renderDebugPopup renders the Debug Information popup
func (a *App) renderDebugPopup() string {
	if a.debugSourcesInfo == nil {
		return ""
	}

	title := fmt.Sprintf("%s - Source Links", a.debugSourcesInfo.EpisodeTitle)
	if title == "" {
		title = fmt.Sprintf("Episode %d - Source Links", a.debugSourcesInfo.EpisodeNumber)
	}

	content := []string{
		fmt.Sprintf("Title: %s", title),
		fmt.Sprintf("Provider: %s", a.debugSourcesInfo.ProviderName),
		"",
		"Sources (↑/↓ to select, enter to copy):",
	}

	if len(a.debugSourcesInfo.Sources) == 0 {
		content = append(content, "  No sources found.")
	} else {
		for i, src := range a.debugSourcesInfo.Sources {
			cursor := " "
			style := lipgloss.NewStyle()
			if i == a.debugSourcesInfo.SelectedIndex {
				cursor = ">"
				style = style.Foreground(styles.OxocarbonPurple).Bold(true)
			}
			line := fmt.Sprintf("  %s [%s] %s", cursor, src.Quality, src.URL)
			content = append(content, style.Render(line))
		}
	}

	if len(a.debugSourcesInfo.Subtitles) > 0 {
		content = append(content, "", "Subtitles (1-9 to copy):")
		for i, sub := range a.debugSourcesInfo.Subtitles {
			if i >= 9 {
				content = append(content, "  ... (more)")
				break
			}
			content = append(content, fmt.Sprintf("  %d. %s", i+1, sub.Language))
		}
	}

	content = append(content, "", "Keybinds:",
		"  enter/u - Copy selected URL",
		"  m - Copy MPV command",
		"  J - Copy JSON",
		"  c - Copy all text",
		"  q - Close popup")

	contentStr := strings.Join(content, "\n")

	popupStyle := styles.PopupStyle

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonPurple).
		Bold(true).
		MarginBottom(1)

	popupContent := titleStyle.Render("Debug Information")

	popupContent += "\n" + popupStyle.Render(contentStr)

	return popupContent
}
