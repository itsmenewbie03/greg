package tui

// This file contains AniList-related methods extracted from model.go
// for better code organization. All methods remain on the App struct.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tracker"
	"github.com/justchokingaround/greg/internal/tracker/mapping"
	"github.com/justchokingaround/greg/internal/tui/components/anilist"
	"github.com/justchokingaround/greg/internal/tui/components/results"
)

// fetchAniListLibrary fetches the user's AniList library
func (a *App) fetchAniListLibrary() tea.Cmd {
	return func() tea.Msg {
		// Check if tracker manager is available
		if a.trackerMgr == nil {
			return anilist.LibraryLoadedMsg{
				Error: fmt.Errorf("tracker manager not initialized"),
			}
		}

		// Type assert to tracker.Manager
		mgr, ok := a.trackerMgr.(*tracker.Manager)
		if !ok {
			return anilist.LibraryLoadedMsg{
				Error: fmt.Errorf("failed to access tracker manager"),
			}
		}

		// Check if AniList is enabled and authenticated
		if !mgr.IsAniListEnabled() {
			return anilist.LibraryLoadedMsg{
				Error: fmt.Errorf("anilist is not enabled: run 'greg auth anilist' to configure"),
			}
		}

		if !mgr.IsAniListAuthenticated() {
			return anilist.LibraryLoadedMsg{
				Error: fmt.Errorf("anilist is not authenticated: run 'greg auth anilist' to authenticate"),
			}
		}

		// Fetch library from AniList
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Determine media type based on current app state
		mediaType := providers.MediaTypeAnime
		if a.currentMediaType == providers.MediaTypeManga {
			mediaType = providers.MediaTypeManga
		}

		library, err := mgr.GetUserLibrary(ctx, mediaType)
		if err != nil {
			return anilist.LibraryLoadedMsg{
				Error: fmt.Errorf("failed to fetch Anilist library: %w", err),
			}
		}

		return anilist.LibraryLoadedMsg{
			Library: library,
			Error:   nil,
		}
	}
}

