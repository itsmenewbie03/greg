package tui

// This file contains WatchParty-related methods extracted from model.go
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
	"github.com/justchokingaround/greg/internal/database"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
	"github.com/justchokingaround/greg/internal/watchparty"
)

// generateWatchPartyURL creates a WatchParty URL for the current episode
func (a *App) generateWatchPartyURL(episodeID string, episodeNumber int, episodeTitle string) tea.Cmd {
	return func() tea.Msg {
		// Get the provider for the current media type
		provider, ok := a.providers[a.currentMediaType]
		if !ok {
			// Try to find any available provider
			for _, p := range a.providers {
				provider = p
				break
			}
			if provider == nil {
				return common.WatchPartyMsg{
					URL: "",
					Err: fmt.Errorf("no provider available"),
				}
			}
		}

		// Get stream URL for the episode
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		stream, err := provider.GetStreamURL(ctx, episodeID, providers.Quality1080p)
		if err != nil {
			return common.WatchPartyMsg{
				URL: "",
				Err: fmt.Errorf("failed to get stream URL: %w", err),
			}
		}

		// Determine proxy configuration - check multiple potential sources
		finalProxyURL := a.getWatchPartyProxy()

		// If no WatchParty-specific proxy is configured, try other potential sources
		if finalProxyURL == "" && a.cfg != nil {
			if cfg, ok := a.cfg.(*config.Config); ok {
				// Try WatchParty default proxy first (double check we didn't miss it)
				if cfg.WatchParty.DefaultProxy != "" && finalProxyURL == "" {
					finalProxyURL = cfg.WatchParty.DefaultProxy
					a.logger.Debug("using watchparty.default_proxy", "proxy", finalProxyURL)
				}
				// Try general network proxy
				if cfg.Network.Proxy != "" && finalProxyURL == "" {
					finalProxyURL = cfg.Network.Proxy
					a.logger.Debug("using network.proxy as fallback for WatchParty", "proxy", finalProxyURL)
				}
				// Try default origin as proxy if it looks like a proxy endpoint
				if cfg.WatchParty.DefaultOrigin != "" && finalProxyURL == "" &&
					(strings.Contains(cfg.WatchParty.DefaultOrigin, "cloudflare") ||
						strings.Contains(cfg.WatchParty.DefaultOrigin, "workers") ||
						strings.Contains(cfg.WatchParty.DefaultOrigin, "proxy")) {
					finalProxyURL = cfg.WatchParty.DefaultOrigin
					a.logger.Debug("using watchparty.default_origin as proxy (looks like proxy)", "origin", finalProxyURL)
				}
				// Try Advanced clipboard command as fallback if it looks like a proxy
				// This might be a misconfigured clipboard command that's actually meant to be a proxy
				// For example if someone set "my-proxy-url.workers.dev" as clipboard command by mistake
				// We won't use this as it's probably not a proxy command
				// Instead let's check if there's a more general advanced proxy field
				// For now, focus on the specific proxy field if it exists
				_ = cfg.Advanced.Clipboard.Command != "" && finalProxyURL == "" &&
					(strings.Contains(cfg.Advanced.Clipboard.Command, "proxy") ||
						strings.Contains(cfg.Advanced.Clipboard.Command, "cloudflare") ||
						strings.Contains(cfg.Advanced.Clipboard.Command, "worker"))
				// Additional fallback checks would go here if there were more proxy fields
				// For now, we've checked all the main potential proxy configuration locations
			}
		}

		// Apply proxy if needed
		videoURL := stream.URL
		if finalProxyURL != "" {
			videoURL, err = watchparty.GenerateProxiedURL(stream.URL, watchparty.ProxyConfig{
				ProxyURL: finalProxyURL,
				Origin:   "", // Will be derived from referer in GenerateProxiedURL
				Referer:  stream.Referer,
			})
			if err != nil {
				return common.WatchPartyMsg{
					URL: "",
					Err: fmt.Errorf("failed to generate proxied URL with proxy %s: %w", finalProxyURL, err),
				}
			}
			a.logger.Debug("proxy applied to video URL", "original", stream.URL, "proxied", videoURL, "proxy", finalProxyURL)
		} else {
			a.logger.Debug("no proxy configured, using original URL", "url", stream.URL)
		}

		// Generate WatchParty URL
		watchPartyFinalURL := watchparty.GenerateWatchPartyURL(videoURL)

		// Log debug information for troubleshooting proxy configuration
		a.logger.Debug("generating WatchParty URL",
			"original_stream_url", stream.URL,
			"proxied_video_url", videoURL,
			"watch_party_url", watchPartyFinalURL,
			"proxy_configured", finalProxyURL != "",
			"proxy_url", finalProxyURL)

		// Prepare WatchParty information for the popup
		wpInfo := &common.WatchPartyInfo{
			URL:           stream.URL,         // Original stream URL
			ProxiedURL:    videoURL,           // Proxied stream URL (same as original if no proxy)
			WatchPartyURL: watchPartyFinalURL, // Complete WatchParty URL with video parameter
			Subtitles:     stream.Subtitles,
			Referer:       stream.Referer,
			Headers:       stream.Headers,
			Title:         a.selectedMedia.Title,
			EpisodeTitle:  episodeTitle,
			EpisodeNumber: episodeNumber,
			ProviderName:  provider.Name(),
		}

		// Get next episode info for "keep watching" functionality
		if a.currentMediaType == providers.MediaTypeAnime {
			// For anime - find next episode in the current season
			for _, ep := range a.episodes {
				if ep.Number == episodeNumber+1 {
					wpInfo.NextEpisodeID = ep.ID
					wpInfo.NextEpisodeTitle = ep.Title
					wpInfo.NextEpisodeNumber = ep.Number
					break
				}
			}
		} else if a.currentMediaType == providers.MediaTypeTV {
			// For TV shows - find next episode considering season/episode format
			for _, ep := range a.episodes {
				if ep.Number == episodeNumber+1 {
					wpInfo.NextEpisodeID = ep.ID
					wpInfo.NextEpisodeTitle = ep.Title
					wpInfo.NextEpisodeNumber = ep.Number
					break
				}
			}
		}

		return common.WatchPartyMsg{
			URL:            watchPartyFinalURL,
			Err:            nil,
			WatchPartyInfo: wpInfo,
		}
	}
}

