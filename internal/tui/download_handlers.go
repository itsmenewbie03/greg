package tui

// This file contains download-related methods extracted from model.go
// for better code organization. All methods remain on the App struct.

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/justchokingaround/greg/internal/config"
	"github.com/justchokingaround/greg/internal/downloader"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/components/mangadownload"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

// resolveAndDownloadMovie resolves a movie's episode ID and starts the download
func (a *App) resolveAndDownloadMovie(mediaID string, title string) tea.Cmd {
	return func() tea.Msg {
		provider := a.providers[a.currentMediaType]
		if provider == nil {
			return nil // Should log error
		}

		// Try to get episode ID using GetMovieEpisodeID interface
		type movieEpisodeIDGetter interface {
			GetMovieEpisodeID(ctx context.Context, mediaID string) (string, error)
		}

		var episodeID string
		if episodeIDGetter, ok := provider.(movieEpisodeIDGetter); ok {
			id, err := episodeIDGetter.GetMovieEpisodeID(context.Background(), mediaID)
			if err == nil {
				episodeID = id
			}
		}

		// If not found via interface, try getting seasons/episodes as fallback
		if episodeID == "" {
			// Try getting seasons
			seasons, err := provider.GetSeasons(context.Background(), mediaID)
			if err == nil && len(seasons) > 0 {
				// Get episodes for first season
				episodes, err := provider.GetEpisodes(context.Background(), seasons[0].ID)
				if err == nil && len(episodes) > 0 {
					episodeID = episodes[0].ID
				}
			}
		}

		// If still not found, try GetMediaDetails
		if episodeID == "" {
			details, err := provider.GetMediaDetails(context.Background(), mediaID)
			if err == nil && len(details.Seasons) > 0 {
				episodes, err := provider.GetEpisodes(context.Background(), details.Seasons[0].ID)
				if err == nil && len(episodes) > 0 {
					episodeID = episodes[0].ID
				}
			}
		}

		if episodeID == "" {
			// Failed to resolve
			return common.DownloadAddedMsg{
				Title:    title,
				Episode:  0,
				Quality:  "Error",
				Location: "Could not resolve movie ID",
			}
		}

		// Now start the download
		// We can reuse startDownload logic but we need to return a Cmd that calls it
		// Since startDownload returns a Cmd, we can't call it directly inside a Cmd function easily without wrapping
		// But we can just return the result of startDownload here if we adapt it,
		// or better, send a message to trigger it.
		// But startDownload is a method on App that returns a Cmd.
		// We can just execute the logic here directly.

		// Get stream URL
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		stream, err := provider.GetStreamURL(ctx, episodeID, providers.Quality1080p)
		if err != nil {
			return nil
		}

		// Create download task
		task := downloader.DownloadTask{
			MediaID:    mediaID,
			MediaTitle: title,
			MediaType:  providers.MediaTypeMovie,
			Episode:    0, // Movies don't have episodes
			Season:     0,
			Quality:    providers.Quality1080p,
			Provider:   provider.Name(),
			StreamURL:  stream.URL,
			StreamType: stream.Type,
			Headers:    stream.Headers,
			Referer:    stream.Referer,
			Subtitles:  stream.Subtitles,
			EmbedSubs:  true,
		}

		// Add to download queue
		if err := a.downloadMgr.AddToQueue(ctx, task); err != nil {
			return nil
		}

		// Get output path
		queue, err := a.downloadMgr.GetQueue(ctx)
		var outputPath string
		if err == nil && len(queue) > 0 {
			for i := len(queue) - 1; i >= 0; i-- {
				if queue[i].MediaID == mediaID {
					outputPath = queue[i].OutputPath
					break
				}
			}
		}

		return common.DownloadAddedMsg{
			Title:    title,
			Episode:  0, // 0 to suppress "Episode 1" in notification
			Quality:  string(providers.Quality1080p),
			Location: outputPath,
		}
	}
}

// startDownload starts a download for an episode
func (a *App) startDownload(episodeID string, episodeNumber int, episodeTitle string) tea.Cmd {
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
				a.logger.Error("no provider available for download")
				return nil
			}
		}

		// Get stream URL for the episode/movie
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		stream, err := provider.GetStreamURL(ctx, episodeID, providers.Quality1080p)
		if err != nil {
			a.logger.Error("failed to get stream URL for download", "error", err)
			return nil
		}

		// Log subtitle info for debugging
		a.logger.Info("download stream info",
			"episode", episodeNumber,
			"subtitles_count", len(stream.Subtitles),
			"embed_subs", true)

		// Create download task
		task := downloader.DownloadTask{
			MediaID:    a.selectedMedia.ID,
			MediaTitle: a.selectedMedia.Title,
			MediaType:  a.selectedMedia.Type,
			Episode:    episodeNumber,
			Season:     0, // TODO: Add season support
			Quality:    providers.Quality1080p,
			Provider:   provider.Name(),
			StreamURL:  stream.URL,
			StreamType: stream.Type,
			Headers:    stream.Headers,
			Referer:    stream.Referer,
			Subtitles:  stream.Subtitles,
			EmbedSubs:  true, // Will use config default
		}

		// Add to download queue
		if err := a.downloadMgr.AddToQueue(ctx, task); err != nil {
			a.logger.Error("failed to add download to queue", "error", err)
			return nil
		}

		// Get the queue to find the output path (manager sets it)
		queue, err := a.downloadMgr.GetQueue(ctx)
		var outputPath string
		if err == nil && len(queue) > 0 {
			// Find the task we just added (it's the last one)
			for i := len(queue) - 1; i >= 0; i-- {
				if queue[i].MediaID == a.selectedMedia.ID && queue[i].Episode == episodeNumber {
					outputPath = queue[i].OutputPath
					break
				}
			}
		}

		a.logger.Info("added to download queue", "title", a.selectedMedia.Title, "episode", episodeNumber)
		a.logger.Info("download output path", "path", outputPath)

		// Send notification message
		return common.DownloadAddedMsg{
			Title:    a.selectedMedia.Title,
			Episode:  episodeNumber,
			Quality:  string(providers.Quality1080p),
			Location: outputPath,
		}
	}
}