// handleDialogInput handles keyboard input for AniList dialogs
func (a *App) handleDialogInput(msg tea.KeyMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg.String() {
	case "esc":
		// Cancel dialog
		a.dialogMode = anilist.DialogNone
		return a, nil

	case "enter":
		// Confirm and submit
		selectedMedia := a.currentAniListMedia
		if selectedMedia == nil {
			a.dialogMode = anilist.DialogNone
			return a, nil
		}

		switch a.dialogMode {
		case anilist.DialogStatus:
			// Get selected status
			statusOptions := []string{"CURRENT", "COMPLETED", "PAUSED", "DROPPED", "PLANNING", "REPEATING"}
			if a.dialogState.StatusIndex >= 0 && a.dialogState.StatusIndex < len(statusOptions) {
				newStatus := statusOptions[a.dialogState.StatusIndex]
				a.dialogMode = anilist.DialogNone
				cmds = append(cmds, a.updateAniListStatus(selectedMedia, newStatus))
				return a, tea.Batch(cmds...)
			}

		case anilist.DialogScore:
			// Validate and submit score
			score, err := anilist.ParseScore(a.dialogState.ScoreInput.Value())
			if err != nil {
				// Show error but keep dialog open
				a.err = fmt.Errorf("invalid score: %v", err)
				return a, nil
			}
			a.dialogMode = anilist.DialogNone
			cmds = append(cmds, a.updateAniListScore(selectedMedia, score))
			return a, tea.Batch(cmds...)

		case anilist.DialogProgress:
			// Validate and submit progress
			progress, err := anilist.ParseProgress(a.dialogState.ProgressInput.Value())
			if err != nil {
				// Show error but keep dialog open
				a.err = fmt.Errorf("invalid progress: %v", err)
				return a, nil
			}
			// Check if progress exceeds total episodes
			if selectedMedia.TotalEpisodes > 0 && progress > selectedMedia.TotalEpisodes {
				a.err = fmt.Errorf("progress cannot exceed %d episodes", selectedMedia.TotalEpisodes)
				return a, nil
			}
			a.dialogMode = anilist.DialogNone
			cmds = append(cmds, a.updateAniListProgress(selectedMedia, progress))
			return a, tea.Batch(cmds...)

		case anilist.DialogDelete:
			// For delete confirmation, enter should be treated as cancel (safe default)
			// Since the dialog asks for 'y' or 'n', Enter is not a valid confirmation option
			a.dialogMode = anilist.DialogNone
			return a, nil

		case anilist.DialogAddToList:
			// Get selected status - using same order as status dialog
			statusOptions := []string{"CURRENT", "COMPLETED", "PAUSED", "DROPPED", "PLANNING", "REPEATING"}
			if a.dialogState.StatusIndex >= 0 && a.dialogState.StatusIndex < len(statusOptions) {
				newStatusStr := statusOptions[a.dialogState.StatusIndex]
				// Parse the string to tracker status (convert to lowercase for ParseWatchStatus)
				newStatus, err := tracker.ParseWatchStatus(strings.ToLower(newStatusStr))
				if err != nil {
					// Fallback to Plan to Watch if parsing fails
					newStatus = tracker.StatusPlanToWatch
				}

				// Create the message with the selected media
				a.dialogMode = anilist.DialogNone
				return a, func() tea.Msg {
					return anilist.AniListAddToListMsg{
						Media:  a.currentAniListMedia,
						Status: newStatus,
					}
				}
			}

		}

	case "up", "k":
		// Status dialog navigation
		if a.dialogMode == anilist.DialogStatus {
			if a.dialogState.StatusIndex > 0 {
				a.dialogState.StatusIndex--
			}
			return a, nil
		}
		// AddToList dialog navigation
		if a.dialogMode == anilist.DialogAddToList {
			if a.dialogState.StatusIndex > 0 {
				a.dialogState.StatusIndex--
			}
			return a, nil
		}

	case "down", "j":
		// Status dialog navigation
		if a.dialogMode == anilist.DialogStatus {
			statusOptions := []string{"CURRENT", "COMPLETED", "PAUSED", "DROPPED", "PLANNING", "REPEATING"}
			if a.dialogState.StatusIndex < len(statusOptions)-1 {
				a.dialogState.StatusIndex++
			}
			return a, nil
		}
		// AddToList dialog navigation
		if a.dialogMode == anilist.DialogAddToList {
			statusOptions := []string{"CURRENT", "COMPLETED", "PAUSED", "DROPPED", "PLANNING", "REPEATING"}
			if a.dialogState.StatusIndex < len(statusOptions)-1 {
				a.dialogState.StatusIndex++
			}
			return a, nil
		}

	case "+", "=":
		// Increment progress in progress dialog
		if a.dialogMode == anilist.DialogProgress {
			selectedMedia := a.currentAniListMedia
			if selectedMedia != nil {
				currentVal := a.dialogState.ProgressInput.Value()
				if progress, err := anilist.ParseProgress(currentVal); err == nil {
					// Don't increment beyond total episodes
					if selectedMedia.TotalEpisodes > 0 && progress >= selectedMedia.TotalEpisodes {
						// Already at max, do nothing
						return a, nil
					}
					a.dialogState.ProgressInput.SetValue(fmt.Sprintf("%d", progress+1))
				}
			}
			return a, nil
		}

	case "-", "_":
		// Decrement progress in progress dialog
		if a.dialogMode == anilist.DialogProgress {
			currentVal := a.dialogState.ProgressInput.Value()
			if progress, err := anilist.ParseProgress(currentVal); err == nil {
				if progress > 0 {
					a.dialogState.ProgressInput.SetValue(fmt.Sprintf("%d", progress-1))
				}
			}
			return a, nil
		}

	case "a", "A":
		// Set progress to all episodes (progress dialog only)
		if a.dialogMode == anilist.DialogProgress {
			selectedMedia := a.currentAniListMedia
			if selectedMedia != nil && selectedMedia.TotalEpisodes > 0 {
				a.dialogState.ProgressInput.SetValue(fmt.Sprintf("%d", selectedMedia.TotalEpisodes))
			}
			return a, nil
		}

	case "y", "Y":
		// Handle confirmation for delete dialog
		if a.dialogMode == anilist.DialogDelete && a.currentAniListMedia != nil {
			// Trigger deletion request
			a.dialogMode = anilist.DialogNone
			// Use the MediaListEntry ID that was stored with the tracked media
			mediaListEntryID := a.currentAniListMedia.ListEntryID
			return a, func() tea.Msg {
				return anilist.AniListDeleteRequestedMsg{MediaListID: mediaListEntryID}
			}
		}

	case "n", "N":
		// Handle cancellation for delete dialog
		if a.dialogMode == anilist.DialogDelete {
			a.dialogMode = anilist.DialogNone
			return a, nil
		}

	default:
		// Pass input to text inputs for score/progress dialogs
		var cmd tea.Cmd
		switch a.dialogMode {
		case anilist.DialogScore:
			a.dialogState.ScoreInput, cmd = a.dialogState.ScoreInput.Update(msg)
			return a, cmd
		case anilist.DialogProgress:
			a.dialogState.ProgressInput, cmd = a.dialogState.ProgressInput.Update(msg)
			return a, cmd
		}
	}

	return a, nil
}