// generateWatchPartyURLWithProvider creates a WatchParty URL using a specific provider
func (a *App) generateWatchPartyURLWithProvider(provider providers.Provider, episodeID string, episodeNumber int, episodeTitle string) tea.Cmd {
	return func() tea.Msg {
		// Get stream URL for the episode using the specified provider
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		stream, err := provider.GetStreamURL(ctx, episodeID, providers.Quality1080p)
		if err != nil {
			return common.WatchPartyMsg{
				URL: "",
				Err: fmt.Errorf("failed to get stream URL: %w", err),
			}
		}

		// Determine proxy configuration - check multiple potential sources
		finalProxyURL := a.getWatchPartyProxy()

		// If no WatchParty-specific proxy is configured, try other potential sources
		if finalProxyURL == "" && a.cfg != nil {
			if cfg, ok := a.cfg.(*config.Config); ok {
				// Try WatchParty default proxy first (double check we didn't miss it)
				if cfg.WatchParty.DefaultProxy != "" && finalProxyURL == "" {
					finalProxyURL = cfg.WatchParty.DefaultProxy
					a.logger.Debug("using watchparty.default_proxy", "proxy", finalProxyURL)
				}
				// Try general network proxy
				if cfg.Network.Proxy != "" && finalProxyURL == "" {
					finalProxyURL = cfg.Network.Proxy
					a.logger.Debug("using network.proxy as fallback for WatchParty", "proxy", finalProxyURL)
				}
				// Try default origin as proxy if it looks like a proxy endpoint
				if cfg.WatchParty.DefaultOrigin != "" && finalProxyURL == "" &&
					(strings.Contains(cfg.WatchParty.DefaultOrigin, "cloudflare") ||
						strings.Contains(cfg.WatchParty.DefaultOrigin, "workers") ||
						strings.Contains(cfg.WatchParty.DefaultOrigin, "proxy")) {
					finalProxyURL = cfg.WatchParty.DefaultOrigin
					a.logger.Debug("using watchparty.default_origin as proxy (looks like proxy)", "origin", finalProxyURL)
				}
				// Try Advanced clipboard command as fallback if it looks like a proxy
				// This might be a misconfigured clipboard command that's actually meant to be a proxy
				// For example if someone set "my-proxy-url.workers.dev" as clipboard command by mistake
				// We won't use this as it's probably not a proxy command
				// Instead let's check if there's a more general advanced proxy field
				// For now, focus on the specific proxy field if it exists
				_ = cfg.Advanced.Clipboard.Command != "" && finalProxyURL == "" &&
					(strings.Contains(cfg.Advanced.Clipboard.Command, "proxy") ||
						strings.Contains(cfg.Advanced.Clipboard.Command, "cloudflare") ||
						strings.Contains(cfg.Advanced.Clipboard.Command, "worker"))
				// Additional fallback checks would go here if there were more proxy fields
				// For now, we've checked all the main potential proxy configuration locations
			}
		}

		// Apply proxy if needed
		videoURL := stream.URL
		if finalProxyURL != "" {
			videoURL, err = watchparty.GenerateProxiedURL(stream.URL, watchparty.ProxyConfig{
				ProxyURL: finalProxyURL,
				Origin:   "", // Will be derived from referer in GenerateProxiedURL
				Referer:  stream.Referer,
			})
			if err != nil {
				return common.WatchPartyMsg{
					URL: "",
					Err: fmt.Errorf("failed to generate proxied URL with proxy %s: %w", finalProxyURL, err),
				}
			}
			a.logger.Debug("proxy applied to video URL", "original", stream.URL, "proxied", videoURL, "proxy", finalProxyURL)
		} else {
			a.logger.Debug("no proxy configured, using original URL", "url", stream.URL)
		}

		// Generate WatchParty URL
		watchPartyFinalURL := watchparty.GenerateWatchPartyURL(videoURL)

		// Log debug information for troubleshooting proxy configuration
		a.logger.Debug("generating WatchParty URL",
			"original_stream_url", stream.URL,
			"proxied_video_url", videoURL,
			"watch_party_url", watchPartyFinalURL,
			"proxy_configured", finalProxyURL != "",
			"proxy_url", finalProxyURL)

		// Prepare WatchParty information for the popup
		wpInfo := &common.WatchPartyInfo{
			URL:           stream.URL,         // Original stream URL
			ProxiedURL:    videoURL,           // Proxied stream URL (same as original if no proxy)
			WatchPartyURL: watchPartyFinalURL, // Complete WatchParty URL with video parameter
			Subtitles:     stream.Subtitles,
			Referer:       stream.Referer,
			Headers:       stream.Headers,
			Title:         a.selectedMedia.Title,
			EpisodeTitle:  episodeTitle,
			EpisodeNumber: episodeNumber,
			ProviderName:  provider.Name(),
		}

		// Get next episode info for "keep watching" functionality
		if a.currentMediaType == providers.MediaTypeAnime {
			// For anime - find next episode in the current season
			for _, ep := range a.episodes {
				if ep.Number == episodeNumber+1 {
					wpInfo.NextEpisodeID = ep.ID
					wpInfo.NextEpisodeTitle = ep.Title
					wpInfo.NextEpisodeNumber = ep.Number
					break
				}
			}
		} else if a.currentMediaType == providers.MediaTypeTV {
			// For TV shows - find next episode considering season/episode format
			for _, ep := range a.episodes {
				if ep.Number == episodeNumber+1 {
					wpInfo.NextEpisodeID = ep.ID
					wpInfo.NextEpisodeTitle = ep.Title
					wpInfo.NextEpisodeNumber = ep.Number
					break
				}
			}
		}

		return common.WatchPartyMsg{
			URL:            watchPartyFinalURL,
			Err:            nil,
			WatchPartyInfo: wpInfo,
		}
	}
}

