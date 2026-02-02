package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justchokingaround/greg/internal/config"
	"github.com/justchokingaround/greg/internal/database"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tracker/mapping"
	"github.com/justchokingaround/greg/internal/tui/common"
)

// handlePerformSearchMsg handles search requests
func (a *App) handlePerformSearchMsg(msg common.PerformSearchMsg) (*App, tea.Cmd) {
	a.state = loadingView
	a.loadingOp = loadingSearch
	var cmds []tea.Cmd
	cmds = append(cmds, a.spinner.Tick, a.performSearch(msg.Query))
	return a, tea.Batch(cmds...)
}

// handleSearchProviderMsg handles provider-specific search requests
func (a *App) handleSearchProviderMsg(msg common.SearchProviderMsg) (*App, tea.Cmd) {
	a.debugLog("SearchProviderMsg: Provider=%s, Query=%s, SaveMapping=%v", msg.ProviderName, msg.Query, msg.SaveMapping)
	a.state = loadingView
	a.loadingOp = loadingProviderSearch
	var cmds []tea.Cmd

	// Update the provider instance using the helper
	if p, err := providers.Get(msg.ProviderName); err == nil {
		a.updateProvider(p)
	} else {
		a.debugLog("SearchProviderMsg: Failed to get provider %s: %v", msg.ProviderName, err)
		a.statusMsg = fmt.Sprintf("⚠ Failed to get provider %s: %v", msg.ProviderName, err)
		a.statusMsgTime = time.Now()
	}

	// If SaveMapping is true and we are in a global switch context (no specific media),
	// update the default provider in config
	if msg.SaveMapping && msg.Query == "Global Default" {
		if cfg, ok := a.cfg.(*config.Config); ok {
			if a.currentMediaType == providers.MediaTypeAnime {
				cfg.Providers.Default.Anime = msg.ProviderName
			} else {
				cfg.Providers.Default.MoviesAndTV = msg.ProviderName
			}
			if err := cfg.Save(); err != nil {
				a.statusMsg = fmt.Sprintf("⚠ Failed to save config: %v", err)
			} else {
				a.statusMsg = fmt.Sprintf("✓ Default provider set to %s", msg.ProviderName)
			}
			a.statusMsgTime = time.Now()
		}
	} else if msg.Query == "Global Default" {
		// Just Once case
		a.statusMsg = fmt.Sprintf("✓ Switched to %s (temporary)", msg.ProviderName)
		a.statusMsgTime = time.Now()
	}

	// If query is "Global Default", we don't need to search, just return to home
	if msg.Query == "Global Default" {
		if a.previousState != -1 {
			a.state = a.previousState
			a.previousState = -1
		} else {
			a.state = homeView
		}

		// Refresh home content to reflect new provider
		if a.state == homeView {
			cmds = append(cmds, a.home.Init())
		}

		return a, tea.Batch(cmds...)
	}

	cmds = append(cmds, a.spinner.Tick, a.searchSpecificProvider(msg.ProviderName, msg.Query))
	return a, tea.Batch(cmds...)
}

// handleSearchResultsMsg handles search results
func (a *App) handleSearchResultsMsg(msg common.SearchResultsMsg) (*App, tea.Cmd) {
	if msg.Err != nil {
		a.err = msg.Err
		a.state = errorView
		return a, nil
	}

	var cmds []tea.Cmd
	var mediaResults []providers.Media
	for _, r := range msg.Results {
		if media, ok := r.(providers.Media); ok {
			// Filter out media items with empty titles to prevent empty entries
			if strings.TrimSpace(media.Title) != "" {
				mediaResults = append(mediaResults, media)
			}
		}
	}

	a.results.SetMediaResults(mediaResults)
	// Enable manga info only for anime
	a.results.SetShowMangaInfo(a.currentMediaType == providers.MediaTypeAnime)
	a.results.SetProviderName(a.providerName)
	a.state = resultsView

	// Trigger fetch for the first few items if needed
	if len(mediaResults) > 0 {
		limit := 5
		if len(mediaResults) < limit {
			limit = len(mediaResults)
		}

		for i := 0; i < limit; i++ {
			if mediaResults[i].Synopsis == "" {
				idx := i
				mid := mediaResults[i].ID
				cmds = append(cmds, func() tea.Msg {
					return common.RequestDetailsMsg{
						MediaID: mid,
						Index:   idx,
					}
				})
			}
		}
	}

	return a, tea.Batch(cmds...)
}

// handleGenerateMediaDebugInfoMsg handles media debug info generation
func (a *App) handleGenerateMediaDebugInfoMsg(msg common.GenerateMediaDebugInfoMsg) (*App, tea.Cmd) {
	// Save current state to return to
	a.previousState = a.state
	// Show loading spinner
	a.state = loadingView
	a.loadingOp = loadingStream
	var cmds []tea.Cmd
	cmds = append(cmds, a.spinner.Tick, a.generateMediaDebugInfo(msg.MediaID, msg.Title, msg.Type))
	return a, tea.Batch(cmds...)
}

