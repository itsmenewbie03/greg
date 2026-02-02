package tui

// This file contains keyboard handling methods extracted from model.go
// for better code organization. All methods remain on the App struct.

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/components/anilist"
	"github.com/justchokingaround/greg/internal/tui/components/downloads"
	"github.com/justchokingaround/greg/internal/tui/components/episodes"
	"github.com/justchokingaround/greg/internal/tui/components/history"
	"github.com/justchokingaround/greg/internal/tui/components/home"
	"github.com/justchokingaround/greg/internal/tui/components/manga"
	"github.com/justchokingaround/greg/internal/tui/components/results"
	"github.com/justchokingaround/greg/internal/tui/components/search"
	"github.com/justchokingaround/greg/internal/tui/components/seasons"
)

// handleKeyMsg processes all keyboard input and routes to appropriate handlers
func (a *App) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle '?' to toggle help (global keybinding, works in all views)
	if msg.String() == "?" {
		// If help is visible and user presses ?, hide it
		// Otherwise, update context based on current state and show it
		if a.helpComponent.IsVisible() {
			a.helpComponent.Hide()
		} else {
			// Update help context based on current state
			a.updateHelpContext()
			a.helpComponent.Show()
		}
		return a, nil
	}

	// If help is visible, ONLY allow help navigation keys - block everything else
	if a.helpComponent.IsVisible() {
		// Handle Ctrl+C - quit immediately even if help is visible
		if msg.Type == tea.KeyCtrlC {
			if a.player != nil {
				_ = a.player.Stop(context.Background())
			}
			return a, tea.Quit
		}

		// Update help component with the key message (for scrolling)
		var helpCmd tea.Cmd
		a.helpComponent, helpCmd = a.helpComponent.Update(msg)

		// Only process esc to close, all other keys are handled by help component
		if msg.String() == "esc" {
			a.helpComponent.Hide()
			return a, helpCmd
		}

		// Block all other keys - only help component handles navigation
		return a, helpCmd
	}

	// Handle download notification dismissal
	if a.showDownloadNotification {
		// 'd' key goes to downloads instead of dismissing
		if msg.String() == "d" {
			a.showDownloadNotification = false
			a.downloadNotificationMsg = ""
			return a, func() tea.Msg {
				return common.GoToDownloadsMsg{}
			}
		}
		// Any other key dismisses the notification
		a.showDownloadNotification = false
		a.downloadNotificationMsg = ""
		return a, nil
	}

	// Handle 'd' to go to downloads view (global keybinding)
	if msg.String() == "d" && a.state == homeView {
		return a, func() tea.Msg {
			return common.GoToDownloadsMsg{}
		}
	}

	// Handle WatchParty popup keys first if popup is visible
	if a.showWatchPartyPopup {
		return a.handleWatchPartyPopupInput(msg)
	}

	// Handle Debug popup keys first if popup is visible
	if a.showDebugPopup {
		return a.handleDebugPopupInput(msg)
	}

	// Handle dialog input first if a dialog is open
	if a.dialogMode != anilist.DialogNone {
		return a.handleDialogInput(msg)
	}

	// Handle playback completion view (special case - needs early handling)
	if a.state == playbackCompletedView {
		return a.handlePlaybackCompletedKeys(msg)
	}

	// Allow cancellation during player launch (special case - needs early handling)
	if a.state == launchingPlayerView {
		return a.handleLaunchingPlayerKeys(msg)
	}

	// Block all navigation when playing (except quit) (special case - needs early handling)
	if a.state == playingView {
		return a.handlePlayingViewKeys(msg)
	}

	// Handle manga reader view (special case - component handles everything)
	if a.state == mangaReaderView {
		// Delegate to manga component
		var cmd tea.Cmd
		mangaModel, cmd := a.mangaComponent.Update(msg)
		a.mangaComponent = mangaModel.(manga.Model)
		return a, cmd
	}

	// Clear quit request if user presses any key other than 'q' or 'esc'
	if a.quitRequested && msg.String() != "q" && msg.String() != "esc" {
		a.quitRequested = false
		a.statusMsg = "" // Clear the warning message
	}

	// Route to specific key handlers (which delegates to components first)
	return a.handleSpecificKeys(msg)
}

