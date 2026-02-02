package tui

// This file contains playback-related methods extracted from model.go
// for better code organization. All methods remain on the App struct.

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gorm.io/gorm"

	"github.com/justchokingaround/greg/internal/audio"
	"github.com/justchokingaround/greg/internal/database"
	"github.com/justchokingaround/greg/internal/player"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tracker"
	"github.com/justchokingaround/greg/internal/tracker/mapping"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

// monitorPlayback schedules the first playback status check
func (a *App) monitorPlayback() tea.Cmd {
	// Schedule tick AND start async progress check
	return tea.Batch(
		tea.Tick(1*time.Second, func(time.Time) tea.Msg { // Reduced from 2s to 1s for faster detection
			return common.PlaybackTickMsg{}
		}),
		a.checkPlaybackProgress(),
	)
}

// checkPlaybackProgress runs GetProgress in a goroutine (truly async)
func (a *App) checkPlaybackProgress() tea.Cmd {
	if a.player == nil {
		return nil
	}
	playerRef := a.player
	return func() tea.Msg {
		// Don't try to get progress if no longer in playing view
		if a.state != playingView {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		progress, err := playerRef.GetProgress(ctx)
		return common.PlaybackProgressMsg{Progress: progress, Err: err}
	}
}

// handlePlaybackTickMsg just schedules next check - no blocking!
func (a *App) handlePlaybackTickMsg() tea.Cmd {
	// If we're no longer in playingView, stop monitoring
	if a.state != playingView {
		a.debugLog("handlePlaybackTickMsg: state changed from playingView, stopping monitoring")
		return nil
	}

	// Check if player reference is gone
	if a.player == nil {
		a.debugLog("handlePlaybackTickMsg: player is nil, ending playback")
		a.syncProgressOnEnd(a.lastProgress)
		return func() tea.Msg {
			return createPlaybackEndedMsg(a.lastProgress)
		}
	}

	// Schedule next tick AND start async progress check
	// The progress check runs in a goroutine, doesn't block
	return tea.Batch(
		tea.Tick(1*time.Second, func(time.Time) tea.Msg { // Reduced from 2s to 1s
			return common.PlaybackTickMsg{}
		}),
		a.checkPlaybackProgress(),
	)
}

// handlePlaybackProgressMsg processes async GetProgress result
func (a *App) handlePlaybackProgressMsg(msg common.PlaybackProgressMsg) tea.Cmd {
	// If we're no longer in playingView, ignore stale message
	if a.state != playingView {
		return nil
	}

	if msg.Err != nil {
		errMsg := msg.Err.Error()
		a.debugLog("handlePlaybackProgressMsg: GetProgress error: %v", msg.Err)

		// Timeout is fine - mpv is just busy, wait for next tick
		if strings.Contains(errMsg, "context deadline exceeded") {
			return nil
		}

		// "player not initialized" during startup grace period - IPC still connecting
		// Give mpv up to 10 seconds to initialize IPC before treating as fatal error
		if strings.Contains(errMsg, "player not initialized") {
			timeSinceLaunch := time.Since(a.launchStartTime)
			if timeSinceLaunch < 10*time.Second {
				a.debugLog("handlePlaybackProgressMsg: IPC not ready yet (%.1fs since launch), waiting...", timeSinceLaunch.Seconds())
				return nil // Wait for next tick
			}
		}

		// Check for Windows-specific IPC failure patterns
		if runtime.GOOS == "windows" {
			if strings.Contains(errMsg, "pipe has been ended") ||
				strings.Contains(errMsg, "file has been closed") ||
				strings.Contains(errMsg, "All pipe instances are busy") ||
				strings.Contains(errMsg, "player not initialized") {
				a.debugLog("handlePlaybackProgressMsg[Windows]: IPC connection lost")
				a.syncProgressOnEnd(a.lastProgress)
				return func() tea.Msg {
					return createPlaybackEndedMsg(a.lastProgress)
				}
			}
		}

		// Linux errors indicating mpv closed
		if strings.Contains(errMsg, "broken pipe") ||
			strings.Contains(errMsg, "connection refused") ||
			strings.Contains(errMsg, "no such file") {
			a.syncProgressOnEnd(a.lastProgress)
			return func() tea.Msg {
				return createPlaybackEndedMsg(a.lastProgress)
			}
		}

		// Unknown error - just wait for next tick
		return nil
	}

	// Store progress
	a.lastProgress = msg.Progress

	// Check if playback has ended
	if msg.Progress.EOF {
		a.debugLog("handlePlaybackProgressMsg: EOF reached, ending playback")
		a.syncProgressOnEnd(a.lastProgress)
		return func() tea.Msg {
			return createPlaybackEndedMsg(a.lastProgress)
		}
	}

	// Progress updated, tick will handle next check
	return nil
}

// autoReturnAfterDelay returns a command that sends PlaybackAutoReturnMsg after a delay
func (a *App) autoReturnAfterDelay(delay time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(delay)
		return common.PlaybackAutoReturnMsg{}
	}
}

// selectBestSubtitle selects the best subtitle from available options, preferring English
func selectBestSubtitle(subtitles []providers.Subtitle) *providers.Subtitle {
	if len(subtitles) == 0 {
		return nil
	}

	// Try to find English subtitle (various language codes)
	englishCodes := []string{"en", "eng", "english", "en-US", "en-GB"}
	for _, sub := range subtitles {
		subLang := strings.ToLower(sub.Language)
		for _, code := range englishCodes {
			if strings.Contains(subLang, strings.ToLower(code)) {
				return &sub
			}
		}
	}

	// If no English subtitle found, return the first one
	return &subtitles[0]
}