// handleMediaSelectedMsg handles media selection from search results
func (a *App) handleMediaSelectedMsg(msg common.MediaSelectedMsg) (*App, tea.Cmd) {
	var cmds []tea.Cmd

	// Check if we are in results view and need to save mapping
	if a.state == resultsView && a.remapShouldSave {
		// Find the selected media
		mediaResults := a.results.GetMediaResults()
		var selectedMedia *providers.Media
		for _, m := range mediaResults {
			if m.ID == msg.MediaID {
				selectedMedia = &m
				break
			}
		}

		if selectedMedia != nil {
			// Save mapping
			if a.mappingMgr != nil {
				if mgr, ok := a.mappingMgr.(*mapping.Manager); ok {
					// We need an ID to map TO.
					// If we are in AniList mode, we have a.currentAniListID.
					if a.currentAniListID > 0 {
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						err := mgr.SelectMapping(ctx, a.currentAniListID, a.providerName, *selectedMedia)
						cancel()

						if err != nil {
							a.statusMsg = fmt.Sprintf("⚠ Failed to save mapping: %v", err)
						} else {
							a.statusMsg = "✓ Mapping saved successfully"
						}
						a.statusMsgTime = time.Now()

						// Clear the flag
						a.remapShouldSave = false
					} else {
						// Not in AniList mode.
						a.statusMsg = "⚠ Cannot save preference without AniList context"
						a.statusMsgTime = time.Now()
						a.remapShouldSave = false
					}
				}
			}
		}
	}

	// Check if this is a provider selection for AniList
	if a.state == providerSelectionView && a.watchingFromAniList {
		a.debugLog("MediaSelectedMsg: Provider selection for AniList, anilistID=%d, mediaID=%s",
			a.currentAniListID, msg.MediaID)

		// Find the selected media from search results
		var selectedMedia *providers.Media
		for _, m := range a.providerSearchResults {
			if m.ID == msg.MediaID {
				selectedMedia = &m
				break
			}
		}

		if selectedMedia == nil {
			a.debugLog("ERROR: MediaSelectedMsg: Selected media not found")
			a.err = fmt.Errorf("selected media not found")
			a.state = errorView
			return a, nil
		}

		a.debugLog("MediaSelectedMsg: Found selected media: %s (ID: %s)",
			selectedMedia.Title, selectedMedia.ID)

		// Save the mapping
		if a.mappingMgr == nil {
			a.debugLog("ERROR: MediaSelectedMsg: mappingMgr is nil, cannot save mapping")
		} else {
			if mgr, ok := a.mappingMgr.(*mapping.Manager); ok {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				a.debugLog("MediaSelectedMsg: Saving mapping (AniList: %d → Provider: %s, Media: %s)",
					a.currentAniListID, a.providerName, selectedMedia.ID)

				if err := mgr.SelectMapping(ctx, a.currentAniListID, a.providerName, *selectedMedia); err != nil {
					a.debugLog("ERROR: MediaSelectedMsg: Failed to save mapping: %v", err)
					a.err = fmt.Errorf("failed to save mapping: %v", err)
					a.state = errorView
					return a, nil
				}

				a.debugLog("SUCCESS: MediaSelectedMsg: Mapping saved successfully")
			} else {
				a.debugLog("ERROR: MediaSelectedMsg: Type assertion to *mapping.Manager failed")
			}
		}

		// Set selected media and proceed to fetch seasons
		a.selectedMedia = *selectedMedia
		a.currentMediaType = selectedMedia.Type
		a.state = loadingView
		a.loadingOp = loadingSeasons
		cmds = append(cmds, a.spinner.Tick, a.getSeasons(a.selectedMedia.ID))
		return a, tea.Batch(cmds...)
	}

	// Normal search flow - but check if this is from AniList fallback manual search
	if a.watchingFromAniList && a.currentAniListID != 0 {
		// This is a manual search selection after AniList fallback
		// We need to save the mapping before proceeding
		a.debugLog("MediaSelectedMsg: Manual search from AniList fallback, saving mapping for AniList ID: %d", a.currentAniListID)

		selectedType := providers.MediaType(msg.Type)
		selectedMedia := providers.Media{
			ID:    msg.MediaID,
			Title: msg.Title,
			Type:  selectedType,
		}

		// Save the mapping for future use
		if a.mappingMgr != nil {
			if mgr, ok := a.mappingMgr.(*mapping.Manager); ok {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				// Get provider name from current provider
				providerType := providers.MediaTypeAnime
				if selectedMedia.Type == providers.MediaTypeManga {
					providerType = providers.MediaTypeManga
				}

				if provider := a.providers[providerType]; provider != nil {
					a.updateProvider(provider)
				}

				a.debugLog("MediaSelectedMsg: Saving mapping (AniList: %d → Provider: %s, Media: %s)",
					a.currentAniListID, a.providerName, selectedMedia.ID)

				if err := mgr.SelectMapping(ctx, a.currentAniListID, a.providerName, selectedMedia); err != nil {
					a.debugLog("ERROR: MediaSelectedMsg: Failed to save mapping: %v", err)
					// Don't fail the whole flow, just log the error
				} else {
					a.debugLog("SUCCESS: MediaSelectedMsg: Mapping saved successfully")
				}
			}
		}

		// Set selected media and proceed to fetch seasons (which will trigger auto-play)
		a.selectedMedia = selectedMedia
		a.currentMediaType = selectedMedia.Type
		a.state = loadingView
		a.loadingOp = loadingSeasons
		cmds = append(cmds, a.spinner.Tick, a.getSeasons(a.selectedMedia.ID))
		return a, tea.Batch(cmds...)
	}

	// Normal search flow (not from AniList)
	selectedType := providers.MediaType(msg.Type)
	a.selectedMedia = providers.Media{
		ID:    msg.MediaID,
		Title: msg.Title,
		Type:  selectedType,
	}

	a.debugLog("MediaSelectedMsg: ID=%s, Title=%s, Type=%s", msg.MediaID, msg.Title, msg.Type)

	// Determine which provider to use based on the selected media type
	// For movies/TV, use the MovieTV provider
	// For anime, use the Anime provider
	providerType := a.currentMediaType
	switch selectedType {
	case providers.MediaTypeMovie, providers.MediaTypeTV:
		providerType = providers.MediaTypeMovieTV
	case providers.MediaTypeAnime:
		providerType = providers.MediaTypeAnime
	case providers.MediaTypeManga:
		providerType = providers.MediaTypeManga
	}

	// Check if provider exists
	if _, ok := a.providers[providerType]; !ok {
		a.err = fmt.Errorf("no provider available for %s", providerType)
		a.state = errorView
		return a, nil
	}

	// Update currentMediaType to match the provider we'll use
	a.currentMediaType = providerType

	// For movies, skip season fetching and go straight to playback
	if selectedType == providers.MediaTypeMovie {
		a.debugLog("Detected as Movie, calling playMovieDirectly (using provider type: %s)", providerType)
		// For movies, set previousState to homeView so we return there after playback
		a.previousState = homeView
		a.state = loadingView
		a.loadingOp = loadingStream
		cmds = append(cmds, a.spinner.Tick, a.playMovieDirectly(a.selectedMedia.ID))
		return a, tea.Batch(cmds...)
	}

	// For TV shows and anime, fetch seasons
	a.debugLog("Detected as TV/Anime/Manga, calling getSeasons (using provider type: %s)", providerType)
	a.state = loadingView
	a.loadingOp = loadingSeasons
	cmds = append(cmds, a.spinner.Tick, a.getSeasons(a.selectedMedia.ID))
	return a, tea.Batch(cmds...)
}