// handleEscapeKey handles the escape key across different views
func (a *App) handleEscapeKey() (tea.Model, tea.Cmd) {
	// Handle error view
	if a.state == errorView {
		a.err = nil
		a.anilistSearchRetried = false
		if a.watchingFromAniList {
			a.state = anilistView
		} else {
			a.state = searchView
		}
		return a, nil
	}

	// Handle provider selection view
	if a.state == providerSelectionView {
		// Return to previous state
		if a.previousState != -1 {
			a.state = a.previousState
			a.previousState = -1
		} else {
			a.state = homeView
		}
		return a, nil
	}

	// Handle navigation for views that need app-level ESC handling
	// Views with complex internal state (search, results, seasons, episodes, anilist, manga) handle esc internally
	// Simple views (home, downloads, history, providerStatus) are handled here
	switch a.state {
	case homeView:
		// Check for active downloads before quitting
		if a.downloadMgr != nil && a.downloadMgr.HasActiveDownloads() {
			if !a.quitRequested {
				a.quitRequested = true
				a.statusMsg = "⚠ Downloads in progress! Press 'esc' again to force quit, 'd' to view downloads, or any other key to cancel"
				a.statusMsgTime = time.Now()
				return a, func() tea.Msg {
					time.Sleep(3 * time.Second)
					return clearStatusMsg{}
				}
			}
		}
		if a.player != nil {
			_ = a.player.Stop(context.Background())
		}
		return a, tea.Quit
	case downloadsView:
		a.state = homeView
		return a, func() tea.Msg {
			return common.RefreshHistoryMsg{}
		}
	case historyView:
		a.state = homeView
		return a, func() tea.Msg {
			return common.RefreshHistoryMsg{}
		}
	case providerStatusView:
		a.state = homeView
		return a, func() tea.Msg {
			return common.RefreshHistoryMsg{}
		}
	}

	return a, nil
}