// syncProgressOnEnd syncs playback progress to AniList when playback ends
func (a *App) syncProgressOnEnd(progress *player.PlaybackProgress) {
	a.debugLog("syncProgressOnEnd: Called with progress=%v", progress != nil)

	if progress == nil {
		a.debugLog("syncProgressOnEnd: progress is nil, returning")
		return
	}

	a.debugLog("syncProgressOnEnd: watchingFromAniList=%v, currentAniListID=%d, percentage=%.1f%%",
		a.watchingFromAniList, a.currentAniListID, progress.Percentage)

	if a.currentEpisodeID != "" && a.currentPlaybackProvider != "" {
		providerName := a.currentPlaybackProvider

		totalSeconds := int(progress.Duration.Seconds())
		currentSeconds := int(progress.CurrentTime.Seconds())
		completed := progress.Percentage >= 85.0

		episodeNumber := a.currentEpisodeNumber
		seasonNumber := a.currentSeasonNumber

		var anilistID *int
		if a.watchingFromAniList && a.currentAniListID > 0 {
			anilistIDVal := a.currentAniListID
			anilistID = &anilistIDVal
		} else {
			anilistID = nil
		}

		a.debugLog("syncProgressOnEnd: Saving history (provider=%s, anilistID=%v)...",
			providerName, anilistID)

		if err := a.savePlaybackProgress(
			anilistID,
			providerName,
			episodeNumber,
			currentSeconds,
			totalSeconds,
			completed,
		); err != nil {
			a.logger.Error("database save failed", "error", err)
			a.err = fmt.Errorf("failed to save progress to database: %v", err)
		}

		if completed && episodeNumber > 0 {
			nextEpisode := a.findNextEpisode(episodeNumber, seasonNumber)
			if nextEpisode != nil {
				if err := a.createNextEpisodePlaceholder(nextEpisode, seasonNumber, providerName, anilistID); err != nil {
					a.logger.Warn("failed to create next episode placeholder", "error", err)
				}
			}
		}

		a.currentPlaybackProvider = ""
	}

	// Only sync to AniList if progress >= 85%
	if progress.Percentage < 85.0 {
		a.debugLog("syncProgressOnEnd: Progress < 85%%, not syncing to AniList")
		return
	}

	// Check if we're watching from AniList and tracker is available
	if !a.watchingFromAniList {
		a.debugLog("syncProgressOnEnd: Not watching from AniList, skipping sync")
		return
	}
	if a.currentAniListID == 0 {
		a.logger.Error("syncProgressOnEnd: currentAniListID is 0, skipping sync\n")
		return
	}
	if a.trackerMgr == nil {
		a.logger.Error("syncProgressOnEnd: trackerMgr is nil, skipping sync\n")
		return
	}

	// Type assert to *tracker.Manager
	mgr, ok := a.trackerMgr.(*tracker.Manager)
	if !ok {
		a.logger.Error("syncProgressOnEnd: Type assertion to *tracker.Manager failed (type: %T)\n", a.trackerMgr)
		return
	}

	if !mgr.IsAniListEnabled() {
		a.debugLog("syncProgressOnEnd: AniList is not enabled")
		return
	}

	if !mgr.IsAniListAuthenticated() {
		a.debugLog("syncProgressOnEnd: AniList is not authenticated")
		return
	}

	a.debugLog("syncProgressOnEnd: All checks passed, starting AniList sync...")

	// Sync to AniList in the background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Use the AniList ID
		mediaID := fmt.Sprintf("%d", a.currentAniListID)

		a.debugLog("AniList Sync: Calling UpdateProgress(mediaID=%s, episode=%d, progress=1.0)",
			mediaID, a.currentEpisodeNumber)

		// Update progress (100% = episode completed)
		if err := mgr.UpdateProgress(ctx, mediaID, a.currentEpisodeNumber, 1.0); err != nil {
			// Set error for display
			a.logger.Error("AniList sync failed", "error", err)
			a.err = fmt.Errorf("failed to sync to anilist: %v", err)
		} else {
			a.logger.Info("AniList sync completed successfully")
		}

		// Check if this is the last episode
		if a.isLastEpisode {
			// TODO: Trigger score prompt dialog
			// This will be implemented in the score dialog component
			_ = a.isLastEpisode // acknowledge
		}
	}()
}

// createPlaybackEndedMsg creates a PlaybackEndedMsg with progress info
func createPlaybackEndedMsg(progress *player.PlaybackProgress) common.PlaybackEndedMsg {
	if progress == nil {
		return common.PlaybackEndedMsg{}
	}

	// Format durations as hh:mm:ss
	watchedHours := int(progress.CurrentTime.Hours())
	watchedMins := int(progress.CurrentTime.Minutes()) % 60
	watchedSecs := int(progress.CurrentTime.Seconds()) % 60

	totalHours := int(progress.Duration.Hours())
	totalMins := int(progress.Duration.Minutes()) % 60
	totalSecs := int(progress.Duration.Seconds()) % 60

	return common.PlaybackEndedMsg{
		WatchedPercentage: progress.Percentage,
		WatchedDuration:   fmt.Sprintf("%d:%02d:%02d", watchedHours, watchedMins, watchedSecs),
		TotalDuration:     fmt.Sprintf("%d:%02d:%02d", totalHours, totalMins, totalSecs),
	}
}

// checkPlayerLaunchStatus returns a command to check player launch status
func (a *App) checkPlayerLaunchStatus() tea.Cmd {
	return func() tea.Msg {
		// Small delay before checking
		time.Sleep(200 * time.Millisecond)
		return common.PlayerLaunchTimeoutCheckMsg{}
	}
}

// checkResumePosition checks if there's a saved progress for this anime/episode
func (a *App) checkResumePosition(anilistID int, episode int) (int, error) {
	a.debugLog("checkResumePosition: anilistID=%d, episode=%d", anilistID, episode)

	if a.db == nil {
		a.logger.Error("checkResumePosition: database is nil\n")
		return 0, fmt.Errorf("database is nil")
	}

	// Find the most recent history entry for this anime and episode
	var history database.History
	err := a.db.Where("anilist_id = ? AND episode = ? AND completed = false", anilistID, episode).
		Order("watched_at DESC").
		First(&history).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			a.debugLog("checkResumePosition: No history record found")
			return 0, nil // No resume position
		}
		a.logger.Error("check resume position query failed", "error", err)
		return 0, err
	}

	a.debugLog("checkResumePosition: Found history - Progress: %d/%d seconds (%.1f%%), Completed: %v",
		history.ProgressSeconds, history.TotalSeconds, history.ProgressPercent, history.Completed)

	// Only resume if progress is less than 85%
	if history.ProgressPercent >= 85.0 {
		a.debugLog("checkResumePosition: Progress >= 85%%, not resuming")
		return 0, nil
	}

	a.debugLog("checkResumePosition: Resuming from %d seconds", history.ProgressSeconds)
	return history.ProgressSeconds, nil
}