// handleSeasonsLoadedMsg handles loaded seasons
func (a *App) handleSeasonsLoadedMsg(msg common.SeasonsLoadedMsg) (*App, tea.Cmd) {
	if msg.Error != nil {
		a.err = msg.Error
		a.state = errorView
		return a, nil
	}

	var cmds []tea.Cmd

	if len(msg.Seasons) == 0 && a.selectedMedia.Type == providers.MediaTypeMovie {
		// It's a movie, play directly
		a.state = loadingView
		a.loadingOp = loadingStream
		cmds = append(cmds, a.spinner.Tick, a.playMovieDirectly(a.selectedMedia.ID))
		return a, tea.Batch(cmds...)
	}

	if len(msg.Seasons) == 0 && a.selectedMedia.Type == providers.MediaTypeTV {
		a.err = fmt.Errorf("no seasons found for this TV show")
		a.state = errorView
		return a, nil
	}

	if len(msg.Seasons) == 0 && a.selectedMedia.Type == providers.MediaTypeAnime {
		// For anime with no seasons, try to get episodes directly via GetMediaDetails
		provider, ok := a.providers[a.currentMediaType]
		if !ok {
			a.err = fmt.Errorf("no provider available for %s", a.currentMediaType)
			a.state = errorView
			return a, nil
		}

		mediaDetails, err := provider.GetMediaDetails(context.Background(), a.selectedMedia.ID)
		if err != nil || mediaDetails == nil || len(mediaDetails.Seasons) == 0 {
			a.err = fmt.Errorf("no episodes found for this anime")
			a.state = errorView
			return a, nil
		}

		// Got seasons from media details, convert them
		var seasons []providers.Season
		for _, sInfo := range mediaDetails.Seasons {
			seasons = append(seasons, providers.Season{
				ID:     sInfo.ID,
				Number: sInfo.Number,
				Title:  sInfo.Title,
			})
		}
		a.seasonsList = seasons

		// If only one season, load episodes directly (auto-skip)
		if len(seasons) == 1 {
			a.state = loadingView
			a.loadingOp = loadingEpisodes
			cmds = append(cmds, a.spinner.Tick, a.getEpisodes(seasons[0].ID))
			return a, tea.Batch(cmds...)
		}

		// Multiple seasons - show selection screen
		a.seasons.SetMediaType(a.selectedMedia.Type)
		a.seasons.SetSeasons(a.seasonsList)
		a.state = seasonView
		return a, nil
	}

	if len(msg.Seasons) == 0 && a.selectedMedia.Type == providers.MediaTypeManga {
		// For manga with no seasons (chapters), try to get chapters directly via GetMediaDetails
		provider, ok := a.providers[a.currentMediaType]
		if !ok {
			a.err = fmt.Errorf("no provider available for %s", a.currentMediaType)
			a.state = errorView
			return a, nil
		}

		mediaDetails, err := provider.GetMediaDetails(context.Background(), a.selectedMedia.ID)
		if err != nil || mediaDetails == nil || len(mediaDetails.Seasons) == 0 {
			a.err = fmt.Errorf("no chapters found for this manga")
			a.state = errorView
			return a, nil
		}

		// Got seasons (chapters) from media details, convert them
		var seasons []providers.Season
		for _, sInfo := range mediaDetails.Seasons {
			seasons = append(seasons, providers.Season{
				ID:     sInfo.ID,
				Number: sInfo.Number,
				Title:  sInfo.Title,
			})
		}
		a.seasonsList = seasons

		// If only one season (Chapters), load episodes directly (auto-skip)
		if len(seasons) == 1 {
			a.state = loadingView
			a.loadingOp = loadingEpisodes
			cmds = append(cmds, a.spinner.Tick, a.getEpisodes(seasons[0].ID))
			return a, tea.Batch(cmds...)
		}

		// Multiple seasons - show selection screen
		a.seasons.SetMediaType(a.selectedMedia.Type)
		a.seasons.SetSeasons(a.seasonsList)
		a.state = seasonView
		return a, nil
	}

	var seasons []providers.Season
	for _, sInfo := range msg.Seasons {
		seasons = append(seasons, providers.Season{
			ID:     sInfo.ID,
			Number: sInfo.Number,
			Title:  sInfo.Title,
		})
	}
	a.seasonsList = seasons

	// If watching from AniList with no seasons found, try to get them using the original media ID
	if a.watchingFromAniList && len(seasons) == 0 {
		// Debug logging
		fmt.Printf("DEBUG: AniList content '%s' has no seasons, trying to get seasons with media ID: %s\n", a.selectedMedia.Title, a.selectedMedia.ID)

		// If no seasons found when watching from AniList, this might be content without seasons
		// Try to get seasons again using the original media ID as a fallback
		// First, get the current provider
		provider, ok := a.providers[a.currentMediaType]
		fmt.Printf("DEBUG: Provider found: %t, Type: %s\n", ok, a.currentMediaType)
		if !ok {
			a.err = fmt.Errorf("no provider available for %s", a.currentMediaType)
			a.state = errorView
			return a, nil
		}

		// Try to get seasons again with the original media ID
		seasons, err := provider.GetSeasons(context.Background(), a.selectedMedia.ID)
		if err != nil {
			fmt.Printf("DEBUG: GetSeasons error for %s: %v\n", a.selectedMedia.ID, err)
		}
		fmt.Printf("DEBUG: GetSeasons returned %d seasons for %s\n", len(seasons), a.selectedMedia.ID)

		if err != nil || len(seasons) == 0 {
			// If no seasons found, this might be a movie or single-cour anime with episodes directly
			// Try to get media details directly to see if we can access episodes another way
			// Instead of showing an error, try to get episodes directly for this type of content
			fmt.Printf("DEBUG: No seasons found or error getting seasons, trying GetMediaDetails...\n")
			mediaDetails, err := provider.GetMediaDetails(context.Background(), a.selectedMedia.ID)
			if err != nil || mediaDetails == nil {
				fmt.Printf("DEBUG: GetMediaDetails failed: %v\n", err)
				// If we still can't get details, show a more helpful error
				a.err = fmt.Errorf("content '%s' does not appear to have seasons available from the provider", a.selectedMedia.Title)
				a.state = errorView
				return a, nil
			}

			fmt.Printf("DEBUG: GetMediaDetails succeeded, number of seasons in detail: %d\n", len(mediaDetails.Seasons))

			// If media details exist but have no seasons, try to get episodes directly
			// In this case, try to use the first season from the media details if it exists
			if len(mediaDetails.Seasons) > 0 {
				// Use the first season from the details
				var seasonsList []providers.Season
				for _, sInfo := range mediaDetails.Seasons {
					seasonsList = append(seasonsList, providers.Season{
						ID:     sInfo.ID,
						Number: sInfo.Number,
						Title:  sInfo.Title,
					})
				}
				a.seasonsList = seasonsList

				// Continue with the same logic for single season
				if len(seasonsList) == 1 {
					a.state = loadingView
					a.loadingOp = loadingEpisodes
					cmds = append(cmds, a.spinner.Tick, a.getEpisodes(seasonsList[0].ID))
					return a, tea.Batch(cmds...)
				}

				// Multiple seasons - show selection screen
				a.seasons.SetMediaType(a.selectedMedia.Type)
				a.seasons.SetSeasons(a.seasonsList)
				a.state = seasonView
				return a, nil
			}

			// If media details exist but have no seasons, this might be a single-cour anime or movie
			// Instead of treating as bad mapping immediately, try to get episodes via seasons
			fmt.Printf("DEBUG: No seasons in media details, checking if media has episodes accessible through seasons for '%s'\n", a.selectedMedia.Title)

			// For some providers, even single-cour anime might have a "virtual" season
			// Let's try getting seasons again but handle the case where the result is a single "virtual" season
			provider := a.providers[a.currentMediaType]
			if provider != nil {
				// Get seasons again to check if there's a virtual season created
				seasons, err := provider.GetSeasons(context.Background(), a.selectedMedia.ID)
				if err == nil && len(seasons) > 0 {
					fmt.Printf("DEBUG: Found %d seasons after second attempt, using first for episodes\n", len(seasons))

					// Use the first season to get episodes
					episodes, err := provider.GetEpisodes(context.Background(), seasons[0].ID)
					if err == nil && len(episodes) > 0 {
						fmt.Printf("DEBUG: Found %d episodes through season '%s' for '%s', proceeding to episode view\n",
							len(episodes), seasons[0].Title, a.selectedMedia.Title)

						// Set episodes and proceed to episode view
						var epList []providers.Episode
						for _, epInfo := range episodes {
							epList = append(epList, providers.Episode{
								ID:     epInfo.ID,
								Number: epInfo.Number,
								Title:  epInfo.Title,
							})
						}
						a.episodes = epList

						// If watching from AniList, auto-play the first episode
						if a.watchingFromAniList && a.currentAniListMedia != nil {
							// Get the episode to play based on AniList progress (1-indexed)
							targetEpisode := a.currentAniListMedia.Progress + 1

							// If all episodes are complete, play the last episode
							if targetEpisode > a.currentAniListMedia.TotalEpisodes {
								targetEpisode = a.currentAniListMedia.TotalEpisodes
							}

							// Find the episode
							var episodeToPlay *providers.Episode
							for i := range epList {
								if epList[i].Number == targetEpisode {
									episodeToPlay = &epList[i]
									break
								}
							}

							if episodeToPlay == nil && len(epList) > 0 {
								// Fallback: play the first episode if exact match not found
								episodeToPlay = &epList[0]
							}

							if episodeToPlay != nil {
								// Clear any status message
								a.statusMsg = ""
								// Set previous state to anilist so user returns there after playback
								a.previousState = anilistView
								return a, func() tea.Msg {
									return common.EpisodeSelectedMsg{
										EpisodeID: episodeToPlay.ID,
										Number:    episodeToPlay.Number,
										Title:     episodeToPlay.Title,
									}
								}
							}
						}

						// For non-AniList context or if auto-play didn't happen, show episode list
						a.episodesComponent.SetMediaType(a.selectedMedia.Type)
						a.episodesComponent.SetEpisodes(a.episodes)
						a.state = episodeView
						return a, nil
					} else {
						fmt.Printf("DEBUG: Getting episodes through season failed for '%s': %v\n", a.selectedMedia.Title, err)
					}
				} else {
					fmt.Printf("DEBUG: Getting seasons again also failed for '%s': %v\n", a.selectedMedia.Title, err)
				}
			}

			// If we get here, direct episode fetching also failed
			// Now treat as bad mapping and delete it
			fmt.Printf("DEBUG: No episodes found directly either, treating as bad mapping for '%s'\n", a.selectedMedia.Title)
			if a.currentAniListMedia != nil && a.currentAniListID != 0 {
				// Delete the bad mapping to avoid reusing it
				if a.mappingMgr != nil {
					if mgr, ok := a.mappingMgr.(*mapping.Manager); ok {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						if err := mgr.DeleteMapping(ctx, a.currentAniListID); err != nil {
							fmt.Printf("DEBUG: Failed to delete bad mapping: %v\n", err)
						} else {
							fmt.Printf("DEBUG: Successfully deleted bad mapping for AniList ID: %d\n", a.currentAniListID)
						}
					}
				}

				// Only retry once to avoid infinite loops
				if !a.anilistSearchRetried {
					fmt.Printf("DEBUG: Retrying provider search with improved query variations...\n")
					a.anilistSearchRetried = true
					a.state = loadingView
					a.loadingOp = loadingProviderSearch
					cmds = append(cmds, a.spinner.Tick, a.searchProvidersForAniList(a.currentAniListMedia))
					return a, tea.Batch(cmds...)
				} else {
					// Already retried once, show error to avoid infinite loop
					fmt.Printf("DEBUG: Already retried search once, showing error\n")
					a.err = fmt.Errorf("no episodes found for '%s' from the provider: tried multiple search variations but couldn't find a valid match. please try: 1) press 'esc' to return, 2) press 's' to use manual search, 3) search for \"%s\" (or similar), 4) select the correct match from results",
						a.selectedMedia.Title,
						strings.ToLower(strings.ReplaceAll(a.selectedMedia.Title, ":", "")))
					a.state = errorView
					return a, nil
				}
			} else {
				// Fallback error if we don't have AniList media info
				a.err = fmt.Errorf("content '%s' does not appear to have episodes available from the provider", a.selectedMedia.Title)
				a.state = errorView
				return a, nil
			}
		}

		// If we got seasons now, update our internal list
		var seasonsList []providers.Season
		for _, sInfo := range seasons {
			seasonsList = append(seasonsList, providers.Season{
				ID:     sInfo.ID,
				Number: sInfo.Number,
				Title:  sInfo.Title,
			})
		}
		a.seasonsList = seasonsList

		// Continue with the same logic for single season
		if len(seasonsList) == 1 {
			a.state = loadingView
			a.loadingOp = loadingEpisodes
			cmds = append(cmds, a.spinner.Tick, a.getEpisodes(seasonsList[0].ID))
			return a, tea.Batch(cmds...)
		}

		// Multiple seasons - show selection screen
		a.seasons.SetMediaType(a.selectedMedia.Type)
		a.seasons.SetSeasons(a.seasonsList)
		a.state = seasonView
		return a, nil
	}

	// If only one season (regardless of AniList source), load episodes directly
	if len(seasons) == 1 {
		a.state = loadingView
		a.loadingOp = loadingEpisodes
		cmds = append(cmds, a.spinner.Tick, a.getEpisodes(seasons[0].ID))
		return a, tea.Batch(cmds...)
	}

	a.seasons.SetMediaType(a.selectedMedia.Type)
	// Multiple seasons - show selection screen
	a.seasons.SetSeasons(a.seasonsList)
	a.state = seasonView
	return a, nil
}