// getWatchPartyProxy gets the WatchParty proxy URL (checks temp override first, then config)
func (a *App) getWatchPartyProxy() string {
	// Check for temporary override first
	if a.tempProxyURL != "" {
		return a.tempProxyURL
	}

	// Fall back to config
	if a.cfg == nil {
		return ""
	}
	cfg, ok := a.cfg.(*config.Config)
	if !ok {
		return ""
	}
	return cfg.WatchParty.DefaultProxy
}

// setWatchPartyProxy sets temporary proxy configuration
func (a *App) setWatchPartyProxy(proxyURL, origin string) {
	a.tempProxyURL = proxyURL
	a.tempOrigin = origin
}

// openWatchPartyInBrowser opens the WatchParty URL in the default browser
func (a *App) openWatchPartyInBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		err := watchparty.OpenURL(url)
		return common.OpenWatchPartyMsg{
			URL: url,
			Err: err,
		}
	}
}

// handleWatchPartyPopupInput handles key input when the WatchParty popup is visible
func (a *App) handleWatchPartyPopupInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		// Close the popup
		a.showWatchPartyPopup = false
		a.watchPartyInfo = nil
		return a, nil

	case "u":
		// Copy direct URL to clipboard
		if a.watchPartyInfo != nil {
			return a, a.copyToClipboardWithNotification(a.watchPartyInfo.URL, "Direct URL")
		}

	case "p":
		// Copy proxied URL to clipboard
		if a.watchPartyInfo != nil {
			return a, a.copyToClipboardWithNotification(a.watchPartyInfo.ProxiedURL, "Proxied URL")
		}

	case "w":
		// Copy WatchParty URL to clipboard
		if a.watchPartyInfo != nil {
			return a, a.copyToClipboardWithNotification(a.watchPartyInfo.WatchPartyURL, "WatchParty URL")
		}

	case "r":
		// Copy referer to clipboard
		if a.watchPartyInfo != nil && a.watchPartyInfo.Referer != "" {
			return a, a.copyToClipboardWithNotification(a.watchPartyInfo.Referer, "Referer")
		}

	// Number keys 1-9 for copying subtitle URLs
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Copy specific subtitle URL to clipboard based on number pressed
		if a.watchPartyInfo != nil && len(a.watchPartyInfo.Subtitles) > 0 {
			// Convert the pressed key to an index (0-based)
			num, err := strconv.Atoi(msg.String())
			if err != nil || num < 1 || num > len(a.watchPartyInfo.Subtitles) {
				// Invalid number pressed, just continue
				return a, nil
			}

			// Adjust for 0-based indexing
			index := num - 1
			if index < len(a.watchPartyInfo.Subtitles) {
				selectedSubtitle := a.watchPartyInfo.Subtitles[index]
				return a, a.copyToClipboardWithNotification(
					selectedSubtitle.URL,
					fmt.Sprintf("Subtitle '%s' URL", selectedSubtitle.Language),
				)
			}
		}

	case "s":
		// Copy all subtitles to clipboard
		if a.watchPartyInfo != nil && len(a.watchPartyInfo.Subtitles) > 0 {
			var subtitleURLs []string
			for _, sub := range a.watchPartyInfo.Subtitles {
				subtitleURLs = append(subtitleURLs, sub.URL)
			}
			subtitlesStr := strings.Join(subtitleURLs, "\n")
			return a, a.copyToClipboardWithNotification(subtitlesStr, "All subtitle URLs")
		}

	case "o":
		// Open the WatchParty URL in the browser
		if a.watchPartyInfo != nil && a.watchPartyInfo.WatchPartyURL != "" {
			cmd := a.openWatchPartyInBrowser(a.watchPartyInfo.WatchPartyURL) // Use the complete WatchParty URL: watchparty.me/create?video=PROXIED_URL
			a.statusMsg = "✓ Opening WatchParty in browser..."
			a.statusMsgTime = time.Now()
			return a, tea.Batch(cmd, func() tea.Msg {
				time.Sleep(3 * time.Second)
				return clearStatusMsg{}
			})
		}

	case "k":
		// Keep watching next episode (if available)
		if a.watchPartyInfo != nil && a.watchPartyInfo.NextEpisodeID != "" {
			// Close the popup and start playing the next episode
			a.showWatchPartyPopup = false
			// Update history to mark current episode as watched (if needed)
			if a.historyService != nil && a.selectedMedia.Title != "" {
				// Record the current episode as completed in history
				historyRecord := database.History{
					MediaID:         a.selectedMedia.ID,
					MediaTitle:      a.selectedMedia.Title,
					MediaType:       string(a.selectedMedia.Type),
					Episode:         a.watchPartyInfo.EpisodeNumber,
					Season:          a.currentSeasonNumber,
					ProviderName:    a.watchPartyInfo.ProviderName,
					ProgressSeconds: 0, // For a completed episode, we're not tracking progress, just completion
					TotalSeconds:    0,
					ProgressPercent: 100.0, // Marked as 100% since it's completed
					WatchedAt:       time.Now(),
					Completed:       true,
				}
				_ = a.historyService.AddOrUpdate(historyRecord)
			}

			// Prepare to play the next episode
			return a, func() tea.Msg {
				return common.EpisodeSelectedMsg{
					EpisodeID: a.watchPartyInfo.NextEpisodeID,
					Number:    a.watchPartyInfo.NextEpisodeNumber,
					Title:     a.watchPartyInfo.NextEpisodeTitle,
				}
			}
		}
	}

	// For any other key, just close the popup
	a.showWatchPartyPopup = false
	a.watchPartyInfo = nil
	return a, nil
}