// updateAniListStatus updates the watch status of a media on AniList
func (a *App) updateAniListStatus(media *tracker.TrackedMedia, newStatus string) tea.Cmd {
	return func() tea.Msg {
		// Type assert to tracker.Manager
		mgr, ok := a.trackerMgr.(*tracker.Manager)
		if !ok {
			return anilist.StatusUpdatedMsg{
				MediaID: media.ServiceID,
				Status:  newStatus,
				Error:   fmt.Errorf("failed to access tracker manager"),
			}
		}

		// Get AniList client
		anilistClient := mgr.GetAniList()
		if anilistClient == nil {
			return anilist.StatusUpdatedMsg{
				MediaID: media.ServiceID,
				Status:  newStatus,
				Error:   fmt.Errorf("AniList client not available"),
			}
		}

		// Convert status string to tracker.WatchStatus
		var status tracker.WatchStatus
		switch newStatus {
		case "CURRENT":
			status = tracker.StatusWatching
		case "COMPLETED":
			status = tracker.StatusCompleted
		case "PAUSED":
			status = tracker.StatusOnHold
		case "DROPPED":
			status = tracker.StatusDropped
		case "PLANNING":
			status = tracker.StatusPlanToWatch
		case "REPEATING":
			status = tracker.StatusRewatching
		default:
			return anilist.StatusUpdatedMsg{
				MediaID: media.ServiceID,
				Status:  newStatus,
				Error:   fmt.Errorf("unknown status: %s", newStatus),
			}
		}

		// Update status on AniList
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := anilistClient.UpdateStatus(ctx, media.ServiceID, status)
		return anilist.StatusUpdatedMsg{
			MediaID: media.ServiceID,
			Status:  newStatus,
			Error:   err,
		}
	}
}

// updateAniListScore updates the score of a media on AniList
func (a *App) updateAniListScore(media *tracker.TrackedMedia, newScore float64) tea.Cmd {
	return func() tea.Msg {
		// Type assert to tracker.Manager
		mgr, ok := a.trackerMgr.(*tracker.Manager)
		if !ok {
			return anilist.ScoreUpdatedMsg{
				MediaID: media.ServiceID,
				Score:   newScore,
				Error:   fmt.Errorf("failed to access tracker manager"),
			}
		}

		// Get AniList client
		anilistClient := mgr.GetAniList()
		if anilistClient == nil {
			return anilist.ScoreUpdatedMsg{
				MediaID: media.ServiceID,
				Score:   newScore,
				Error:   fmt.Errorf("AniList client not available"),
			}
		}

		// Update score on AniList
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := anilistClient.UpdateScore(ctx, media.ServiceID, newScore)
		return anilist.ScoreUpdatedMsg{
			MediaID: media.ServiceID,
			Score:   newScore,
			Error:   err,
		}
	}
}

// updateAniListProgress updates the episode progress of a media on AniList
func (a *App) updateAniListProgress(media *tracker.TrackedMedia, newProgress int) tea.Cmd {
	return func() tea.Msg {
		// Type assert to tracker.Manager
		mgr, ok := a.trackerMgr.(*tracker.Manager)
		if !ok {
			return anilist.ProgressUpdatedMsg{
				MediaID:  media.ServiceID,
				Episode:  newProgress,
				Progress: 0,
				Error:    fmt.Errorf("failed to access tracker manager"),
			}
		}

		// Get AniList client
		anilistClient := mgr.GetAniList()
		if anilistClient == nil {
			return anilist.ProgressUpdatedMsg{
				MediaID:  media.ServiceID,
				Episode:  newProgress,
				Progress: 0,
				Error:    fmt.Errorf("AniList client not available"),
			}
		}

		// Update progress on AniList
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Progress percentage is 100% since we're setting episode count
		err := anilistClient.UpdateProgress(ctx, media.ServiceID, newProgress, 1.0)
		return anilist.ProgressUpdatedMsg{
			MediaID:  media.ServiceID,
			Episode:  newProgress,
			Progress: 1.0,
			Error:    err,
		}
	}
}