// savePlaybackProgress saves progress to the database
// Supports both AniList content (with anilistID) and direct provider content (anilistID = nil)
func (a *App) savePlaybackProgress(anilistIDPtr *int, providerName string, episode int, progressSeconds int, totalSeconds int, completed bool) error {
	// Handle nil AniList ID for non-AniList content
	var anilistID int
	hasAniListID := false
	if anilistIDPtr != nil {
		anilistID = *anilistIDPtr
		hasAniListID = true
		a.debugLog("savePlaybackProgress: anilistID=%d, episode=%d, progress=%d/%d, completed=%v",
			anilistID, episode, progressSeconds, totalSeconds, completed)
	} else {
		a.debugLog("savePlaybackProgress: Direct provider content, episode=%d, progress=%d/%d, completed=%v",
			episode, progressSeconds, totalSeconds, completed)
	}

	if a.db == nil {
		a.logger.Error("savePlaybackProgress: database is nil\n")
		return fmt.Errorf("database connection is nil")
	}

	progressPercent := 0.0
	if totalSeconds > 0 {
		progressPercent = (float64(progressSeconds) / float64(totalSeconds)) * 100.0
	}

	// Determine MediaID, MediaTitle, MediaType based on content source
	var mediaID string
	var mediaTitle string
	var mediaType string

	if hasAniListID {
		mediaID = fmt.Sprintf("anilist:%d", anilistID)
		mediaTitle = a.currentAniListMedia.Title
		mediaType = "anime"
	} else {
		mediaID = a.selectedMedia.ID
		mediaTitle = a.selectedMedia.Title

		switch a.selectedMedia.Type {
		case providers.MediaTypeAnime:
			mediaType = "anime"
		case providers.MediaTypeMovie:
			mediaType = "movie"
		case providers.MediaTypeTV:
			mediaType = "tv"
		case providers.MediaTypeManga:
			mediaType = "manga"
		case providers.MediaTypeMovieTV:
			if episode > 1 || a.currentSeasonNumber > 0 {
				mediaType = "tv"
			} else {
				mediaType = "movie"
			}
		default:
			mediaType = "movie"
		}
	}

	// If this is a completed watch, delete any previous incomplete records for this media/episode
	if completed {
		a.debugLog("savePlaybackProgress: Deleting old incomplete records for media_id=%s, episode=%d", mediaID, episode)
		result := a.db.Where("media_id = ? AND episode = ? AND completed = false", mediaID, episode).
			Delete(&database.History{})
		if result.Error != nil {
			a.logger.Warn("failed to delete old playback records", "error", result.Error)
		} else if result.RowsAffected > 0 {
			a.debugLog("savePlaybackProgress: Deleted %d old incomplete record(s)", result.RowsAffected)
		}
	} else {
		// For incomplete watches, update existing incomplete record if it exists
		var existing database.History
		err := a.db.Where("media_id = ? AND episode = ? AND completed = false", mediaID, episode).
			Order("watched_at DESC").
			First(&existing).Error

		if err == nil {
			// Update existing record
			a.debugLog("savePlaybackProgress: Updating existing incomplete record (ID: %d)", existing.ID)
			existing.ProgressSeconds = progressSeconds
			existing.TotalSeconds = totalSeconds
			existing.ProgressPercent = progressPercent
			existing.WatchedAt = time.Now()
			existing.ProviderName = providerName

			if err := a.db.Save(&existing).Error; err != nil {
				a.debugLog("ERROR: savePlaybackProgress: Failed to update history: %v", err)
				return fmt.Errorf("failed to update history record: %w", err)
			}

			a.debugLog("savePlaybackProgress: History record updated successfully (ID: %d)", existing.ID)
			return nil
		}
		// If no existing record, fall through to create new one
	}

	history := database.History{
		MediaID:         mediaID,
		MediaTitle:      mediaTitle,
		MediaType:       mediaType,
		Episode:         episode,
		Season:          a.currentSeasonNumber,
		ProgressSeconds: progressSeconds,
		TotalSeconds:    totalSeconds,
		ProgressPercent: progressPercent,
		WatchedAt:       time.Now(),
		Completed:       completed,
		AniListID:       anilistIDPtr, // Can be nil for non-AniList content
		ProviderName:    providerName,
	}

	a.debugLog("savePlaybackProgress: Creating new history record (mediaID=%s, mediaType=%s)...", mediaID, mediaType)
	if err := a.db.Create(&history).Error; err != nil {
		a.debugLog("ERROR: savePlaybackProgress: Failed to create history: %v", err)
		return fmt.Errorf("failed to create history record: %w", err)
	}

	a.debugLog("savePlaybackProgress: History record saved successfully (ID: %d)", history.ID)
	return nil
}

func (a *App) findNextEpisode(currentEpisode int, currentSeason int) *providers.Episode {
	if len(a.episodes) == 0 {
		return nil
	}

	for i := range a.episodes {
		if a.episodes[i].Number == currentEpisode+1 {
			return &a.episodes[i]
		}
	}

	return nil
}

func (a *App) createNextEpisodePlaceholder(nextEpisode *providers.Episode, seasonNumber int, providerName string, anilistIDPtr *int) error {
	if a.db == nil || nextEpisode == nil {
		return nil
	}

	mediaType := "movie"
	if a.selectedMedia.Type != "" {
		switch a.selectedMedia.Type {
		case providers.MediaTypeAnime:
			mediaType = "anime"
		case providers.MediaTypeManga:
			mediaType = "manga"
		case providers.MediaTypeMovie:
			mediaType = "movie"
		case providers.MediaTypeTV:
			mediaType = "tv"
		case providers.MediaTypeMovieTV:
			if nextEpisode.Number > 1 || seasonNumber > 0 {
				mediaType = "tv"
			} else {
				mediaType = "movie"
			}
		default:
			mediaType = "movie"
		}
	}

	history := database.History{
		MediaID:         a.selectedMedia.ID,
		MediaTitle:      a.selectedMedia.Title,
		MediaType:       mediaType,
		Episode:         nextEpisode.Number,
		Season:          seasonNumber,
		ProgressSeconds: 0,
		TotalSeconds:    0,
		ProgressPercent: 0,
		WatchedAt:       time.Now(),
		Completed:       false,
		AniListID:       anilistIDPtr,
		ProviderName:    providerName,
	}

	a.debugLog("createNextEpisodePlaceholder: Creating placeholder for next episode (episode=%d, season=%d)...", nextEpisode.Number, seasonNumber)
	if err := a.db.Create(&history).Error; err != nil {
		a.debugLog("ERROR: createNextEpisodePlaceholder: Failed to create placeholder: %v", err)
		return fmt.Errorf("failed to create next episode placeholder: %w", err)
	}

	a.debugLog("createNextEpisodePlaceholder: Placeholder created successfully (ID: %d)", history.ID)
	return nil
}

// cleanEpisodeTitle removes common episode number patterns from titles
func cleanEpisodeTitle(title string, mediaTitle string) string {
	cleanedTitle := title
	// Remove various episode number patterns
	re := regexp.MustCompile(`^(Eps?\.?\s*\d+[:\-\s]*|Episode\s+\d+[:\-\s]*)`)
	cleanedTitle = re.ReplaceAllString(cleanedTitle, "")
	cleanedTitle = strings.TrimSpace(cleanedTitle)

	// If cleaned title is empty or just "Movie", use media title
	if cleanedTitle == "" || cleanedTitle == "Movie" {
		cleanedTitle = mediaTitle
	}
	return cleanedTitle
}

// Message Handlers

// handleEpisodeSelectedMsg handles episode selection
func (a *App) handleEpisodeSelectedMsg(msg common.EpisodeSelectedMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd

	// Only set previousState if it wasn't already set (e.g., by auto-play)
	if a.previousState == 0 {
		a.previousState = a.state
	}
	a.currentEpisodeID = msg.EpisodeID

	// For single-episode content (anime movies), treat as movie with episode 0
	episodeNumber := msg.Number
	if len(a.episodes) == 1 {
		episodeNumber = 0
	}
	a.currentEpisodeNumber = episodeNumber

	// Check if this is the last episode (for AniList tracking)
	if a.watchingFromAniList && a.currentAniListMedia != nil {
		a.isLastEpisode = (episodeNumber >= a.currentAniListMedia.TotalEpisodes)
	} else {
		a.isLastEpisode = false
	}

	cleanedTitle := msg.Title
	// Remove various episode number patterns
	re := regexp.MustCompile(`^(Eps?\.?\s*\d+[:\-\s]*|Episode\s+\d+[:\-\s]*)`)
	cleanedTitle = re.ReplaceAllString(cleanedTitle, "")
	cleanedTitle = strings.TrimSpace(cleanedTitle)

	// If cleaned title is empty or just "Movie", use media title
	if cleanedTitle == "" || cleanedTitle == "Movie" {
		cleanedTitle = a.selectedMedia.Title
	}
	a.currentEpisodeTitle = cleanedTitle

	// Check for Manga
	if a.currentMediaType == providers.MediaTypeManga {
		a.state = loadingView
		a.loadingOp = loadingMangaPages
		cmds = append(cmds, a.spinner.Tick, a.getMangaPages(msg.EpisodeID))
		return a, tea.Batch(cmds...)
	}

	return a, func() tea.Msg {
		return common.PlaybackStartingMsg{
			EpisodeID:     msg.EpisodeID,
			EpisodeNumber: episodeNumber,
			EpisodeTitle:  cleanedTitle,
		}
	}
}