// renderWatchPartyPopup renders the WatchParty information popup
func (a *App) renderWatchPartyPopup() string {
	if a.watchPartyInfo == nil {
		return ""
	}

	title := fmt.Sprintf("%s - WatchParty Info", a.watchPartyInfo.Title)
	if a.watchPartyInfo.EpisodeTitle != "" && a.watchPartyInfo.EpisodeTitle != a.watchPartyInfo.Title {
		title = fmt.Sprintf("%s - %s (Ep %d)", a.watchPartyInfo.Title, a.watchPartyInfo.EpisodeTitle, a.watchPartyInfo.EpisodeNumber)
	}

	// Create the popup content
	content := []string{
		fmt.Sprintf("Title: %s", title),
		fmt.Sprintf("Provider: %s", a.watchPartyInfo.ProviderName),
		"",
		"URLs:",
		fmt.Sprintf("  Direct: %s", a.watchPartyInfo.URL),
		fmt.Sprintf("  Proxied: %s", a.watchPartyInfo.ProxiedURL),
		fmt.Sprintf("  WatchParty: %s", a.watchPartyInfo.WatchPartyURL),
		"",
		"Keybinds:",
		"  u - Copy direct URL to clipboard",
		"  p - Copy proxied URL to clipboard",
		"  w - Copy WatchParty URL to clipboard",
		"  o - Open in browser",
		"  r - Copy referer to clipboard",
		"  k - Keep watching next episode",
		"  q - Close popup",
		"",
		"Subtitles:",
	}

	// Get user's preferred subtitle language from config
	preferredLang := "en" // Default to English
	cfg, ok := a.cfg.(*config.Config)
	if ok && cfg.Player.SubtitleLang != "" {
		preferredLang = cfg.Player.SubtitleLang
	}

	// Filter subtitles to only show the preferred language
	filteredSubtitles := []providers.Subtitle{}
	for _, sub := range a.watchPartyInfo.Subtitles {
		if strings.Contains(strings.ToLower(sub.Language), strings.ToLower(preferredLang)) ||
			strings.Contains(strings.ToLower(sub.Language), "english") {
			filteredSubtitles = append(filteredSubtitles, sub)
		}
	}

	if len(filteredSubtitles) == 0 {
		content = append(content, "  No subtitles available in preferred language")
	} else {
		for i, sub := range filteredSubtitles {
			content = append(content, fmt.Sprintf("  %d. %s: %s", i+1, sub.Language, sub.URL))
		}
		content = append(content, "  1-9 - Copy specific subtitle URL", "  s - Copy all subtitles to clipboard")
	}

	if a.watchPartyInfo.NextEpisodeTitle != "" {
		content = append(content, "", fmt.Sprintf("Next: %s (Ep %d)", a.watchPartyInfo.NextEpisodeTitle, a.watchPartyInfo.NextEpisodeNumber))
	}

	// Join all content
	contentStr := strings.Join(content, "\n")

	// Style for the popup
	popupStyle := styles.PopupStyle.BorderForeground(styles.OxocarbonBlue)

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonBlue).
		Bold(true).
		MarginBottom(1)

	// Create the popup with title and content
	popupContent := titleStyle.Render("WatchParty Information")
	popupContent += "\n" + popupStyle.Render(contentStr)

	// For now, just return the popup text - the View function will handle positioning
	return popupContent
}