// handleSeasonSelectedMsg handles season selection
func (a *App) handleSeasonSelectedMsg(msg common.SeasonSelectedMsg) (*App, tea.Cmd) {
	// Find and store the season number from seasonsList
	for _, season := range a.seasonsList {
		if season.ID == msg.SeasonID {
			a.currentSeasonNumber = season.Number
			break
		}
	}
	a.state = loadingView
	a.loadingOp = loadingEpisodes
	var cmds []tea.Cmd
	cmds = append(cmds, a.spinner.Tick, a.getEpisodes(msg.SeasonID))
	return a, tea.Batch(cmds...)
}

// handleEpisodesLoadedMsg handles loaded episodes
func (a *App) handleEpisodesLoadedMsg(msg common.EpisodesLoadedMsg) (*App, tea.Cmd) {
	if msg.Error != nil {
		a.err = msg.Error
		a.state = errorView
		return a, nil
	}

	var episodes []providers.Episode
	for _, epInfo := range msg.Episodes {
		episodes = append(episodes, providers.Episode{
			ID:     epInfo.EpisodeID,
			Number: epInfo.Number,
			Title:  epInfo.Title,
		})
	}
	a.episodes = episodes

	// If watching from AniList, auto-play the current episode
	if a.watchingFromAniList && a.currentAniListMedia != nil {
		// Update total episodes/chapters from provider if Anilist data is missing or zero
		// This fixes display issues where total chapters shows as 0
		if a.currentAniListMedia.TotalEpisodes == 0 && len(episodes) > 0 {
			// Find the highest episode number
			maxNum := 0
			for _, ep := range episodes {
				if ep.Number > maxNum {
					maxNum = ep.Number
				}
			}

			// Use max number if reasonable, otherwise use count
			if maxNum > 0 && maxNum >= len(episodes) {
				a.currentAniListMedia.TotalEpisodes = maxNum
			} else {
				a.currentAniListMedia.TotalEpisodes = len(episodes)
			}
		}

		// Get the episode to play based on AniList progress
		// Progress is the number of episodes completed, so play Progress + 1
		targetEpisode := a.currentAniListMedia.Progress + 1

		a.debugLog("AniList Auto-Play: Progress=%d, Target=%d, TotalEpisodes=%d",
			a.currentAniListMedia.Progress, targetEpisode, a.currentAniListMedia.TotalEpisodes)

		// Check local history for latest progress to see if we're ahead of AniList
		// This handles cases where AniList sync might be delayed or failed
		if a.db != nil {
			var latestHistory database.History
			// Find the latest watched episode for this media
			err := a.db.Where("media_id = ?", a.selectedMedia.ID).
				Order("episode DESC").
				First(&latestHistory).Error

			if err == nil {
				// If we have local history
				if latestHistory.Episode > targetEpisode {
					// Local history is ahead, use it
					targetEpisode = latestHistory.Episode
					// If the latest episode is completed, move to the next one
					if latestHistory.Completed {
						targetEpisode++
					}
					a.debugLog("AniList Auto-Play: Local history override -> Target=%d", targetEpisode)
				} else if latestHistory.Episode == targetEpisode && latestHistory.Completed {
					// If the current target is completed locally, move to next
					targetEpisode++
					a.debugLog("AniList Auto-Play: Local history completed override -> Target=%d", targetEpisode)
				}
			}
		}

		// If all episodes are complete, play the last episode
		if targetEpisode > a.currentAniListMedia.TotalEpisodes && a.currentAniListMedia.TotalEpisodes > 0 {
			targetEpisode = a.currentAniListMedia.TotalEpisodes
		}

		// Find the episode
		var episodeToPlay *providers.Episode
		for i := range episodes {
			if episodes[i].Number == targetEpisode {
				episodeToPlay = &episodes[i]
				break
			}
		}

		if episodeToPlay == nil {
			a.debugLog("AniList Auto-Play: Target episode %d not found in %d episodes", targetEpisode, len(episodes))
			// Log first few episodes to see what we have
			for i := 0; i < 5 && i < len(episodes); i++ {
				a.debugLog("  Ep %d: Number=%d, Title=%s", i, episodes[i].Number, episodes[i].Title)
			}
		} else {
			a.debugLog("AniList Auto-Play: Found episode %d (ID: %s)", episodeToPlay.Number, episodeToPlay.ID)
		}

		if episodeToPlay == nil && len(episodes) > 0 {
			// Fallback: play the first episode if exact match not found
			episodeToPlay = &episodes[0]
			a.debugLog("AniList Auto-Play: Falling back to first episode %d", episodeToPlay.Number)
		}

		if episodeToPlay != nil {
			// Clear any status message (e.g., "Switched to manual search")
			a.statusMsg = ""

			// Set previous state to anilist so user returns there after playback
			a.previousState = anilistView
			return a, func() tea.Msg {
				return common.EpisodeSelectedMsg{
					EpisodeID: episodeToPlay.ID,
					Number:    episodeToPlay.Number,
					Title:     episodeToPlay.Title,
				}
			}
		}
	}

	// If there's only one episode (anime movie), play it directly
	if len(episodes) == 1 {
		// Set previous state to search so user can go back after playback
		a.previousState = searchView
		return a, func() tea.Msg {
			return common.EpisodeSelectedMsg{
				EpisodeID: episodes[0].ID,
				Number:    episodes[0].Number,
				Title:     episodes[0].Title,
			}
		}
	}

	// Multiple episodes - show selection screen
	a.episodesComponent.SetMediaType(a.selectedMedia.Type)
	a.episodesComponent.SetEpisodes(a.episodes)
	a.state = episodeView
	return a, nil
}