// handleMangaPagesLoadedMsg handles loaded manga pages
func (a *App) handleMangaPagesLoadedMsg(msg common.MangaPagesLoadedMsg) (*App, tea.Cmd) {
	if msg.Err != nil {
		a.err = msg.Err
		a.state = errorView
		return a, nil
	}
	// Pass title and chapter info
	// a.currentEpisodeTitle contains the chapter title (or media title if not available)
	// a.currentEpisodeNumber contains the chapter number
	chapterStr := fmt.Sprintf("Chapter %d", a.currentEpisodeNumber)
	if a.currentEpisodeTitle != "" && a.currentEpisodeTitle != a.selectedMedia.Title {
		chapterStr = fmt.Sprintf("Chapter %d: %s", a.currentEpisodeNumber, a.currentEpisodeTitle)
	}

	providerName := a.providerName
	if providerName == "" {
		if p, ok := a.providers[providers.MediaTypeManga]; ok {
			providerName = p.Name()
		}
	}

	var anilistID *int
	if a.watchingFromAniList {
		id := a.currentAniListID
		anilistID = &id
	}
	a.mangaComponent.SetContent(msg.Pages, a.selectedMedia.Title, chapterStr, a.selectedMedia.ID, a.currentEpisodeNumber, providerName, anilistID)
	a.state = mangaReaderView
	return a, a.mangaComponent.RenderCurrentPage()
}

// handleChapterCompletedMsg handles chapter completion
func (a *App) handleChapterCompletedMsg(msg common.ChapterCompletedMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	if a.watchingFromAniList {
		// Update AniList progress
		if a.currentAniListMedia != nil {
			cmds = append(cmds, a.updateAniListProgress(a.currentAniListMedia, msg.Chapter))
		}
	}
	return a, tea.Batch(cmds...)
}

// handleMangaQuitMsg handles manga reader quit
func (a *App) handleMangaQuitMsg(msg common.MangaQuitMsg) (*App, tea.Cmd) {
	a.state = episodeView
	if len(a.episodes) > 0 {
		a.episodesComponent.SetMediaType(a.selectedMedia.Type)
		a.episodesComponent.SetEpisodes(a.episodes)
		if a.currentEpisodeNumber > 0 {
			a.episodesComponent.SetCursorToEpisode(a.currentEpisodeNumber)
		}
	}
	// Clear screen to remove any Sixel artifacts
	return a, tea.ClearScreen
}

// handleResumePlaybackMsg handles resuming playback from history
func (a *App) handleResumePlaybackMsg(msg common.ResumePlaybackMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Resume playback from continue watching
	// We need to fetch the media and episodes first, then play
	a.selectedMedia = providers.Media{
		ID:    msg.MediaID,
		Title: msg.MediaTitle,
		Type:  providers.MediaType(msg.MediaType),
	}
	a.cameFromHistory = true
	a.currentEpisodeNumber = msg.Episode
	// Set previousState to episodeView so we return to the episode list after playback
	a.previousState = episodeView

	// Show loading state
	a.state = loadingView
	a.loadingOp = loadingStream

	// We need to construct an episode ID to fetch the stream
	// For now, we'll use the MediaID combined with episode info
	// This will need to fetch episodes first to get the proper episode ID
	cmds = append(cmds, a.spinner.Tick, a.resumePlaybackFromHistory(msg))
	return a, tea.Batch(cmds...)
}

// handlePlaybackStartingMsg handles playback initiation
func (a *App) handlePlaybackStartingMsg(msg common.PlaybackStartingMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Show loading state while fetching stream URL
	a.state = loadingView
	a.loadingOp = loadingStream
	cmds = append(cmds, a.spinner.Tick, a.startPlayback(msg.EpisodeID, msg.EpisodeNumber, msg.EpisodeTitle))
	return a, tea.Batch(cmds...)
}

// handlePlayerLaunchingMsg handles player launch initiation
func (a *App) handlePlayerLaunchingMsg(msg common.PlayerLaunchingMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Player launch initiated, transition to launching state
	a.previousState = a.state // Save current state to return to on cancel
	a.state = launchingPlayerView
	a.launchStartTime = time.Now()
	// Start ticker to check for launch completion or timeout
	cmds = append(cmds, a.spinner.Tick, a.checkPlayerLaunchStatus())
	return a, tea.Batch(cmds...)
}

// handlePlayerLaunchTimeoutCheckMsg handles player launch timeout checks
func (a *App) handlePlayerLaunchTimeoutCheckMsg(msg common.PlayerLaunchTimeoutCheckMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Check if player has launched successfully or timed out
	if a.state != launchingPlayerView {
		// Not in launching state anymore, stop checking
		return a, nil
	}

	// Check if player is now in playing state
	if a.player.IsPlaying() {
		// Success! Transition to playing view
		a.state = playingView
		cmds = append(cmds, a.monitorPlayback())
		return a, tea.Batch(cmds...)
	}

	// Check for timeout (15 seconds)
	elapsed := time.Since(a.launchStartTime)
	if elapsed > 15*time.Second {
		// Timeout - player failed to launch
		if a.player != nil {
			_ = a.player.Stop(context.Background())
		}
		a.err = fmt.Errorf("player launch timeout: mpv did not start within 15 seconds\n\n" +
			"Possible causes:\n" +
			"- mpv.exe is not installed or not in PATH\n" +
			"- mpv.exe lacks permissions to create named pipes (Windows)\n" +
			"- Antivirus blocking mpv.exe\n" +
			"- Missing video/stream headers (check provider implementation)")
		a.state = errorView
		return a, nil
	}

	// Still launching, check again in 200ms
	cmds = append(cmds, a.checkPlayerLaunchStatus())
	return a, tea.Batch(cmds...)
}

// handlePlaybackStartedMsg handles successful playback start
func (a *App) handlePlaybackStartedMsg(msg common.PlaybackStartedMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Playback started successfully, transition to playing view
	a.state = playingView
	// Record when playback started (for IPC initialization grace period)
	a.launchStartTime = time.Now()
	// Start monitoring playback
	cmds = append(cmds, a.monitorPlayback())
	return a, tea.Batch(cmds...)
}