// downloadMangaChapters downloads manga chapters and adds them to the download queue
func (a *App) downloadMangaChapters(provider providers.MangaProvider, episodes []common.EpisodeInfo) {
	ctx := context.Background()

	for _, ep := range episodes {
		// Update UI with current chapter
		a.msgChan <- mangadownload.ProgressUpdateMsg{
			ChapterName: ep.Title,
			Operation:   "Fetching pages...",
		}

		// Get manga pages
		pages, err := provider.GetMangaPages(ctx, ep.EpisodeID)
		if err != nil {
			a.logger.Error("failed to get manga pages", "chapter", ep.Title, "error", err)
			a.msgChan <- mangadownload.ChapterFailedMsg{
				ChapterName: ep.Title,
				Error:       err,
			}
			continue
		}

		// Update operation
		a.msgChan <- mangadownload.ProgressUpdateMsg{
			ChapterName: ep.Title,
			Operation:   fmt.Sprintf("Downloading %d pages...", len(pages)),
		}

		// Generate output path
		sanitizedTitle := sanitizeFilename(a.selectedMedia.Title)
		filename := fmt.Sprintf("%s - Chapter %d.cbz", sanitizedTitle, ep.Number)
		outputPath := filepath.Join(a.getDownloadPath(), "manga", sanitizedTitle, filename)

		// Create download task
		task := &downloader.MangaDownloadTask{
			ID:           fmt.Sprintf("manga-%d-%s", time.Now().Unix(), ep.EpisodeID),
			MediaID:      a.selectedMedia.ID,
			MediaTitle:   a.selectedMedia.Title,
			ChapterID:    ep.EpisodeID,
			ChapterTitle: ep.Title,
			ChapterNum:   ep.Number,
			Provider:     provider.Name(),
			Pages:        pages,
			Format:       downloader.FormatCBZ,
			OutputPath:   outputPath,
		}

		// Download the chapter directly (synchronously for progress feedback)
		if a.downloadMgr != nil {
			err = a.downloadMgr.DownloadMangaChapter(ctx, provider, task)
		} else {
			err = fmt.Errorf("download manager not initialized")
		}

		if err != nil {
			a.logger.Error("failed to download manga chapter", "chapter", ep.Title, "error", err)
			a.msgChan <- mangadownload.ChapterFailedMsg{
				ChapterName: ep.Title,
				Error:       err,
			}
		} else {
			a.logger.Info("manga chapter downloaded", "chapter", ep.Title, "path", task.OutputPath)

			// Add to download manager queue for tracking (mark as completed)
			task.Status = downloader.StatusCompleted
			now := time.Now()
			task.StartedAt = &now
			task.CompletedAt = &now
			task.Progress = 100.0

			if addErr := a.downloadMgr.AddMangaToQueue(ctx, provider, task); addErr != nil {
				a.logger.Warn("failed to add completed manga to queue", "error", addErr)
			}

			a.msgChan <- mangadownload.ChapterCompleteMsg{
				ChapterName: ep.Title,
				FilePath:    task.OutputPath,
			}
		}
	}
}

// sanitizeFilename removes invalid characters from filenames
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "",
		"?", "",
		"\"", "",
		"<", "",
		">", "",
		"|", "",
	)
	return replacer.Replace(name)
}

// getDownloadPath returns the download path from config or default
func (a *App) getDownloadPath() string {
	if a.cfg != nil {
		if appCfg, ok := a.cfg.(*config.Config); ok {
			return appCfg.Downloads.Path
		}
	}
	// Default to current directory
	return "."
}

// renderDownloadNotification renders a pretty download notification popup
func (a *App) renderDownloadNotification() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonGreen).
		Bold(true).
		Align(lipgloss.Center)

	messageStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonBase06).
		Align(lipgloss.Center).
		MarginTop(1)

	hintStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonBase04).
		Italic(true).
		Align(lipgloss.Center).
		MarginTop(1)

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		titleStyle.Render("Download Started"),
		messageStyle.Render(a.downloadNotificationMsg),
		hintStyle.Render("(Auto-dismissing in 3s or press any key)"),
	)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OxocarbonGreen).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Center)

	return boxStyle.Render(content)
}