// Message handlers for WatchParty messages

func (a *App) handleWatchPartyMsg(msg common.WatchPartyMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		a.statusMsg = fmt.Sprintf("✗ Failed to generate WatchParty URL: %v", msg.Err)
		a.statusMsgTime = time.Now()
		return a, func() tea.Msg {
			time.Sleep(5 * time.Second)
			return clearStatusMsg{}
		}
	}

	a.showWatchPartyPopup = true
	a.watchPartyInfo = msg.WatchPartyInfo
	return a, nil
}

func (a *App) handleSetWatchPartyProxyMsg(msg common.SetWatchPartyProxyMsg) (tea.Model, tea.Cmd) {
	a.setWatchPartyProxy(msg.ProxyURL, msg.Origin)
	a.statusMsg = "✓ WatchParty proxy updated"
	a.statusMsgTime = time.Now()
	return a, func() tea.Msg {
		time.Sleep(2 * time.Second)
		return clearStatusMsg{}
	}
}

func (a *App) handleOpenWatchPartyMsg(msg common.OpenWatchPartyMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		errorMsg := fmt.Sprintf("✗ Failed to open browser. WatchParty URL: %s", msg.URL)
		a.err = fmt.Errorf("failed to open WatchParty URL in browser: %w", msg.Err)
		a.statusMsg = errorMsg
		a.statusMsgTime = time.Now()
		return a, func() tea.Msg {
			time.Sleep(3 * time.Second)
			return clearStatusMsg{}
		}
	}

	a.statusMsg = "✓ WatchParty room opened in your browser!"
	a.statusMsgTime = time.Now()
	return a, func() tea.Msg {
		time.Sleep(2 * time.Second)
		return clearStatusMsg{}
	}
}

func (a *App) handleGenerateWatchPartyMsg(msg common.GenerateWatchPartyMsg) (tea.Model, tea.Cmd) {
	return a, a.generateWatchPartyURL(msg.EpisodeID, msg.Number, msg.Title)
}