// performSearch performs a search with the current provider
func (a *App) performSearch(query string) tea.Cmd {
	// Save query for this media type
	if a.searchQueries == nil {
		a.searchQueries = make(map[providers.MediaType]string)
	}
	a.searchQueries[a.currentMediaType] = query

	return func() tea.Msg {
		provider, ok := a.providers[a.currentMediaType]
		if !ok {
			return common.SearchResultsMsg{Err: fmt.Errorf("no provider available for %s", a.currentMediaType)}
		}

		// Add timeout to prevent hanging indefinitely
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		results, err := provider.Search(ctx, query)
		if err != nil {
			return common.SearchResultsMsg{Err: fmt.Errorf("search failed: %w", err)}
		}

		// Filter results based on current media type
		// Some providers (like HDRezka) return mixed results even when searching for a specific type
		var filteredResults []providers.Media
		for _, r := range results {
			switch a.currentMediaType {
			case providers.MediaTypeAnime:
				if r.Type == providers.MediaTypeAnime {
					filteredResults = append(filteredResults, r)
				}
			case providers.MediaTypeMovie:
				if r.Type == providers.MediaTypeMovie {
					filteredResults = append(filteredResults, r)
				}
			case providers.MediaTypeTV:
				if r.Type == providers.MediaTypeTV {
					filteredResults = append(filteredResults, r)
				}
			case providers.MediaTypeMovieTV:
				if r.Type == providers.MediaTypeMovie || r.Type == providers.MediaTypeTV {
					filteredResults = append(filteredResults, r)
				}
			default:
				// For other types (like All or Manga), keep everything or let provider handle it
				filteredResults = append(filteredResults, r)
			}
		}
		results = filteredResults

		// Enrich results with detailed information (genres, rating, synopsis)
		// We no longer fetch details immediately to avoid API rate limits.
		// Instead, details are fetched on-demand as the user scrolls (lazy loading).
		// This makes the initial search much faster and prevents "thundering herd" issues.

		// TODO: This is a temporary hack to convert the results
		var interfaceResults []interface{}
		for _, r := range results {
			interfaceResults = append(interfaceResults, r)
		}
		return common.SearchResultsMsg{Results: interfaceResults, Err: nil}
	}
}