// Message handlers

// handleMediaDownloadMsg handles direct download request for movies
func (a *App) handleMediaDownloadMsg(msg common.MediaDownloadMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Handle direct download request for movies
	if a.downloadMgr != nil {
		// We need to resolve the episode ID first
		a.selectedMedia = providers.Media{
			ID:    msg.MediaID,
			Title: msg.Title,
			Type:  providers.MediaType(msg.Type),
		}
		a.currentMediaType = a.selectedMedia.Type

		// Start resolution and download
		cmds = append(cmds, a.resolveAndDownloadMovie(msg.MediaID, msg.Title))
	}
	return a, tea.Batch(cmds...)
}

// handleEpisodeDownloadMsg handles download request for an episode
func (a *App) handleEpisodeDownloadMsg(msg common.EpisodeDownloadMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Handle download request
	if a.downloadMgr != nil {
		// Start download in background
		cmds = append(cmds, a.startDownload(msg.EpisodeID, msg.Number, msg.Title))
	}
	return a, tea.Batch(cmds...)
}

// handleBatchDownloadMsg handles batch download request
func (a *App) handleBatchDownloadMsg(msg common.BatchDownloadMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd
	// Handle batch download request
	if a.currentMediaType == providers.MediaTypeManga && len(msg.Episodes) > 0 {
		// Manga batch download - show progress UI
		a.mangaDownloadComponent.SetTotal(len(msg.Episodes))
		a.state = mangaDownloadProgressView

		// Start downloads in background
		provider, ok := a.providers[a.currentMediaType]
		if ok {
			if mangaProvider, ok := provider.(providers.MangaProvider); ok {
				// Start downloading each chapter
				go a.downloadMangaChapters(mangaProvider, msg.Episodes)
			}
		}

		return a, a.mangaDownloadComponent.Init()
	} else if a.downloadMgr != nil {
		// Video downloads - queue them asynchronously
		// Don't create individual commands - they timeout when batched
		go func() {
			episodeList := msg.Episodes
			successCount := 0
			a.logger.Info("batch download started", "count", len(episodeList))

			for i, ep := range episodeList {
				a.logger.Info("processing episode for download", "index", i+1, "episode", ep.Number, "id", ep.EpisodeID)

				// Get provider
				provider, ok := a.providers[a.currentMediaType]
				if !ok {
					a.logger.Error("no provider available", "episode", ep.Number)
					continue
				}

				// Get stream URL with retry
				var stream *providers.StreamURL
				var err error
				for attempt := 1; attempt <= 2; attempt++ {
					ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
					stream, err = provider.GetStreamURL(ctx, ep.EpisodeID, providers.Quality1080p)
					cancel()

					if err == nil {
						break
					}

					a.logger.Warn("stream URL attempt failed", "episode", ep.Number, "attempt", attempt, "error", err)
					if attempt < 2 {
						time.Sleep(2 * time.Second)
					}
				}

				if err != nil {
					a.logger.Error("failed to get stream URL after retries", "episode", ep.Number, "error", err)
					// Continue with next episode anyway - don't stop batch
					continue
				}

				a.logger.Info("got stream URL", "episode", ep.Number, "url_length", len(stream.URL))

				// Create download task
				task := downloader.DownloadTask{
					MediaID:    a.selectedMedia.ID,
					MediaTitle: a.selectedMedia.Title,
					MediaType:  a.selectedMedia.Type,
					Episode:    ep.Number,
					Season:     0,
					Quality:    providers.Quality1080p,
					Provider:   provider.Name(),
					StreamURL:  stream.URL,
					StreamType: stream.Type,
					Headers:    stream.Headers,
					Referer:    stream.Referer,
					Subtitles:  stream.Subtitles,
					EmbedSubs:  true,
				}

				// Add to queue
				if err := a.downloadMgr.AddToQueue(context.Background(), task); err != nil {
					a.logger.Error("failed to add to download queue", "episode", ep.Number, "error", err)
					// Check if it's a duplicate
					if strings.Contains(err.Error(), "already in queue") {
						a.logger.Info("skipping duplicate episode", "episode", ep.Number)
					}
				} else {
					a.logger.Info("added to download queue", "episode", ep.Number)
					successCount++
				}

				// Small delay between episodes to avoid overwhelming the provider
				if i < len(episodeList)-1 {
					time.Sleep(500 * time.Millisecond)
				}
			}

			a.logger.Info("batch download queuing complete", "total_attempted", len(episodeList), "total_added", successCount, "total_skipped", len(episodeList)-successCount)
		}()

		// Switch to downloads view immediately
		cmds = append(cmds, func() tea.Msg {
			return common.GoToDownloadsMsg{}
		})
	}
	return a, tea.Batch(cmds...)
}

// handleDownloadAddedMsg handles download added notification
func (a *App) handleDownloadAddedMsg(msg common.DownloadAddedMsg) (*App, tea.Cmd) {
	// Silently add download - no notification needed
	// User can see progress in downloads view
	return a, nil
}