func (a *App) handleShareViaWatchPartyMsg(msg common.ShareViaWatchPartyMsg) (tea.Model, tea.Cmd) {
	if a.currentEpisodeID != "" && a.currentEpisodeNumber > 0 {
		return a, a.generateWatchPartyURL(a.currentEpisodeID, a.currentEpisodeNumber, a.currentEpisodeTitle)
	} else if len(a.episodes) > 0 {
		episode := a.episodes[0]
		return a, a.generateWatchPartyURL(episode.ID, episode.Number, episode.Title)
	}

	a.err = fmt.Errorf("no episode selected to share")
	a.state = errorView
	return a, nil
}

func (a *App) handleShareHistoryViaWatchPartyMsg(msg common.ShareHistoryViaWatchPartyMsg) (tea.Model, tea.Cmd) {
	var provider providers.Provider
	for _, p := range a.providers {
		if p.Name() == msg.ProviderName {
			provider = p
			break
		}
	}

	if provider == nil {
		// If provider not found by name, try to use appropriate media type
		providerMap := a.providers
		for _, p := range providerMap {
			provider = p
			break
		}
	}

	if provider == nil {
		a.err = fmt.Errorf("no provider available to get stream URL for history item")
		a.state = errorView
		return a, nil
	}

	// For history items, we need to get the episode ID from the provider
	if msg.Episode == 0 {
		type movieEpisodeIDGetter interface {
			GetMovieEpisodeID(ctx context.Context, mediaID string) (string, error)
		}

		episodeIDGetter, ok := provider.(movieEpisodeIDGetter)
		if !ok {
			a.err = fmt.Errorf("provider does not support direct movie playback")
			a.state = errorView
			return a, nil
		}

		episodeID, err := episodeIDGetter.GetMovieEpisodeID(context.Background(), msg.MediaID)
		if err != nil {
			a.err = fmt.Errorf("failed to get movie episode ID: %w", err)
			a.state = errorView
			return a, nil
		}

		return a, a.generateWatchPartyURL(episodeID, msg.Episode, msg.MediaTitle)
	} else {
		a.err = fmt.Errorf("watch party from history for TV/anime episodes is not yet supported")
		a.state = errorView
		return a, nil
	}
}

func (a *App) handleShareRecentViaWatchPartyMsg(msg common.ShareRecentViaWatchPartyMsg) (tea.Model, tea.Cmd) {
	var provider providers.Provider
	for _, p := range a.providers {
		if p.Name() == msg.ProviderName {
			provider = p
			break
		}
	}

	if provider == nil {
		// If provider by name not found, try to find appropriate one
		providerMap := a.providers
		for _, p := range providerMap {
			provider = p
			break
		}
	}

	if provider == nil {
		a.err = fmt.Errorf("no provider available to get stream URL for recent item")
		a.state = errorView
		return a, nil
	}

	// For recent items, we try to get the episode ID from the provider
	// For movies (episode 0), we can try directly
	if msg.Episode == 0 { // Likely a movie
		// This is a temporary solution to call the provider-specific method.
		type movieEpisodeIDGetter interface {
			GetMovieEpisodeID(ctx context.Context, mediaID string) (string, error)
		}

		episodeIDGetter, ok := provider.(movieEpisodeIDGetter)
		if !ok {
			a.err = fmt.Errorf("provider does not support direct movie playback")
			a.state = errorView
			return a, nil
		}

		episodeID, err := episodeIDGetter.GetMovieEpisodeID(context.Background(), msg.MediaID)
		if err != nil {
			a.err = fmt.Errorf("failed to get movie episode ID: %w", err)
			a.state = errorView
			return a, nil
		}

		return a, a.generateWatchPartyURL(episodeID, msg.Episode, msg.MediaTitle)
	} else {
		// For TV/anime episodes from recent items, we might need to get seasons/episodes
		// For now, we'll just return an error since it's complex to implement
		// a proper lookup from recent item to episode ID
		a.err = fmt.Errorf("watch party from recent items for TV/anime episodes is not yet supported")
		a.state = errorView
		return a, nil
	}
}