// handlePlaybackEndedMsg handles playback completion
func (a *App) handlePlaybackEndedMsg(msg common.PlaybackEndedMsg) (*App, tea.Cmd) {
	// Playback ended, show completion message with progress info
	if a.player != nil {
		_ = a.player.Stop(context.Background())
	}

	// Build completion message with better formatting
	var lines []string

	// Title line
	if a.currentEpisodeNumber == 0 { // For movies
		lines = append(lines, fmt.Sprintf("✓ Playback completed: %s", a.selectedMedia.Title))
	} else { // For episodes
		lines = append(lines, fmt.Sprintf("✓ Playback completed: %s", a.selectedMedia.Title))
		lines = append(lines, fmt.Sprintf("   Episode %d", a.currentEpisodeNumber))
	}

	// Progress info
	if msg.WatchedPercentage > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Watched: %.1f%%", msg.WatchedPercentage))
		lines = append(lines, fmt.Sprintf("Duration: %s / %s", msg.WatchedDuration, msg.TotalDuration))

		// Show AniList sync status if watching from AniList
		if a.watchingFromAniList && a.currentAniListID > 0 {
			lines = append(lines, "")
			if msg.WatchedPercentage >= 85.0 {
				lines = append(lines, "✓ Syncing to AniList...")
			} else {
				lines = append(lines, "→ Saved locally (watch ≥85% to sync)")
			}
		}
	}

	// Help text - only offer to continue if episode was completed (>= 85%)
	lines = append(lines, "")
	episodeCompleted := msg.WatchedPercentage >= 85.0
	var autoReturn bool // Flag to determine if we should auto-return
	if a.watchingFromAniList && !a.isLastEpisode && episodeCompleted {
		// Episode completed and not the last episode - offer to continue
		lines = append(lines, styles.HelpStyle.Render("Continue watching next episode?"))
		lines = append(lines, styles.HelpStyle.Render("[y/Enter] Yes  [n/Esc] No, return to library"))
		autoReturn = false // Don't auto-return, wait for user choice
	} else if a.watchingFromAniList && a.isLastEpisode && episodeCompleted {
		// Last episode completed - auto-return after showing completion
		lines = append(lines, styles.HelpStyle.Render("That was the last episode!"))
		lines = append(lines, styles.HelpStyle.Render("Returning to library in 2 seconds..."))
		autoReturn = true
	} else {
		// Stopped early or not watching from AniList - auto-return
		lines = append(lines, styles.HelpStyle.Render("Press any key to return..."))
		autoReturn = true
	}

	// Join with newlines and add padding
	content := strings.Join(lines, "\n")

	// Create bordered box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(50).
		Align(lipgloss.Center)

	completionMsg := boxStyle.Render(content)

	// Store in a dedicated field instead of error
	a.playbackCompletionMsg = completionMsg
	a.state = playbackCompletedView
	a.episodeCompleted = episodeCompleted // Store for keypress handler

	// Store episode number for cursor positioning (before we clear it later)
	a.lastPlayedEpisodeNumber = a.currentEpisodeNumber

	// Don't clear episode state yet - we need it for the continue watching logic
	a.lastProgress = nil

	// If we should auto-return, start a timer
	if autoReturn {
		return a, a.autoReturnAfterDelay(500 * time.Millisecond)
	}

	return a, nil
}

// handlePlaybackErrorMsg handles playback errors
func (a *App) handlePlaybackErrorMsg(msg common.PlaybackErrorMsg) (*App, tea.Cmd) {
	if msg.Error != nil {
		a.err = msg.Error
		a.state = errorView
		return a, nil
	}
	return a, nil
}

// handlePlaybackAutoReturnMsg handles automatic return after playback
func (a *App) handlePlaybackAutoReturnMsg(msg common.PlaybackAutoReturnMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Auto-return from playback completion view
	// Only process if we're actually in the playback completed view
	if a.state != playbackCompletedView {
		return a, nil
	}

	// Clear the completion message
	a.playbackCompletionMsg = ""
	returnState := a.previousState
	a.previousState = -1

	// Clear episode state
	a.currentEpisodeID = ""
	a.currentSeasonNumber = 0
	a.currentEpisodeNumber = 0
	a.currentEpisodeTitle = ""

	episodeWasCompleted := a.episodeCompleted
	a.episodeCompleted = false

	// Clear any loading operation that might be lingering
	a.loadingOp = 0

	// If we were watching from AniList, refresh the library
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
		// Default to home view
		a.state = homeView
		// Trigger home view to reload recent history
		cmds = append(cmds, func() tea.Msg {
			return common.RefreshHistoryMsg{}
		})
	}

	return a, tea.Batch(cmds...)
}
func (a *App) playMovieDirectly(mediaID string) tea.Cmd {
	// Capture provider and mediaType BEFORE goroutine
	providerType := a.currentMediaType
	provider := a.providers[providerType]

	return func() tea.Msg {
		a.debugLog("playMovieDirectly goroutine executing with mediaID=%s", mediaID)
		a.debugLog("providerType=%s", providerType)

		if provider == nil {
			a.debugLog("ERROR: No provider found for mediaType=%s", providerType)
			return common.PlaybackErrorMsg{Error: fmt.Errorf("no provider available for %s", providerType)}
		}
		a.debugLog("Using provider: %s", provider.Name())

		// Track provider for this playback session
		a.currentPlaybackProvider = provider.Name()

		// This is a temporary solution to call the provider-specific method.
		// A better solution would be to have a more generic way to handle movies.
		type movieEpisodeIDGetter interface {
			GetMovieEpisodeID(ctx context.Context, mediaID string) (string, error)
		}

		episodeIDGetter, ok := provider.(movieEpisodeIDGetter)
		if !ok {
			a.debugLog("ERROR: Provider %s does not implement GetMovieEpisodeID", provider.Name())
			return common.PlaybackErrorMsg{Error: fmt.Errorf("provider does not support direct movie playback")}
		}

		a.debugLog("Calling GetMovieEpisodeID...")
		episodeID, err := episodeIDGetter.GetMovieEpisodeID(context.Background(), mediaID)
		if err != nil {
			a.debugLog("ERROR: GetMovieEpisodeID failed: %v", err)
			return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to get movie episode ID: %w", err)}
		}
		a.debugLog("Got episodeID=%s", episodeID)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		a.debugLog("Calling GetStreamURL for episodeID=%s", episodeID)
		stream, err := provider.GetStreamURL(ctx, episodeID, providers.Quality1080p)
		if err != nil {
			a.debugLog("ERROR: GetStreamURL failed: %v", err)
			return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to get stream URL: %w", err)}
		}
		a.debugLog("Got stream URL: %s", stream.URL)

		// Check if debug mode is enabled
		if a.isDebugMode() {
			a.debugLog("Debug mode enabled, storing info and quitting")
			a.debugInfo = &DebugInfo{
				MediaTitle:    a.selectedMedia.Title,
				EpisodeTitle:  a.selectedMedia.Title,
				EpisodeNumber: 0,
				StreamURL:     stream.URL,
				Quality:       stream.Quality,
				Type:          stream.Type,
				Referer:       stream.Referer,
				Headers:       stream.Headers,
				Subtitles:     stream.Subtitles,
				Error:         nil,
			}
			a.forceQuit = true
			return nil
		}

		if a.player == nil {
			return common.PlaybackErrorMsg{Error: fmt.Errorf("player not initialized")}
		}

		// Audio track selection for movies
		audioTrackIndex := 0 // Default to first track
		if len(stream.AudioTracks) > 0 {
			// Determine effective audio preference
			preference := a.audioPreference // From CLI flag or empty
			if preference == "" && a.currentAniListID != 0 {
				// Check database for per-show memory
				if dbPref, err := database.GetAudioPreference(a.db, a.currentAniListID); err == nil && dbPref != "" {
					preference = dbPref
				}
			}

			// Try to find matching track
			if selectedTrack := audio.SelectAudioTrack(stream.AudioTracks, preference); selectedTrack != nil {
				audioTrackIndex = selectedTrack.Index
			}
		}

		options := player.PlayOptions{
			Title:      a.selectedMedia.Title,
			Episode:    0, // 0 for movie
			Headers:    stream.Headers,
			Referer:    stream.Referer,
			AudioTrack: audioTrackIndex,
		}

		// Add subtitle if available, preferring English
		if subtitle := selectBestSubtitle(stream.Subtitles); subtitle != nil {
			options.SubtitleURL = subtitle.URL
			options.SubtitleLang = "en,eng,english"
		}

		if err := a.player.Play(context.Background(), stream.URL, options); err != nil {
			return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to start playback: %w", err)}
		}

		a.currentEpisodeID = episodeID
		a.currentEpisodeNumber = 0
		a.currentSeasonNumber = 0 // Movies don't have seasons
		a.currentEpisodeTitle = a.selectedMedia.Title

		return common.PlaybackStartedMsg{}
	}
}