// handlePlaybackCompletedKeys handles keyboard input in playback completed view
func (a *App) handlePlaybackCompletedKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	// If watching from AniList, episode was completed (>= 85%), and not last episode, handle continue watching prompt
	if a.watchingFromAniList && !a.isLastEpisode && a.episodeCompleted {
		switch msg.String() {
		case "y", "Y", "enter":
			// User wants to continue - play next episode
			a.playbackCompletionMsg = ""
			// Don't reset watchingFromAniList - keep AniList context

			// Find and play the next episode (current + 1)
			targetEpisode := a.currentEpisodeNumber + 1
			if targetEpisode > a.currentAniListMedia.TotalEpisodes {
				// No more episodes
				a.watchingFromAniList = false
				a.previousState = -1
				a.currentEpisodeID = ""
				a.currentEpisodeNumber = 0
				a.currentSeasonNumber = 0
				a.currentEpisodeTitle = ""
				a.episodeCompleted = false
				a.state = loadingView
				a.loadingOp = loadingAniListLibrary
				cmds = append(cmds, a.spinner.Tick, a.fetchAniListLibrary())
				return a, tea.Batch(cmds...)
			}

			// Find the episode in our episodes list
			var episodeToPlay *providers.Episode
			for i := range a.episodes {
				if a.episodes[i].Number == targetEpisode {
					episodeToPlay = &a.episodes[i]
					break
				}
			}

			if episodeToPlay != nil {
				// Clear old episode state before playing new one
				a.currentEpisodeID = ""
				a.currentEpisodeNumber = 0
				a.currentSeasonNumber = 0
				a.currentEpisodeTitle = ""
				a.episodeCompleted = false

				// Play the episode directly
				a.previousState = anilistView
				return a, func() tea.Msg {
					return common.EpisodeSelectedMsg{
						EpisodeID: episodeToPlay.ID,
						Number:    episodeToPlay.Number,
						Title:     episodeToPlay.Title,
					}
				}
			}

			// If we couldn't find the episode, fall through to return to library
			fallthrough

		case "n", "N", "esc":
			// User wants to stop - return to library
			a.playbackCompletionMsg = ""
			a.watchingFromAniList = false
			a.previousState = -1

			// Clear episode state
			a.currentEpisodeID = ""
			a.currentEpisodeNumber = 0
			a.currentSeasonNumber = 0
			a.currentEpisodeTitle = ""
			a.episodeCompleted = false

			a.state = loadingView
			a.loadingOp = loadingAniListLibrary
			cmds = append(cmds, a.spinner.Tick, a.fetchAniListLibrary())
			return a, tea.Batch(cmds...)

		default:
			// Ignore other keys
			return a, nil
		}
	}

	// Handle manual completion dialog (when user pressed 'q' during playback)
	// This is shown when showCompletionDialog is true but not watching from AniList
	if a.showCompletionDialog && !a.watchingFromAniList {
		switch msg.String() {
		case "y", "Y", "enter":
			// User says they completed watching - save and return to episode list
			a.playbackCompletionMsg = ""
			a.showCompletionDialog = false

			// Save progress as 100% completed
			if a.lastProgress != nil {
				a.lastProgress.CurrentTime = a.lastProgress.Duration
				a.lastProgress.Percentage = 100.0
				a.syncProgressOnEnd(a.lastProgress)
			}

			// Set next episode cursor BEFORE clearing state
			nextEpisode := a.currentEpisodeNumber + 1
			a.lastPlayedEpisodeNumber = a.currentEpisodeNumber

			// Clear episode state
			a.currentEpisodeID = ""
			a.currentSeasonNumber = 0
			a.currentEpisodeNumber = 0
			a.currentEpisodeTitle = ""
			a.lastProgress = nil
			a.episodeCompleted = true

			// Return to episode view and set cursor
			a.state = episodeView
			a.episodesComponent.SetMediaType(a.selectedMedia.Type)
			a.episodesComponent.SetEpisodes(a.episodes)
			a.episodesComponent.SetCursorToEpisode(nextEpisode)
			return a, nil

		case "n", "N", "esc":
			// User says they didn't complete - save and go to home
			a.playbackCompletionMsg = ""
			a.showCompletionDialog = false

			// Save progress as-is
			if a.lastProgress != nil {
				a.syncProgressOnEnd(a.lastProgress)
			}

			// Clear episode state
			a.currentEpisodeID = ""
			a.currentSeasonNumber = 0
			a.currentEpisodeNumber = 0
			a.currentEpisodeTitle = ""
			a.lastProgress = nil
			a.lastPlayedEpisodeNumber = 0
			a.episodeCompleted = false

			a.state = homeView
			return a, func() tea.Msg {
				return common.RefreshHistoryMsg{}
			}

		default:
			// Ignore other keys in the completion dialog
			return a, nil
		}
	}

	// Default behavior for non-AniList or last episode (any key returns)
	a.playbackCompletionMsg = ""
	a.showCompletionDialog = false
	returnState := a.previousState
	a.previousState = -1

	// Store episodeCompleted before clearing it
	episodeWasCompleted := a.episodeCompleted

	// Clear episode state
	a.currentEpisodeID = ""
	a.currentSeasonNumber = 0
	a.currentEpisodeNumber = 0
	a.currentEpisodeTitle = ""
	a.episodeCompleted = false

	// Clear any loading operation that might be lingering
	a.loadingOp = 0

	// If we were watching from AniList, always refresh the library to show updated progress
	// (regardless of what screen we came from - could be anilistView, episodeView, etc.)
	if a.watchingFromAniList {
		a.watchingFromAniList = false
		a.state = loadingView
		a.loadingOp = loadingAniListLibrary
		cmds = append(cmds, a.spinner.Tick, a.fetchAniListLibrary())
		return a, tea.Batch(cmds...)
	}

	// For non-AniList playback, return to previous state (or home if not set)
	// Make sure we go to a valid view state, not a loading state
	if returnState != 0 && returnState != loadingView && returnState != launchingPlayerView {
		a.state = returnState
		// If returning to episode view, update the episodes component
		if a.state == episodeView && len(a.episodes) > 0 {
			a.episodesComponent.SetMediaType(a.selectedMedia.Type)
			a.episodesComponent.SetEpisodes(a.episodes)
			if a.lastPlayedEpisodeNumber > 0 {
				targetEpisode := a.lastPlayedEpisodeNumber
				if episodeWasCompleted {
					targetEpisode = a.lastPlayedEpisodeNumber + 1
				}
				a.episodesComponent.SetCursorToEpisode(targetEpisode)
				a.lastPlayedEpisodeNumber = 0
			}
		}
	} else if len(a.episodes) > 0 {
		a.state = episodeView
		a.episodesComponent.SetMediaType(a.selectedMedia.Type)
		a.episodesComponent.SetEpisodes(a.episodes)
		if a.lastPlayedEpisodeNumber > 0 {
			targetEpisode := a.lastPlayedEpisodeNumber
			if episodeWasCompleted {
				targetEpisode = a.lastPlayedEpisodeNumber + 1
			}
			a.episodesComponent.SetCursorToEpisode(targetEpisode)
			a.lastPlayedEpisodeNumber = 0
		}
	} else {
		a.state = homeView
		// Refresh history when returning to home after manual keypress
		cmds = append(cmds, func() tea.Msg {
			return common.RefreshHistoryMsg{}
		})
	}
	return a, tea.Batch(cmds...)
}