// searchProvidersForAniList searches providers for an AniList anime
func (a *App) searchProvidersForAniList(media *tracker.TrackedMedia) tea.Cmd {
	return func() tea.Msg {
		a.debugLog("searchProvidersForAniList: Called for '%s' (ServiceID: %s)",
			media.Title, media.ServiceID)

		if a.mappingMgr == nil {
			a.debugLog("ERROR: searchProvidersForAniList: mappingMgr is nil")
			return anilist.ProviderSearchResultMsg{
				AniListID: extractAniListID(media.ServiceID),
				Error:     fmt.Errorf("mapping manager not initialized"),
			}
		}

		mgr, ok := a.mappingMgr.(*mapping.Manager)
		if !ok {
			a.debugLog("ERROR: searchProvidersForAniList: Type assertion failed")
			return anilist.ProviderSearchResultMsg{
				AniListID: extractAniListID(media.ServiceID),
				Error:     fmt.Errorf("invalid mapping manager type"),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		anilistID := extractAniListID(media.ServiceID)
		a.debugLog("searchProvidersForAniList: Extracted AniList ID: %d", anilistID)

		// Determine provider type based on media type
		providerType := providers.MediaTypeAnime
		if media.Type == providers.MediaTypeManga {
			providerType = providers.MediaTypeManga
		}

		// Get the provider name
		availProviders := providers.GetByType(providerType)
		if len(availProviders) == 0 {
			a.debugLog("ERROR: searchProvidersForAniList: No providers available for %s", providerType)
			return anilist.ProviderSearchResultMsg{
				AniListID: anilistID,
				Error:     fmt.Errorf("no providers available for %s", providerType),
			}
		}
		providerName := availProviders[0].Name()
		a.debugLog("searchProvidersForAniList: Using provider: %s", providerName)

		// Try to get or create mapping
		a.debugLog("searchProvidersForAniList: Checking for existing mapping...")
		providerMapping, searchResults, err := mgr.GetOrCreateMapping(
			ctx,
			anilistID,
			media.Title,
			providerType,
		)

		if err != nil {
			a.debugLog("ERROR: searchProvidersForAniList: GetOrCreateMapping failed: %v", err)
			return anilist.ProviderSearchResultMsg{
				AniListID: anilistID,
				Error:     err,
			}
		}

		// If we got an existing mapping, return it
		if providerMapping != nil {
			a.debugLog("SUCCESS: searchProvidersForAniList: Found existing mapping (Provider: %s, MediaID: %s)",
				providerMapping.ProviderName, providerMapping.ProviderMediaID)
			return anilist.ProviderSearchResultMsg{
				AniListID:    anilistID,
				ProviderName: providerName,
				Mapping:      providerMapping,
			}
		}

		a.debugLog("searchProvidersForAniList: No existing mapping, returning %d search results",
			len(searchResults))

		// Otherwise, return search results for user selection
		var results []interface{}
		for _, r := range searchResults {
			results = append(results, r)
		}

		return anilist.ProviderSearchResultMsg{
			AniListID:     anilistID,
			ProviderName:  providerName,
			SearchResults: results,
		}
	}
}

// extractAniListID extracts the numeric AniList ID from a ServiceID string
func extractAniListID(serviceID string) int {
	// ServiceID is typically "anilist:12345" or just the number
	parts := strings.Split(serviceID, ":")
	idStr := serviceID
	if len(parts) > 1 {
		idStr = parts[1]
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0
	}
	return id
}

// searchAniListForNewAnime searches AniList for new anime by title
func (a *App) searchAniListForNewAnime(query string) tea.Cmd {
	return func() tea.Msg {
		if a.trackerMgr == nil {
			return anilist.AniListSearchResultMsg{
				Query:   query,
				Results: []tracker.TrackedMedia{},
				Error:   fmt.Errorf("tracker manager not initialized"),
			}
		}

		mgr, ok := a.trackerMgr.(*tracker.Manager)
		if !ok || mgr.GetAniList() == nil {
			return anilist.AniListSearchResultMsg{
				Query:   query,
				Results: []tracker.TrackedMedia{},
				Error:   fmt.Errorf("AniList tracker not available"),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Determine media type based on current app state
		mediaType := providers.MediaTypeAnime
		if a.currentMediaType == providers.MediaTypeManga {
			mediaType = providers.MediaTypeManga
		}

		results, err := mgr.SearchMedia(ctx, query, mediaType)
		if err != nil {
			return anilist.AniListSearchResultMsg{
				Query:   query,
				Results: []tracker.TrackedMedia{},
				Error:   fmt.Errorf("failed to search Anilist: %w", err),
			}
		}

		return anilist.AniListSearchResultMsg{
			Query:   query,
			Results: results,
			Error:   nil,
		}
	}
}

// deleteFromAniList removes a media from AniList by its list entry ID
func (a *App) deleteFromAniList(mediaListID int) tea.Cmd {
	return func() tea.Msg {
		if a.trackerMgr == nil {
			return anilist.AniListDeleteResultMsg{
				Error: fmt.Errorf("tracker manager not initialized"),
			}
		}

		mgr, ok := a.trackerMgr.(*tracker.Manager)
		if !ok || mgr.GetAniList() == nil {
			return anilist.AniListDeleteResultMsg{
				Error: fmt.Errorf("AniList tracker not available"),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := mgr.GetAniList().DeleteFromList(ctx, mediaListID)
		if err != nil {
			return anilist.AniListDeleteResultMsg{
				Error: fmt.Errorf("failed to delete from Anilist: %w", err),
			}
		}

		return anilist.AniListDeleteResultMsg{
			Error: nil,
		}
	}
}

// Message handlers

// handleAniListSearchRequestedMsg handles AniList search request
func (a *App) handleAniListSearchRequestedMsg(msg anilist.AniListSearchRequestedMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Handle AniList search request
	cmds = append(cmds, a.spinner.Tick, a.searchAniListForNewAnime(msg.Query))
	return a, tea.Batch(cmds...)
}

// handleAniListSearchResultMsg handles AniList search results
func (a *App) handleAniListSearchResultMsg(msg anilist.AniListSearchResultMsg) (*App, tea.Cmd) {
	// Handle AniList search results - pass to AniList component
	var anilistModel tea.Model
	var cmd tea.Cmd
	anilistModel, cmd = a.anilistComponent.Update(msg)
	a.anilistComponent = anilistModel.(anilist.Model)
	if msg.Error != nil {
		a.err = msg.Error
		a.state = errorView
		return a, nil
	}
	return a, cmd
}

// handleAniListAddToListDialogOpenMsg handles opening add-to-list dialog
func (a *App) handleAniListAddToListDialogOpenMsg(msg anilist.AniListAddToListDialogOpenMsg) (*App, tea.Cmd) {
	// Open the add-to-list dialog with the selected media
	if msg.Media != nil {
		a.dialogMode = anilist.DialogAddToList
		a.anilistSearchRetried = false // Reset retry flag for new anime selection
		// Store the media for the dialog to use
		a.currentAniListMedia = msg.Media
	}
	return a, nil
}

// handleAniListAddToListMsg handles adding anime to AniList
func (a *App) handleAniListAddToListMsg(msg anilist.AniListAddToListMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// User selected an anime and status to add to AniList
	if msg.Media != nil {
		// Add to AniList with the selected status
		if a.trackerMgr != nil {
			trackerMgr, ok := a.trackerMgr.(*tracker.Manager)
			if ok && trackerMgr.GetAniList() != nil {
				// Update status to the selected status
				anilistID := extractAniListID(msg.Media.ServiceID)
				ctx := context.Background()
				err := trackerMgr.GetAniList().UpdateStatus(ctx, fmt.Sprintf("%d", anilistID), msg.Status)
				if err != nil {
					a.logger.Warn("could not update AniList status", "error", err)
				} else {
					statusText := "Watching"
					switch msg.Status {
					case tracker.StatusPlanToWatch:
						statusText = "Plan to Watch"
					case tracker.StatusCompleted:
						statusText = "Completed"
					case tracker.StatusOnHold:
						statusText = "On Hold"
					case tracker.StatusDropped:
						statusText = "Dropped"
					}
					a.logger.Info("added to AniList", "title", msg.Media.Title, "status", statusText)
				}
			}
		}

		// Only start playback if status is "Watching", otherwise return to AniList menu
		if msg.Status == tracker.StatusWatching {
			// Start playback with AniList context set
			a.watchingFromAniList = true
			a.currentAniListMedia = msg.Media
			a.currentAniListID = extractAniListID(msg.Media.ServiceID)
			a.anilistSearchRetried = false
			a.state = loadingView
			a.loadingOp = loadingProviderSearch
			cmds = append(cmds, a.spinner.Tick, a.searchProvidersForAniList(msg.Media))
		} else {
			// Status is not "Watching", so just refresh the AniList library and return to AniList view
			a.state = loadingView
			a.loadingOp = loadingAniListLibrary
			cmds = append(cmds, a.spinner.Tick, a.fetchAniListLibrary())
		}
	}
	return a, tea.Batch(cmds...)
}

// handleAniListDeleteConfirmationMsg handles delete confirmation dialog
func (a *App) handleAniListDeleteConfirmationMsg(msg anilist.AniListDeleteConfirmationMsg) (*App, tea.Cmd) {
	// Open the delete confirmation dialog with the selected media
	if msg.Media != nil {
		a.dialogMode = anilist.DialogDelete
		// Store the media for the dialog to use
		a.currentAniListMedia = msg.Media
	}
	return a, nil
}

// handleAniListSearchSelectMsg handles legacy search select (deprecated)
func (a *App) handleAniListSearchSelectMsg(msg anilist.AniListSearchSelectMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// This is now legacy handling - if someone still sends this message,
	// add to AniList with "CURRENT" status as default
	if msg.Media != nil {
		// Add to AniList with "CURRENT" status
		if a.trackerMgr != nil {
			trackerMgr, ok := a.trackerMgr.(*tracker.Manager)
			if ok && trackerMgr.GetAniList() != nil {
				// Update status to CURRENT
				anilistID := extractAniListID(msg.Media.ServiceID)
				ctx := context.Background()
				err := trackerMgr.GetAniList().UpdateStatus(ctx, fmt.Sprintf("%d", anilistID), tracker.StatusWatching)
				if err != nil {
					// Log error but continue with playback
					a.logger.Warn("could not update AniList status", "error", err)
				} else {
					a.logger.Info("added to AniList", "title", msg.Media.Title, "status", "Watching")
				}
			}
		}

		// Now start playback with AniList context set
		a.watchingFromAniList = true
		a.currentAniListMedia = msg.Media
		a.currentAniListID = extractAniListID(msg.Media.ServiceID)
		a.anilistSearchRetried = false
		a.state = loadingView
		a.loadingOp = loadingProviderSearch
		cmds = append(cmds, a.spinner.Tick, a.searchProvidersForAniList(msg.Media))
	}
	return a, tea.Batch(cmds...)
}

// handleAniListDeleteRequestedMsg handles delete request
func (a *App) handleAniListDeleteRequestedMsg(msg anilist.AniListDeleteRequestedMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Handle request to delete from AniList
	cmds = append(cmds, a.spinner.Tick, a.deleteFromAniList(msg.MediaListID))
	return a, tea.Batch(cmds...)
}

// handleAniListDeleteResultMsg handles delete result
func (a *App) handleAniListDeleteResultMsg(msg anilist.AniListDeleteResultMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	if msg.Error != nil {
		a.err = fmt.Errorf("failed to delete from Anilist: %v", msg.Error)
		a.state = errorView
		return a, nil
	}

	// Success: show confirmation and refresh the library
	a.statusMsg = "✓ Successfully deleted from Anilist"
	a.statusMsgTime = time.Now()
	a.state = loadingView
	a.loadingOp = loadingAniListLibrary
	// Clear status message after 2 seconds
	clearCmd := func() tea.Msg {
		time.Sleep(2 * time.Second)
		return clearStatusMsg{}
	}
	cmds = append(cmds, a.spinner.Tick, a.fetchAniListLibrary(), clearCmd)
	return a, tea.Batch(cmds...)
}

// handleSelectMediaMsg handles selecting anime from AniList library
func (a *App) handleSelectMediaMsg(msg anilist.SelectMediaMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// User selected an anime from AniList library - search providers
	if msg.Media != nil {
		a.watchingFromAniList = true
		a.currentAniListMedia = msg.Media
		a.currentAniListID = extractAniListID(msg.Media.ServiceID)
		a.anilistSearchRetried = false // Reset retry flag for new anime selection
		a.state = loadingView
		a.loadingOp = loadingProviderSearch
		cmds = append(cmds, a.spinner.Tick, a.searchProvidersForAniList(msg.Media))
	}
	return a, tea.Batch(cmds...)
}

// handleRefreshLibraryMsg handles library refresh request
func (a *App) handleRefreshLibraryMsg(msg anilist.RefreshLibraryMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Refresh the AniList library
	a.state = loadingView
	a.loadingOp = loadingAniListLibrary
	cmds = append(cmds, a.spinner.Tick, a.fetchAniListLibrary())
	return a, tea.Batch(cmds...)
}

// handleOpenStatusUpdateMsg handles opening status update dialog
func (a *App) handleOpenStatusUpdateMsg(msg anilist.OpenStatusUpdateMsg) (*App, tea.Cmd) {
	// Open status update dialog
	if msg.Media != nil {
		a.currentAniListMedia = msg.Media
		a.dialogMode = anilist.DialogStatus
		// Find current status index in status options
		a.dialogState.StatusIndex = 0 // Default to first option
		for i, opt := range []string{"CURRENT", "COMPLETED", "PAUSED", "DROPPED", "PLANNING", "REPEATING"} {
			if string(msg.Media.Status) == opt {
				a.dialogState.StatusIndex = i
				break
			}
		}
	}
	return a, nil
}

// handleOpenScoreUpdateMsg handles opening score update dialog
func (a *App) handleOpenScoreUpdateMsg(msg anilist.OpenScoreUpdateMsg) (*App, tea.Cmd) {
	// Open score update dialog
	if msg.Media != nil {
		a.currentAniListMedia = msg.Media
		a.dialogMode = anilist.DialogScore
		// Set current score in input
		if msg.Media.Score > 0 {
			a.dialogState.ScoreInput.SetValue(fmt.Sprintf("%.1f", msg.Media.Score))
		} else {
			a.dialogState.ScoreInput.SetValue("")
		}
		a.dialogState.ScoreInput.Focus()
	}
	return a, nil
}

// handleOpenProgressUpdateMsg handles opening progress update dialog
func (a *App) handleOpenProgressUpdateMsg(msg anilist.OpenProgressUpdateMsg) (*App, tea.Cmd) {
	// Open progress update dialog
	if msg.Media != nil {
		a.currentAniListMedia = msg.Media
		a.dialogMode = anilist.DialogProgress
		// Set current progress in input
		a.dialogState.ProgressInput.SetValue(fmt.Sprintf("%d", msg.Media.Progress))
		a.dialogState.ProgressInput.Focus()
	}
	return a, nil
}

// handleStatusUpdatedMsg handles status update result
func (a *App) handleStatusUpdatedMsg(msg anilist.StatusUpdatedMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Handle status update result
	if msg.Error != nil {
		a.err = fmt.Errorf("failed to update status: %v", msg.Error)
		a.state = errorView
		return a, nil
	}
	// Refresh library to get updated data
	a.state = loadingView
	a.loadingOp = loadingAniListLibrary
	cmds = append(cmds, a.spinner.Tick, a.fetchAniListLibrary())
	return a, tea.Batch(cmds...)
}

// handleScoreUpdatedMsg handles score update result
func (a *App) handleScoreUpdatedMsg(msg anilist.ScoreUpdatedMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Handle score update result
	if msg.Error != nil {
		a.err = fmt.Errorf("failed to update score: %v", msg.Error)
		a.state = errorView
		return a, nil
	}
	// Refresh library to get updated data
	a.state = loadingView
	a.loadingOp = loadingAniListLibrary
	cmds = append(cmds, a.spinner.Tick, a.fetchAniListLibrary())
	return a, tea.Batch(cmds...)
}

// handleProgressUpdatedMsg handles progress update result
func (a *App) handleProgressUpdatedMsg(msg anilist.ProgressUpdatedMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Handle progress update result
	if msg.Error != nil {
		a.err = fmt.Errorf("failed to update progress: %v", msg.Error)
		a.state = errorView
		return a, nil
	}

	// If we are reading manga, don't switch view, just show notification
	if a.state == mangaReaderView {
		a.statusMsg = fmt.Sprintf("✓ Progress updated: Chapter %d", msg.Episode)
		a.mangaComponent.StatusMessage = a.statusMsg
		a.statusMsgTime = time.Now()
		return a, func() tea.Msg {
			time.Sleep(2 * time.Second)
			return clearStatusMsg{}
		}
	}

	// Refresh library to get updated data
	a.state = loadingView
	a.loadingOp = loadingAniListLibrary
	cmds = append(cmds, a.spinner.Tick, a.fetchAniListLibrary(), tea.ClearScreen)
	return a, tea.Batch(cmds...)
}

// handleProviderSearchResultMsg handles provider search results
func (a *App) handleProviderSearchResultMsg(msg anilist.ProviderSearchResultMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Handle provider search results
	if msg.Error != nil {
		a.err = fmt.Errorf("failed to find anime on providers: %v", msg.Error)
		a.state = errorView
		return a, nil
	}

	// Store the provider name
	a.providerName = msg.ProviderName

	// If we got a direct mapping (existing), proceed to fetch episodes
	if msg.Mapping != nil {
		providerMapping, ok := msg.Mapping.(*mapping.ProviderMapping)
		if !ok || providerMapping.Media == nil {
			a.err = fmt.Errorf("invalid provider mapping")
			a.state = errorView
			return a, nil
		}

		// Set the selected media and provider
		a.selectedMedia = *providerMapping.Media

		// Enforce type if watching from AniList and it's manga
		if a.watchingFromAniList && a.currentAniListMedia != nil && a.currentAniListMedia.Type == providers.MediaTypeManga {
			a.selectedMedia.Type = providers.MediaTypeManga
		}

		a.currentMediaType = a.selectedMedia.Type

		// Update provider instance to ensure we use the correct one
		if p, err := providers.Get(msg.ProviderName); err == nil {
			a.updateProvider(p)
		}

		// Fetch seasons for this media
		a.state = loadingView
		a.loadingOp = loadingSeasons
		cmds = append(cmds, a.spinner.Tick, a.getSeasons(providerMapping.ProviderMediaID))
		return a, tea.Batch(cmds...)
	}

	// If we got search results, show selection UI
	if len(msg.SearchResults) > 0 {
		// Convert interface{} to providers.Media
		var mediaResults []providers.Media
		for _, r := range msg.SearchResults {
			if media, ok := r.(providers.Media); ok {
				// Enforce type if watching from AniList and it's manga
				if a.watchingFromAniList && a.currentAniListMedia != nil && a.currentAniListMedia.Type == providers.MediaTypeManga {
					media.Type = providers.MediaTypeManga
				}
				// Filter out media items with empty titles to prevent empty entries
				if strings.TrimSpace(media.Title) != "" {
					mediaResults = append(mediaResults, media)
				}
			}
		}

		// If there's only 1 result when watching from AniList, use it directly
		if len(mediaResults) == 1 && a.watchingFromAniList {
			testMedia := mediaResults[0]
			a.debugLog("Only 1 search result found for AniList content, using directly: %s (ID: %s)", testMedia.Title, testMedia.ID)

			// Save the mapping automatically and proceed to get seasons/episodes
			if a.mappingMgr != nil {
				if mgr, ok := a.mappingMgr.(*mapping.Manager); ok {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()

					a.debugLog("Auto-selecting mapping (AniList: %d → Provider: %s, Media: %s)",
						a.currentAniListID, msg.ProviderName, testMedia.ID)

					if err := mgr.SelectMapping(ctx, a.currentAniListID, msg.ProviderName, testMedia); err != nil {
						a.debugLog("ERROR: Failed to save auto-selected mapping: %v", err)
						// Continue anyway, just log the error
					} else {
						a.debugLog("SUCCESS: Auto-selected mapping saved successfully")
					}
				}
			}

			// Set selected media and proceed to fetch seasons
			a.selectedMedia = testMedia
			a.currentMediaType = testMedia.Type
			a.state = loadingView
			a.loadingOp = loadingSeasons
			cmds = append(cmds, a.spinner.Tick, a.getSeasons(testMedia.ID))
			return a, tea.Batch(cmds...)
		}

		a.providerSearchResults = mediaResults

		// Reuse the results component for provider selection
		a.providerSelectionResult = results.New()
		a.providerSelectionResult.SetMediaResults(mediaResults)
		a.providerSelectionResult.SetIsProviderSelection(true) // Mark as provider selection

		a.state = providerSelectionView
		return a, nil
	}

	a.err = fmt.Errorf("no search results returned")
	a.state = errorView
	return a, nil
}

// handleRemapRequestedMsg handles remap request
func (a *App) handleRemapRequestedMsg(msg anilist.RemapRequestedMsg) (*App, tea.Cmd) {
	// User wants to remap the provider for this anime
	if msg.Media != nil {
		// TODO: Implement remapping flow
		a.err = fmt.Errorf("provider remapping not yet available in beta")
		a.state = errorView
	}
	return a, nil
}