// searchSpecificProvider searches using a specific provider
func (a *App) searchSpecificProvider(providerName string, query string) tea.Cmd {
	return func() tea.Msg {
		a.debugLog("searchSpecificProvider: provider=%s, query=%s, currentMediaType=%s", providerName, query, a.currentMediaType)

		provider, err := providers.Get(providerName)
		if err != nil {
			a.debugLog("searchSpecificProvider: Failed to get provider: %v", err)
			return common.SearchResultsMsg{Err: fmt.Errorf("provider not found: %s", providerName)}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		results, err := provider.Search(ctx, query)
		if err != nil {
			a.debugLog("searchSpecificProvider: Search failed: %v", err)
			return common.SearchResultsMsg{Err: fmt.Errorf("search failed: %w", err)}
		}

		a.debugLog("searchSpecificProvider: Got %d raw results", len(results))

		// Filter results based on current media type (same as performSearch)
		var filteredResults []providers.Media
		for _, r := range results {
			switch a.currentMediaType {
			case providers.MediaTypeAnime:
				if r.Type == providers.MediaTypeAnime {
					filteredResults = append(filteredResults, r)
				}
			case providers.MediaTypeMovie:
				if r.Type == providers.MediaTypeMovie {
					filteredResults = append(filteredResults, r)
				}
			case providers.MediaTypeTV:
				if r.Type == providers.MediaTypeTV {
					filteredResults = append(filteredResults, r)
				}
			case providers.MediaTypeMovieTV:
				if r.Type == providers.MediaTypeMovie || r.Type == providers.MediaTypeTV {
					filteredResults = append(filteredResults, r)
				}
			default:
				// For other types (like All or Manga), keep everything or let provider handle it
				filteredResults = append(filteredResults, r)
			}
		}
		results = filteredResults

		a.debugLog("searchSpecificProvider: After filtering, %d results for type %s", len(results), a.currentMediaType)

		var interfaceResults []interface{}
		for _, r := range results {
			interfaceResults = append(interfaceResults, r)
		}

		return common.SearchResultsMsg{Results: interfaceResults}
	}
}

// fetchMediaDetails fetches detailed media information
func (a *App) fetchMediaDetails(mediaID string, index int) tea.Cmd {
	return func() tea.Msg {
		// Acquire semaphore to limit concurrent fetches
		a.detailsSem <- struct{}{}
		defer func() { <-a.detailsSem }()

		provider, ok := a.providers[a.currentMediaType]
		if !ok {
			return common.DetailsLoadedMsg{Err: fmt.Errorf("no provider available"), Index: index}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		details, err := provider.GetMediaDetails(ctx, mediaID)
		if err != nil {
			return common.DetailsLoadedMsg{Err: err, Index: index, Media: providers.Media{ID: mediaID}}
		}

		return common.DetailsLoadedMsg{
			Media: details.Media,
			Index: index,
		}
	}
}

// getSeasons retrieves seasons for the given media ID
func (a *App) getSeasons(mediaID string) tea.Cmd {
	return func() tea.Msg {
		provider := a.providers[a.currentMediaType]
		seasons, err := provider.GetSeasons(context.Background(), mediaID)
		if err != nil {
			return common.SeasonsLoadedMsg{Error: err}
		}

		var seasonInfos []common.SeasonInfo
		for _, s := range seasons {
			seasonInfos = append(seasonInfos, common.SeasonInfo{
				ID:     s.ID,
				Number: s.Number,
				Title:  s.Title,
			})
		}
		return common.SeasonsLoadedMsg{Seasons: seasonInfos}
	}
}

// getEpisodes retrieves episodes for the given season ID
func (a *App) getEpisodes(seasonID string) tea.Cmd {
	return func() tea.Msg {
		provider := a.providers[a.currentMediaType]
		episodes, err := provider.GetEpisodes(context.Background(), seasonID)
		if err != nil {
			return common.EpisodesLoadedMsg{Error: err}
		}

		var episodeInfos []common.EpisodeInfo
		for _, ep := range episodes {
			episodeInfos = append(episodeInfos, common.EpisodeInfo{
				EpisodeID: ep.ID,
				Number:    ep.Number,
				Title:     ep.Title,
			})
		}
		return common.EpisodesLoadedMsg{Episodes: episodeInfos}
	}
}

func (a *App) handleRequestDetailsMsg(msg common.RequestDetailsMsg) (tea.Model, tea.Cmd) {
	return a, a.fetchMediaDetails(msg.MediaID, msg.Index)
}

func (a *App) handleDetailsLoadedMsg(msg common.DetailsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		a.debugLog("Failed to fetch details for %s: %v", msg.Media.ID, msg.Err)
		return a, nil
	}
	a.results.UpdateMediaItem(msg.Index, msg.Media)
	return a, nil
}