// handleLaunchingPlayerKeys handles keyboard input during player launch
func (a *App) handleLaunchingPlayerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle Ctrl+C using msg.Type for reliable detection
	if msg.Type == tea.KeyCtrlC {
		if a.player != nil {
			_ = a.player.Stop(context.Background())
		}
		return a, tea.Quit
	}

	switch msg.String() {
	case "q":
		// Cleanup player before quitting
		if a.player != nil {
			_ = a.player.Stop(context.Background())
		}
		return a, tea.Quit
	case "esc":
		// Cancel launch and return to episode list
		if a.player != nil {
			_ = a.player.Stop(context.Background())
		}
		a.state = a.previousState
		if a.state == sessionState(0) {
			a.state = episodeView
		}
		return a, nil
	default:
		// Ignore all other keys during launch
		return a, nil
	}
}

// handlePlayingViewKeys handles keyboard input during video playback
func (a *App) handlePlayingViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle Ctrl+C - quit the TUI application immediately
	if msg.Type == tea.KeyCtrlC {
		if a.player != nil {
			_ = a.player.Stop(context.Background())
		}
		return a, tea.Quit
	}

	switch msg.String() {
	case "q":
		// Stop playback and show completion dialog
		if a.player != nil {
			_ = a.player.Stop(context.Background())
		}
		// Transition to completion dialog view
		a.state = playbackCompletedView
		a.showCompletionDialog = true
		a.completionDialogMsg = "Did you finish watching this video?\n\n[y] Yes - Episode completed\n[n] No - Not completed"
		a.episodeCompleted = false // User-initiated quit, default to not completed
		return a, nil
	default:
		// Ignore all other keys during playback
		return a, nil
	}
}