func (a *App) startPlayback(episodeID string, episodeNumber int, episodeTitle string) tea.Cmd {
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
				return common.PlaybackErrorMsg{Error: fmt.Errorf("no provider available")}
			}
		}

		// Track provider for this playback session
		a.currentPlaybackProvider = provider.Name()

		// Get stream URL for the episode/movie
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		stream, err := provider.GetStreamURL(ctx, episodeID, providers.Quality1080p)
		if err != nil {
			return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to get stream URL: %w", err)}
		}

		// Check if debug mode is enabled
		if a.isDebugMode() {
			a.debugInfo = &DebugInfo{
				MediaTitle:    a.selectedMedia.Title,
				EpisodeTitle:  episodeTitle,
				EpisodeNumber: episodeNumber,
				StreamURL:     stream.URL,
				Quality:       stream.Quality,
				Type:          stream.Type,
				Referer:       stream.Referer,
				Headers:       stream.Headers,
				Subtitles:     stream.Subtitles,
				Error:         nil,
			}
			a.forceQuit = true
			return nil
		}

		// Check if player is initialized
		if a.player == nil {
			return common.PlaybackErrorMsg{Error: fmt.Errorf("player not initialized")}
		}

		// Prepare playback options
		var title string
		if episodeNumber == 0 { // For movies played directly
			title = a.selectedMedia.Title
		} else { // For episodes and other content
			title = fmt.Sprintf("%s - Episode %d", a.selectedMedia.Title, episodeNumber)
		}

		// Audio track selection (CLI > DB > config hierarchy)
		audioTrackIndex := 0 // Default to first track
		if len(stream.AudioTracks) > 0 {
			// Determine effective audio preference
			preference := a.audioPreference // From CLI flag or empty
			if preference == "" && a.currentAniListID != 0 {
				// Check database for per-show memory
				if dbPref, err := database.GetAudioPreference(a.db, a.currentAniListID); err == nil && dbPref != "" {
					preference = dbPref
				}
			}
			// If still empty, will use config default (already in a.audioPreference from CLI init)

			// Try to find matching track
			if selectedTrack := audio.SelectAudioTrack(stream.AudioTracks, preference); selectedTrack != nil {
				audioTrackIndex = selectedTrack.Index
			} else {
				// No matching track found - show audio selector TUI
				return common.ShowAudioSelectorMsg{
					Tracks:       stream.AudioTracks,
					Stream:       stream,
					AniListID:    a.currentAniListID,
					EpisodeID:    episodeID,
					EpisodeNum:   episodeNumber,
					EpisodeTitle: episodeTitle,
				}
			}
		}

		options := player.PlayOptions{
			Title:      title,
			Episode:    episodeNumber,
			Headers:    stream.Headers,
			Referer:    stream.Referer,
			AudioTrack: audioTrackIndex,
		}

		// Check for resume position if watching from AniList
		if a.watchingFromAniList && a.currentAniListID > 0 {
			if resumeSeconds, err := a.checkResumePosition(a.currentAniListID, episodeNumber); err == nil && resumeSeconds > 0 {
				options.StartTime = time.Duration(resumeSeconds) * time.Second
				// Note: User will see resume position when playback starts
			}
		}

		// Add subtitle if available, preferring English
		if subtitle := selectBestSubtitle(stream.Subtitles); subtitle != nil {
			options.SubtitleURL = subtitle.URL
			options.SubtitleLang = "en,eng,english"
		}

		// Play the stream with MPV (now async - returns immediately)
		if err := a.player.Play(context.Background(), stream.URL, options); err != nil {
			return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to start playback: %w", err)}
		}

		// Player launch initiated, transition to launching state
		return common.PlayerLaunchingMsg{}
	}
}

// continuePlaybackWithAudioTrack continues playback after audio track selection
func (a *App) continuePlaybackWithAudioTrack(audioTrackIndex int) tea.Cmd {
	return func() tea.Msg {
		// Check if we have a pending stream
		if a.pendingStream == nil {
			return common.PlaybackErrorMsg{Error: fmt.Errorf("no pending stream for audio track selection")}
		}

		stream := a.pendingStream
		a.pendingStream = nil // Clear pending stream

		// Check if player is initialized
		if a.player == nil {
			return common.PlaybackErrorMsg{Error: fmt.Errorf("player not initialized")}
		}

		// Prepare playback options
		var title string
		if a.currentEpisodeNumber == 0 { // For movies played directly
			title = a.selectedMedia.Title
		} else { // For episodes and other content
			title = fmt.Sprintf("%s - Episode %d", a.selectedMedia.Title, a.currentEpisodeNumber)
		}

		options := player.PlayOptions{
			Title:      title,
			Episode:    a.currentEpisodeNumber,
			Headers:    stream.Headers,
			Referer:    stream.Referer,
			AudioTrack: audioTrackIndex,
		}

		// Check for resume position if watching from AniList
		if a.watchingFromAniList && a.currentAniListID > 0 {
			if resumeSeconds, err := a.checkResumePosition(a.currentAniListID, a.currentEpisodeNumber); err == nil && resumeSeconds > 0 {
				options.StartTime = time.Duration(resumeSeconds) * time.Second
			}
		}

		// Add subtitle if available, preferring English
		if subtitle := selectBestSubtitle(stream.Subtitles); subtitle != nil {
			options.SubtitleURL = subtitle.URL
			options.SubtitleLang = "en,eng,english"
		}

		// Play the stream with MPV (now async - returns immediately)
		if err := a.player.Play(context.Background(), stream.URL, options); err != nil {
			return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to start playback: %w", err)}
		}

		// Player launch initiated, transition to launching state
		return common.PlayerLaunchingMsg{}
	}
}