func (a *App) handleShareMediaViaWatchPartyMsg(msg common.ShareMediaViaWatchPartyMsg) (tea.Model, tea.Cmd) {
	var provider providers.Provider
	mediaType := providers.MediaType(msg.Type)

	var ok bool
	provider, ok = a.providers[mediaType]

	// If the provider was found but the media ID format suggests a different provider type,
	// try to select the more appropriate provider for the actual content
	if ok && provider != nil {
		// Check if the media ID format suggests a specific provider type
		// For example, "movie/..." IDs should use movie providers, not anime providers
		if strings.HasPrefix(msg.MediaID, "movie/") && mediaType == providers.MediaTypeAnime {
			// This looks like a movie that was found by an anime provider - try to use a movie provider instead
			// This could happen when searching with default "anime" type but finding a movie
			for _, p := range a.providers {
				if p.Type() == providers.MediaTypeMovie || p.Type() == providers.MediaTypeMovieTV {
					provider = p
					break
				}
			}
		} else if strings.HasPrefix(msg.MediaID, "tv/") && mediaType == providers.MediaTypeAnime {
			// Similar case for TV shows
			for _, p := range a.providers {
				if p.Type() == providers.MediaTypeTV || p.Type() == providers.MediaTypeMovieTV {
					provider = p
					break
				}
			}
		}
	}

	// If no provider was found based on the original type, try to find one based on the ID format
	if !ok || provider == nil {
		// Try to select provider based on media ID format
		if strings.HasPrefix(msg.MediaID, "movie/") {
			// Look for movie provider
			for _, p := range a.providers {
				if p.Type() == providers.MediaTypeMovie || p.Type() == providers.MediaTypeMovieTV {
					provider = p
					break
				}
			}
		} else if strings.HasPrefix(msg.MediaID, "tv/") {
			// Look for TV provider
			for _, p := range a.providers {
				if p.Type() == providers.MediaTypeTV || p.Type() == providers.MediaTypeMovieTV {
					provider = p
					break
				}
			}
		} else {
			// Default to finding any available provider
			for _, p := range a.providers {
				provider = p
				break
			}
		}
	}

	if provider == nil {
		a.err = fmt.Errorf("no provider available to get stream URL for media: %s", msg.Title)
		a.state = errorView
		return a, nil
	}

	// For all media types, get seasons first to determine structure
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seasons, err := provider.GetSeasons(ctx, msg.MediaID)
	if err != nil {
		// If GetSeasons fails, this might be a movie that needs the direct method
		// (Some providers handle movies as direct, others via seasons/episodes)
		type movieEpisodeIDGetter interface {
			GetMovieEpisodeID(ctx context.Context, mediaID string) (string, error)
		}

		episodeIDGetter, ok := provider.(movieEpisodeIDGetter)
		if !ok {
			// Provider doesn't support direct movie playback, which is needed in this case
			// Since we couldn't get seasons and provider doesn't support direct method,
			// we need to get media details to see if it has seasons/episodes info
			mediaDetails, err := provider.GetMediaDetails(ctx, msg.MediaID)
			if err != nil || mediaDetails == nil {
				a.err = fmt.Errorf("provider does not support direct movie playback and seasons could not be retrieved")
				a.state = errorView
				return a, nil
			}

			// If media details have seasons, try to get episodes that way
			if len(mediaDetails.Seasons) > 0 {
				// Use the first season to get episodes
				var episodes []providers.Episode
				for _, seasonInfo := range mediaDetails.Seasons {
					episodes, err = provider.GetEpisodes(ctx, seasonInfo.ID)
					if err == nil && len(episodes) > 0 {
						// Found episodes via media details, use the first one
						episode := episodes[0]                                                            // For movies, typically just one episode
						return a, a.generateWatchPartyURLWithProvider(provider, episode.ID, 0, msg.Title) // Episode 0 for movies
					}
				}
			}

			// If we still couldn't get episodes, try the direct movie method again
			// or report an error
			a.err = fmt.Errorf("could not determine how to get movie episode from provider")
			a.state = errorView
			return a, nil
		}

		episodeID, err := episodeIDGetter.GetMovieEpisodeID(context.Background(), msg.MediaID)
		if err != nil {
			a.err = fmt.Errorf("failed to get movie episode ID: %w", err)
			a.state = errorView
			return a, nil
		}

		return a, a.generateWatchPartyURLWithProvider(provider, episodeID, 0, msg.Title) // Episode 0 for movies
	}

	// If we got seasons, try to get episodes
	if len(seasons) > 0 {
		// For movies, typically the first season (or only season) should have the movie episode
		firstSeason := seasons[0]
		episodes, err := provider.GetEpisodes(ctx, firstSeason.ID)
		if err != nil || len(episodes) == 0 {
			// If no episodes found via seasons, check if this might be a movie
			mediaDetails, err := provider.GetMediaDetails(ctx, msg.MediaID)
			if err != nil || mediaDetails == nil {
				a.err = fmt.Errorf("provider implementation doesn't support getting episodes from seasons and failed to get media details: %w", err)
				a.state = errorView
				return a, nil
			}

			// Check if this is a movie by looking at metadata
			isMovie := msg.Type == string(providers.MediaTypeMovie) ||
				mediaDetails.TotalEpisodes == 1 ||
				(len(mediaDetails.Seasons) == 0 && mediaDetails.Status == "Completed" && mediaDetails.Type == providers.MediaTypeAnime)

			if isMovie {
				// Try the direct movie method as a fallback
				type movieEpisodeIDGetter interface {
					GetMovieEpisodeID(ctx context.Context, mediaID string) (string, error)
				}

				episodeIDGetter, ok := provider.(movieEpisodeIDGetter)
				if !ok {
					// Provider doesn't implement direct movie interface
					a.err = fmt.Errorf("provider implementation doesn't support movie playback directly and no episodes found in seasons")
					a.state = errorView
					return a, nil
				}

				episodeID, err := episodeIDGetter.GetMovieEpisodeID(context.Background(), msg.MediaID)
				if err != nil {
					a.err = fmt.Errorf("failed to get movie episode ID: %w", err)
					a.state = errorView
					return a, nil
				}

				return a, a.generateWatchPartyURLWithProvider(provider, episodeID, 0, msg.Title) // Episode 0 for movies
			} else {
				// For non-movies, report that episodes weren't found
				a.err = fmt.Errorf("no episodes found for media - seasons exist but no episodes available")
				a.state = errorView
				return a, nil
			}
		}

		// If we found episodes, use the first one (for movies there's usually just one)
		if len(episodes) > 0 {
			episode := episodes[0]                                                            // For movies, typically just one episode
			return a, a.generateWatchPartyURLWithProvider(provider, episode.ID, 0, msg.Title) // Episode 0 for movies
		}
	} else {
		// No seasons found - this could be a movie or an anime movie
		// Check if the media is likely a movie by getting its details and checking structure
		mediaDetails, err := provider.GetMediaDetails(ctx, msg.MediaID)
		if err != nil || mediaDetails == nil {
			a.err = fmt.Errorf("failed to get media details to determine if it's a movie: %w", err)
			a.state = errorView
			return a, nil
		}

		// Check if this is a movie by looking at metadata
		// Movies typically have TotalEpisodes == 1 or no seasons but have episodes
		isMovie := msg.Type == string(providers.MediaTypeMovie) ||
			mediaDetails.TotalEpisodes == 1 ||
			(len(mediaDetails.Seasons) == 0 && mediaDetails.Status == "Completed" && mediaDetails.Type == providers.MediaTypeAnime)

		if isMovie {
			// Try the direct movie method
			type movieEpisodeIDGetter interface {
				GetMovieEpisodeID(ctx context.Context, mediaID string) (string, error)
			}

			episodeIDGetter, ok := provider.(movieEpisodeIDGetter)
			if !ok {
				// If provider doesn't implement direct movie interface, try to get from media details
				// Some providers return episodes directly even without seasons
				seasonsCheck, err := provider.GetSeasons(ctx, msg.MediaID)
				if err == nil && len(seasonsCheck) > 0 {
					// Try getting episodes for the first season
					firstSeason := seasonsCheck[0]
					episodes, err := provider.GetEpisodes(ctx, firstSeason.ID)
					if err == nil && len(episodes) > 0 {
						episode := episodes[0]                                                            // For movies, typically just one episode
						return a, a.generateWatchPartyURLWithProvider(provider, episode.ID, 0, msg.Title) // Episode 0 for movies
					}
				}

				// For API-based providers, we need to try alternative approaches if seasons didn't work
				// Try to get episodes directly through the media details if possible
				// This handles cases where the provider might provide movie content differently
				// For example, anime movies might be structured as single episodes
				if len(mediaDetails.Seasons) == 0 && mediaDetails.TotalEpisodes > 0 {
					// This could be a provider that doesn't use seasons for single episodes
					// We'll try to work with the provider implementation directly
					a.err = fmt.Errorf("provider doesn't implement direct movie interface and no seasons found")
					a.state = errorView
					return a, nil
				}

				// If still no luck, we have to admit the provider doesn't support this
				a.err = fmt.Errorf("provider doesn't implement required interfaces for movie playback")
				a.state = errorView
				return a, nil
			}

			episodeID, err := episodeIDGetter.GetMovieEpisodeID(context.Background(), msg.MediaID)
			if err != nil {
				a.err = fmt.Errorf("failed to get movie episode ID: %w", err)
				a.state = errorView
				return a, nil
			}

			return a, a.generateWatchPartyURLWithProvider(provider, episodeID, 0, msg.Title) // Episode 0 for movies
		} else {
			// For non-movies with no seasons, this might be an error
			a.err = fmt.Errorf("no seasons found for non-movie media - structure not recognized. Media type: %s, TotalEpisodes: %d", mediaDetails.Type, mediaDetails.TotalEpisodes)
			a.state = errorView
			return a, nil
		}
	}

	// Fallback if no path succeeded
	a.err = fmt.Errorf("failed to generate WatchParty URL - no valid episodes found")
	a.state = errorView
	return a, nil
}