// handleSpecificKeys handles individual key bindings (ctrl+c, q, w, p, 1, 2, 3, m, esc, etc.)
func (a *App) handleSpecificKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Block navigation keys if any component has active input mode
	// This ensures j/k/up/down keys type characters instead of triggering navigation
	if a.state == resultsView && a.results.IsInputActive() {
		var cmd tea.Cmd
		var resultsModel tea.Model
		resultsModel, cmd = a.results.Update(msg)
		a.results = resultsModel.(results.Model)
		return a, cmd
	}
	if a.state == episodeView && a.episodesComponent.IsInputActive() {
		var cmd tea.Cmd
		var episodesModel tea.Model
		episodesModel, cmd = a.episodesComponent.Update(msg)
		a.episodesComponent = episodesModel.(episodes.Model)
		return a, cmd
	}
	if a.state == seasonView && a.seasons.IsInputActive() {
		var cmd tea.Cmd
		var seasonsModel tea.Model
		seasonsModel, cmd = a.seasons.Update(msg)
		a.seasons = seasonsModel.(seasons.Model)
		return a, cmd
	}
	if a.state == historyView && a.historyComponent.IsInputActive() {
		var cmd tea.Cmd
		var historyModel tea.Model
		historyModel, cmd = a.historyComponent.Update(msg)
		a.historyComponent = historyModel.(history.Model)
		return a, cmd
	}

	// Handle view-specific keys that should NOT be handled at app level
	// These keys get delegated to components for their internal handling
	switch a.state {
	case homeView:
		// Home view needs to handle recent items navigation (up/down/enter)
		// AND app-level shortcuts (s/l/h/d/tab/P/1/2/3)
		// Pass to component first, if it returns nil, fall through to app-level handlers
		var cmd tea.Cmd
		var homeModel tea.Model
		homeModel, cmd = a.home.Update(msg)
		a.home = homeModel.(*home.Model)
		// If component handled it (returned non-nil cmd), use that
		if cmd != nil {
			return a, cmd
		}
		// Otherwise fall through to app-level handlers below
	case searchView:
		// Search view handles all keys internally (text input + enter/esc)
		var cmd tea.Cmd
		var searchModel tea.Model
		searchModel, cmd = a.search.Update(msg)
		a.search = searchModel.(search.Model)
		return a, cmd
	case resultsView:
		// Results view handles navigation internally
		var cmd tea.Cmd
		var resultsModel tea.Model
		resultsModel, cmd = a.results.Update(msg)
		a.results = resultsModel.(results.Model)
		return a, cmd
	case seasonView:
		// Seasons view handles navigation internally
		var cmd tea.Cmd
		var seasonsModel tea.Model
		seasonsModel, cmd = a.seasons.Update(msg)
		a.seasons = seasonsModel.(seasons.Model)
		return a, cmd
	case episodeView:
		// Episodes view handles navigation internally
		var cmd tea.Cmd
		var episodesModel tea.Model
		episodesModel, cmd = a.episodesComponent.Update(msg)
		a.episodesComponent = episodesModel.(episodes.Model)
		return a, cmd
	case anilistView:
		// AniList view handles navigation + dialogs internally
		var cmd tea.Cmd
		var anilistModel tea.Model
		anilistModel, cmd = a.anilistComponent.Update(msg)
		a.anilistComponent = anilistModel.(anilist.Model)
		return a, cmd
	case downloadsView:
		// Downloads view handles navigation internally
		var cmd tea.Cmd
		var downloadsModel tea.Model
		downloadsModel, cmd = a.downloadsComponent.Update(msg)
		a.downloadsComponent = downloadsModel.(downloads.Model)
		return a, cmd
	case historyView:
		// History view handles navigation internally
		var cmd tea.Cmd
		var historyModel tea.Model
		historyModel, cmd = a.historyComponent.Update(msg)
		a.historyComponent = historyModel.(history.Model)
		return a, cmd
	case providerSelectionView:
		isAniListSelection := a.watchingFromAniList && len(a.providerSearchResults) > 0

		if !isAniListSelection {
			switch msg.String() {
			case "enter":
				if selected := a.providerSelectionResult.GetSelectedMedia(); selected != nil {
					a.providerName = selected.ID
					return a.switchProvider(false)
				}
			case "ctrl+s":
				if selected := a.providerSelectionResult.GetSelectedMedia(); selected != nil {
					a.providerName = selected.ID
					return a.switchProvider(true)
				}
			}
		}

		var cmd tea.Cmd
		var providerSelectionModel tea.Model
		providerSelectionModel, cmd = a.providerSelectionResult.Update(msg)
		a.providerSelectionResult = providerSelectionModel.(results.Model)
		return a, cmd
	}

	// App-level key handlers for homeView and other states
	switch msg.String() {
	case "ctrl+c":
		// Cleanup player before quitting
		if a.player != nil {
			_ = a.player.Stop(context.Background())
		}
		return a, tea.Quit
	case "s":
		// Go to search from home view - NOT in searchView (user needs to type freely)
		if a.state == homeView {
			return a, func() tea.Msg {
				return common.GoToSearchMsg{}
			}
		}
	case "l":
		// Go to AniList view from home (only for Anime/Manga)
		if a.state == homeView && a.currentMediaType != providers.MediaTypeMovieTV {
			return a, func() tea.Msg {
				return common.GoToAniListMsg{}
			}
		}
	case "h":
		// Go to History view from home
		if a.state == homeView {
			return a, func() tea.Msg {
				return common.GoToHistoryMsg{MediaType: a.currentMediaType}
			}
		}
	case "d":
		// Go to Downloads view from home
		if a.state == homeView {
			return a, func() tea.Msg {
				return common.GoToDownloadsMsg{}
			}
		}
	case "tab":
		// Toggle provider from home view
		if a.state == homeView {
			return a, func() tea.Msg {
				return common.ToggleProviderMsg{}
			}
		}
	case "P":
		// Go to Provider Status view from home (capital P)
		if a.state == homeView {
			return a, func() tea.Msg {
				return common.GoToProviderStatusMsg{}
			}
		}
	case "ctrl+h":
		// Global keybind to return to home view from anywhere
		if a.state != homeView {
			a.statusMsg = ""
			a.state = homeView
			a.cameFromHistory = false
			a.quitRequested = false // Clear quit warning if any
			// Refresh history to show latest
			return a, func() tea.Msg {
				return common.RefreshHistoryMsg{}
			}
		}
		return a, nil
	case "q":
		// Only quit if not in a view that handles 'q' itself (views with text input or internal navigation)
		if a.state != anilistView && a.state != searchView && a.state != resultsView && a.state != seasonView && a.state != episodeView {
			// Check if there are active downloads
			if a.downloadMgr != nil && a.downloadMgr.HasActiveDownloads() {
				if !a.quitRequested {
					// First 'q' press - show warning
					a.quitRequested = true
					a.statusMsg = "⚠ Downloads in progress! Press 'q' again to force quit, 'alt+d' to view downloads, or any other key to cancel"
					a.statusMsgTime = time.Now()
					// Clear status message after 2 seconds
					return a, func() tea.Msg {
						time.Sleep(2 * time.Second)
						return clearStatusMsg{}
					}
				}
				// Second 'q' press - force quit
			}

			// Clear any pending status message before quitting
			a.quitRequested = false
			a.statusMsg = ""

			// Cleanup player before quitting
			if a.player != nil {
				_ = a.player.Stop(context.Background())
			}
			return a, tea.Quit
		}
	case "w":
		// Share via WatchParty in episode view
		if a.state == episodeView && len(a.episodes) > 0 {
			// Use currently selected episode from episodesComponent
			currentIndex := a.episodesComponent.GetCurrentIndex()
			if currentIndex >= 0 && currentIndex < len(a.episodes) {
				episode := a.episodes[currentIndex]
				return a, func() tea.Msg {
					return common.GenerateWatchPartyMsg{
						EpisodeID: episode.ID,
						Number:    episode.Number,
						Title:     episode.Title,
					}
				}
			}
		}
	case "p":
		// Switch provider (lowercase p) - NOT in searchView (user needs to type freely)
		// NOT in anilistView (conflicts with progress update keybind 'p')
		if a.state == homeView || a.state == resultsView || a.state == seasonView || a.state == episodeView {
			return a.handleProviderSwitch()
		}
	case "1":
		// Quick switch to Movies/TV mode - NOT in searchView (user needs to type freely)
		if a.state == homeView {
			return a.switchToMediaType(providers.MediaTypeMovieTV)
		}
	case "2":
		// Quick switch to Anime mode - NOT in searchView (user needs to type freely)
		if a.state == homeView {
			return a.switchToMediaType(providers.MediaTypeAnime)
		}
	case "3":
		// Quick switch to Manga mode - NOT in searchView (user needs to type freely)
		if a.state == homeView {
			return a.switchToMediaType(providers.MediaTypeManga)
		}
	case "m":
		// Scrape manga info if in season or episode view and it's an anime
		if (a.state == seasonView || a.state == episodeView) && a.selectedMedia.Type == providers.MediaTypeAnime {
			return a, func() tea.Msg {
				return common.MangaInfoMsg{
					AnimeTitle: a.selectedMedia.Title,
				}
			}
		}
		// Also allow in AniList view if an anime is selected
		if a.state == anilistView {
			selectedMedia := a.anilistComponent.GetSelectedMedia()
			if selectedMedia != nil && (selectedMedia.Type == "anime" || selectedMedia.Type == "ANIME") {
				return a, func() tea.Msg {
					return common.MangaInfoMsg{
						AnimeTitle: selectedMedia.Title,
					}
				}
			}
		}
	case "esc":
		// App-level ESC handling (only reached if component didn't handle it)
		return a.handleEscapeKey()
	}

	return a, nil
}