// resumePlaybackFromHistory resumes playback from history with stored progress
func (a *App) resumePlaybackFromHistory(msg common.ResumePlaybackMsg) tea.Cmd {
	return func() tea.Msg {
		// Update current media type from message if available
		if msg.MediaType != "" {
			switch msg.MediaType {
			case "anime":
				a.currentMediaType = providers.MediaTypeAnime
			case "movie":
				a.currentMediaType = providers.MediaTypeMovie
			case "tv":
				a.currentMediaType = providers.MediaTypeTV
			case "manga":
				a.currentMediaType = providers.MediaTypeManga
			}
		} else {
			// Try to infer from provider name if possible
			if msg.ProviderName != "" {
				for _, p := range a.providers {
					if p.Name() == msg.ProviderName {
						if p.Type() == providers.MediaTypeManga {
							a.currentMediaType = providers.MediaTypeManga
							msg.MediaType = "manga" // Set it for later check
						}
						break
					}
				}
			}
		}

		// Get the provider (try to use stored provider name if available)
		var provider providers.Provider
		var ok bool

		// Try to find provider by name if stored
		if msg.ProviderName != "" {
			for _, p := range a.providers {
				if p.Name() == msg.ProviderName {
					provider = p
					ok = true
					break
				}
			}
		}

		// Fall back to current media type provider
		if !ok {
			provider, ok = a.providers[a.currentMediaType]
			if !ok {
				// Try any available provider
				for _, p := range a.providers {
					provider = p
					break
				}
				if provider == nil {
					return common.PlaybackErrorMsg{Error: fmt.Errorf("no provider available")}
				}
			}
		}

		// Track provider for this playback session
		a.currentPlaybackProvider = provider.Name()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Check if this is an AniList media ID that needs mapping
		actualMediaID := msg.MediaID
		isAniListMedia := strings.HasPrefix(msg.MediaID, "anilist:")
		var anilistID int

		if isAniListMedia {
			// Extract AniList ID and look up provider mapping
			anilistID = extractAniListID(msg.MediaID)
			if anilistID == 0 {
				return common.PlaybackErrorMsg{Error: fmt.Errorf("invalid AniList ID: %s", msg.MediaID)}
			}

			// Set AniList context for progress tracking
			a.watchingFromAniList = true
			a.currentAniListID = anilistID

			// Try to look up the provider mapping to get the media ID
			var mappingFound bool
			if a.mappingMgr != nil {
				if mgr, ok := a.mappingMgr.(*mapping.Manager); ok {
					providerMapping, err := mgr.GetMapping(ctx, anilistID)
					if err == nil && providerMapping != nil {
						// Mapping exists
						mappingFound = true

						// If we have a provider from history, verify it matches
						// If not, we'll need to search with the history provider
						if msg.ProviderName != "" && providerMapping.ProviderName != msg.ProviderName {
							// Provider mismatch - the user watched with a different provider than mapped
							// We'll search for the anime with the provider from history
							a.logger.Debug("Provider mismatch: history=%s, mapping=%s. Searching with history provider...\n",
								msg.ProviderName, providerMapping.ProviderName)

							// Search for the anime using the provider from history
							searchResults, err := provider.Search(ctx, msg.MediaTitle)
							if err != nil || len(searchResults) == 0 {
								return common.PlaybackErrorMsg{Error: fmt.Errorf("could not find '%s' using %s provider. Please play from your AniList library to update the mapping", msg.MediaTitle, provider.Name())}
							}

							// Use the first result (best match)
							actualMediaID = searchResults[0].ID
							a.selectedMedia = searchResults[0]
						} else {
							// Provider matches or no provider in history - use the mapping
							actualMediaID = providerMapping.ProviderMediaID
							a.selectedMedia = providers.Media{
								ID:    actualMediaID,
								Title: msg.MediaTitle,
								Type:  provider.Type(),
							}
						}
					}
				}
			}

			// If no mapping was found, search with current provider
			if !mappingFound {
				a.logger.Debug("no mapping found, searching with provider", "anilist_id", anilistID, "provider", provider.Name())

				searchResults, err := provider.Search(ctx, msg.MediaTitle)
				if err != nil || len(searchResults) == 0 {
					return common.PlaybackErrorMsg{Error: fmt.Errorf("no provider mapping found for '%s'. Please play it from your AniList library to create a mapping with %s", msg.MediaTitle, provider.Name())}
				}

				// Use the first result (best match)
				actualMediaID = searchResults[0].ID
				a.selectedMedia = searchResults[0]
			}
		} else {
			// Non-AniList content - verify the media ID is still valid by trying a quick lookup
			// If it fails, fall back to searching by title
			a.selectedMedia = providers.Media{
				ID:    actualMediaID,
				Title: msg.MediaTitle,
				Type:  provider.Type(),
			}
		}

		// For movies (episode 0), we need to get the movie episode ID first
		if msg.Episode == 0 {
			// Check if provider supports GetMovieEpisodeID interface
			type movieEpisodeIDGetter interface {
				GetMovieEpisodeID(ctx context.Context, mediaID string) (string, error)
			}

			episodeIDGetter, hasMovieMethod := provider.(movieEpisodeIDGetter)
			var movieEpisodeID string

			if hasMovieMethod {
				// Provider supports GetMovieEpisodeID - use it
				a.logger.Debug("Getting movie episode ID", "media_id", actualMediaID)
				var err error
				movieEpisodeID, err = episodeIDGetter.GetMovieEpisodeID(ctx, actualMediaID)
				if err != nil {
					// If getting movie episode ID fails, try to search for the media by title as fallback
					a.logger.Debug("Failed to get movie episode ID, searching by title", "media_id", actualMediaID, "error", err)

					searchResults, searchErr := provider.Search(ctx, msg.MediaTitle)
					if searchErr == nil && len(searchResults) > 0 {
						// Update to use the first search result
						actualMediaID = searchResults[0].ID
						a.selectedMedia = searchResults[0]

						// Try again with the new media ID
						movieEpisodeID, err = episodeIDGetter.GetMovieEpisodeID(ctx, actualMediaID)
					}

					if err != nil {
						return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to get movie episode ID: %w", err)}
					}
				}
			} else {
				// Provider doesn't have GetMovieEpisodeID - use media ID directly
				a.logger.Debug("Provider doesn't support GetMovieEpisodeID", "media_id", actualMediaID)
				movieEpisodeID = actualMediaID
			}

			// Now get the stream URL using the episode ID
			stream, err := provider.GetStreamURL(ctx, movieEpisodeID, providers.Quality1080p)
			if err != nil {
				// If getting stream fails and we haven't tried searching yet, try that
				if hasMovieMethod && movieEpisodeID == actualMediaID {
					a.logger.Debug("Failed to get stream for media ID %s: %v. Attempting to search by title...", actualMediaID, err)

					searchResults, searchErr := provider.Search(ctx, msg.MediaTitle)
					if searchErr == nil && len(searchResults) > 0 {
						// Update to use the first search result
						actualMediaID = searchResults[0].ID
						a.selectedMedia = searchResults[0]

						// Try to get episode ID and stream with new media ID
						movieEpisodeID, err = episodeIDGetter.GetMovieEpisodeID(ctx, actualMediaID)
						if err == nil {
							stream, err = provider.GetStreamURL(ctx, movieEpisodeID, providers.Quality1080p)
						}
					}
				}

				if err != nil {
					return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to get stream URL: %w", err)}
				}
			}

			// If this is AniList content, fetch the full media details for proper tracking
			if isAniListMedia {
				a.currentAniListMedia = &tracker.TrackedMedia{
					ServiceID: fmt.Sprintf("anilist:%d", anilistID),
					Title:     msg.MediaTitle,
					Type:      providers.MediaTypeAnime,
				}
			}

			// Set current episode info for tracking
			a.currentEpisodeID = actualMediaID
			a.currentEpisodeNumber = 0
			a.currentSeasonNumber = msg.Season // Should be 0 for movies
			a.currentEpisodeTitle = msg.MediaTitle

			// Audio track selection for history movie playback
			audioTrackIndex := 0
			if len(stream.AudioTracks) > 0 {
				preference := a.audioPreference
				if preference == "" && anilistID != 0 {
					if dbPref, err := database.GetAudioPreference(a.db, anilistID); err == nil && dbPref != "" {
						preference = dbPref
					}
				}
				if selectedTrack := audio.SelectAudioTrack(stream.AudioTracks, preference); selectedTrack != nil {
					audioTrackIndex = selectedTrack.Index
				}
			}

			// Build proper play options with title, headers, subtitles
			playOpts := player.PlayOptions{
				Title:      msg.MediaTitle,
				Episode:    0,
				StartTime:  time.Duration(msg.ProgressSeconds) * time.Second,
				Headers:    stream.Headers,
				Referer:    stream.Referer,
				AudioTrack: audioTrackIndex,
			}

			// Add subtitle if available, preferring English
			if subtitle := selectBestSubtitle(stream.Subtitles); subtitle != nil {
				playOpts.SubtitleURL = subtitle.URL
				playOpts.SubtitleLang = "en,eng,english"
			}

			if err := a.player.Play(context.Background(), stream.URL, playOpts); err != nil {
				return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to start player: %w", err)}
			}

			return common.PlayerLaunchingMsg{}
		}

		// For TV/Anime, we need to get episodes
		// Try to determine if this is single-season or multi-season content
		var episodeID string
		var episodes []providers.Episode
		var err error

		// Try to get seasons first
		seasons, seasonsErr := provider.GetSeasons(ctx, actualMediaID)

		// If getting seasons fails, try searching by title as fallback
		if seasonsErr != nil {
			a.logger.Debug("Failed to get seasons for media ID %s: %v. Attempting to search by title...", actualMediaID, seasonsErr)

			searchResults, searchErr := provider.Search(ctx, msg.MediaTitle)
			if searchErr == nil && len(searchResults) > 0 {
				// Update to use the first search result
				actualMediaID = searchResults[0].ID
				a.selectedMedia = searchResults[0]

				// Try again with the new media ID
				seasons, seasonsErr = provider.GetSeasons(ctx, actualMediaID)
			}
		}

		// If getting seasons fails or returns empty/single season, treat as single-season content
		if seasonsErr != nil || len(seasons) == 0 || (len(seasons) == 1 && seasons[0].ID == actualMediaID) {
			// Single season content - episodes directly from media ID
			episodes, err = provider.GetEpisodes(ctx, actualMediaID)
			if err != nil {
				return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to get episodes: %w", err)}
			}
		} else {
			// Multi-season content
			var seasonID string

			// If we have a specific season number from history, try to find it
			if msg.Season > 0 {
				for _, season := range seasons {
					if season.Number == msg.Season {
						seasonID = season.ID
						break
					}
				}
			}

			// If we didn't find the season by number, try to use the first season
			// or if only one season exists
			if seasonID == "" {
				if len(seasons) == 1 {
					seasonID = seasons[0].ID
				} else {
					return common.PlaybackErrorMsg{Error: fmt.Errorf("could not determine correct season (requested season %d, found %d seasons)", msg.Season, len(seasons))}
				}
			}

			// Get episodes for the season
			episodes, err = provider.GetEpisodes(ctx, seasonID)
			if err != nil {
				// If getting episodes by season ID fails, fall back to direct media ID
				episodes, err = provider.GetEpisodes(ctx, actualMediaID)
				if err != nil {
					return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to get episodes: %w", err)}
				}
			}
		}

		// Store episodes so we can return to the episode list after playback
		a.episodes = episodes

		// Find the episode by number
		for _, ep := range episodes {
			if ep.Number == msg.Episode {
				episodeID = ep.ID
				break
			}
		}

		if episodeID == "" {
			return common.PlaybackErrorMsg{Error: fmt.Errorf("episode %d not found", msg.Episode)}
		}

		// Handle Manga
		if msg.MediaType == "manga" {
			mangaProvider, ok := provider.(providers.MangaProvider)
			if !ok {
				return common.PlaybackErrorMsg{Error: fmt.Errorf("provider does not support manga")}
			}

			// Set current episode info
			a.currentEpisodeID = episodeID
			a.currentEpisodeNumber = msg.Episode
			a.currentSeasonNumber = msg.Season
			a.currentEpisodeTitle = msg.MediaTitle // Or fetch chapter title if possible

			pages, err := mangaProvider.GetMangaPages(ctx, episodeID)
			if err != nil {
				return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to get manga pages: %w", err)}
			}

			return common.MangaPagesLoadedMsg{
				Pages: pages,
			}
		}

		// Get stream URL
		stream, err := provider.GetStreamURL(ctx, episodeID, providers.Quality1080p)
		if err != nil {
			return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to get stream URL: %w", err)}
		}

		// If this is AniList content, fetch the full media details for proper tracking
		if isAniListMedia {
			a.currentAniListMedia = &tracker.TrackedMedia{
				ServiceID: fmt.Sprintf("anilist:%d", anilistID),
				Title:     msg.MediaTitle,
				Type:      providers.MediaTypeAnime,
			}
		}

		// Set current episode info for tracking (before playback starts)
		a.currentEpisodeID = episodeID
		a.currentEpisodeNumber = msg.Episode
		a.currentSeasonNumber = msg.Season
		a.currentEpisodeTitle = msg.MediaTitle

		// Audio track selection for history episode playback
		audioTrackIndex := 0
		if len(stream.AudioTracks) > 0 {
			preference := a.audioPreference
			if preference == "" && anilistID != 0 {
				if dbPref, err := database.GetAudioPreference(a.db, anilistID); err == nil && dbPref != "" {
					preference = dbPref
				}
			}
			if selectedTrack := audio.SelectAudioTrack(stream.AudioTracks, preference); selectedTrack != nil {
				audioTrackIndex = selectedTrack.Index
			}
		}

		// Build proper play options with title, headers, subtitles
		title := fmt.Sprintf("%s - Episode %d", msg.MediaTitle, msg.Episode)

		playOpts := player.PlayOptions{
			Title:      title,
			Episode:    msg.Episode,
			StartTime:  time.Duration(msg.ProgressSeconds) * time.Second,
			Headers:    stream.Headers,
			Referer:    stream.Referer,
			AudioTrack: audioTrackIndex,
		}

		// Add subtitle if available, preferring English
		if subtitle := selectBestSubtitle(stream.Subtitles); subtitle != nil {
			playOpts.SubtitleURL = subtitle.URL
			playOpts.SubtitleLang = "en,eng,english"
		}

		if err := a.player.Play(context.Background(), stream.URL, playOpts); err != nil {
			return common.PlaybackErrorMsg{Error: fmt.Errorf("failed to start player: %w", err)}
		}

		return common.PlayerLaunchingMsg{}
	}
}