// handleProviderSwitch shows the provider selection menu
func (a *App) handleProviderSwitch() (tea.Model, tea.Cmd) {
	// Get available providers for current media type
	providersList := providers.GetByType(a.currentMediaType)
	if len(providersList) > 0 {
		// Create a list of "media" items representing providers
		var providerItems []providers.Media
		for _, p := range providersList {
			providerItems = append(providerItems, providers.Media{
				ID:    p.Name(),
				Title: p.Name(),
				Type:  a.currentMediaType,
			})
		}

		// Reuse results component
		a.providerSelectionResult = results.New()
		a.providerSelectionResult.SetMediaResults(providerItems)
		a.providerSelectionResult.SetShowMangaInfo(false)
		a.providerSelectionResult.SetIsProviderSelection(true) // Mark as provider selection

		// Clear any previous provider search results to avoid confusion
		a.providerSearchResults = nil

		a.previousState = a.state
		a.state = providerSelectionView
		return a, nil
	}

	return a, nil
}

// switchToMediaType switches the current media type and updates related state
func (a *App) switchToMediaType(mediaType providers.MediaType) (tea.Model, tea.Cmd) {
	if a.currentMediaType != mediaType {
		// Save current query
		if a.searchQueries == nil {
			a.searchQueries = make(map[providers.MediaType]string)
		}
		a.searchQueries[a.currentMediaType] = a.search.GetValue()

		a.currentMediaType = mediaType
		a.home.CurrentMediaType = a.currentMediaType

		// Update provider name
		if provider, ok := a.providers[a.currentMediaType]; ok {
			a.home.SetProvider(provider.Name())
			a.helpComponent.SetProviderName(provider.Name())
		}

		// Restore query for new media type
		if query, ok := a.searchQueries[a.currentMediaType]; ok {
			a.search.SetValue(query)
		} else {
			a.search.SetValue("")
		}

		// Reload recent history if on home view
		if a.state == homeView {
			return a, a.home.Init()
		}
	}

	return a, nil
}
