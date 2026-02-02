package providers

import (
	"fmt"
	"sort"
	"strings"

	"github.com/justchokingaround/greg/internal/providers/api"
)

// APIResultToMedia converts a SearchResult to Media
func APIResultToMedia(sr api.SearchResult, mediaType MediaType) Media {
	// Extract year from release date if available
	year := 0
	if len(sr.ReleaseDate) >= 4 {
		_, _ = fmt.Sscanf(sr.ReleaseDate[:4], "%d", &year)
	}

	return Media{
		ID:        sr.ID,
		Title:     sr.Title,
		Type:      mediaType,
		Year:      year,
		PosterURL: sr.Image,
	}
}

// APIInfoToMediaDetails converts InfoResponse to MediaDetails
func APIInfoToMediaDetails(ir api.InfoResponse, mediaType MediaType) *MediaDetails {
	// Determine status from Status field
	var statusStr string
	switch strings.ToUpper(ir.Status) {
	case "ONGOING", "AIRING", "RELEASING":
		statusStr = "Ongoing"
	case "COMPLETED", "FINISHED":
		statusStr = "Completed"
	case "CANCELLED":
		statusStr = "Cancelled"
	case "UPCOMING", "NOT_YET_AIRED":
		statusStr = "Upcoming"
	default:
		statusStr = "Unknown"
	}

	// Extract year from release date if available
	year := 0
	if len(ir.ReleaseDate) >= 4 {
		_, _ = fmt.Sscanf(ir.ReleaseDate[:4], "%d", &year)
	}

	// Parse rating from string to float64
	rating := 0.0
	if ir.Rating != "" {
		_, _ = fmt.Sscanf(ir.Rating, "%f", &rating)
	}

	details := &MediaDetails{
		Media: Media{
			ID:            ir.ID,
			Title:         ir.Title,
			Type:          mediaType,
			Year:          year,
			Synopsis:      ir.Description,
			PosterURL:     ir.Image,
			Rating:        rating,
			Genres:        ir.Genres,
			TotalEpisodes: ir.TotalEpisodes,
			Status:        statusStr,
		},
		Seasons: []Season{},
	}

	// For movies, don't create seasons - let the TUI handle direct playback
	if mediaType == MediaTypeMovie {
		return details
	}

	// For manga, handle chapters separately
	if mediaType == MediaTypeManga && len(ir.Chapters) > 0 {
		// Set total episodes to chapter count
		details.TotalEpisodes = len(ir.Chapters)
		// Create a single "Chapters" season
		details.Seasons = append(details.Seasons, Season{
			ID:     fmt.Sprintf("%s-chapters", ir.ID),
			Number: 1,
			Title:  "Chapters",
		})
		return details
	}

	// For TV shows and anime, group episodes by season number
	seasonNums := make(map[int]bool)
	for _, ep := range ir.Episodes {
		seasonNum := ep.Season
		if seasonNum == 0 {
			seasonNum = 1 // Default to season 1 if not specified
		}
		seasonNums[seasonNum] = true
	}

	// Create season metadata (without episodes - they'll be fetched via GetEpisodes)
	if len(seasonNums) > 0 {
		// Extract season numbers into a slice and sort them
		sortedSeasonNums := make([]int, 0, len(seasonNums))
		for seasonNum := range seasonNums {
			sortedSeasonNums = append(sortedSeasonNums, seasonNum)
		}
		sort.Ints(sortedSeasonNums)

		// Create seasons in sorted order
		for _, seasonNum := range sortedSeasonNums {
			title := fmt.Sprintf("Season %d", seasonNum)
			if mediaType == MediaTypeManga {
				title = "Chapters"
			}
			season := Season{
				ID:     fmt.Sprintf("%s-season-%d", ir.ID, seasonNum),
				Number: seasonNum,
				Title:  title,
			}
			details.Seasons = append(details.Seasons, season)
		}
	} else if len(ir.Episodes) > 0 {
		// No season info, create default season 1
		// For anime type, we might want to avoid the -season-1 suffix if it's a movie-like anime
		// but for consistency with the system, we'll use the same format
		title := "Season 1"
		if mediaType == MediaTypeManga {
			title = "Chapters"
		}
		details.Seasons = append(details.Seasons, Season{
			ID:     fmt.Sprintf("%s-season-1", ir.ID),
			Number: 1,
			Title:  title,
		})
	}

	return details
}

// GetEpisodesFromAPIInfo extracts episodes for a specific season from InfoResponse
func GetEpisodesFromAPIInfo(ir api.InfoResponse, seasonNumber int) []Episode {
	episodes := make([]Episode, 0)

	for _, ep := range ir.Episodes {
		epSeasonNum := ep.Season
		if epSeasonNum == 0 {
			epSeasonNum = 1 // Default to season 1
		}

		if epSeasonNum == seasonNumber {
			// For providers like SFlix, the mediaID is stored in ep.URL
			// We need to combine episodeID and mediaID with a separator
			episodeID := ep.ID
			if ep.URL != "" {
				// Store as "episodeID|mediaID" so GetStreamURL can extract both
				episodeID = fmt.Sprintf("%s|%s", ep.ID, ep.URL)
			}

			episodes = append(episodes, Episode{
				ID:     episodeID,
				Number: ep.Number,
				Season: epSeasonNum,
				Title:  ep.Title,
			})
		}
	}

	return episodes
}

// GetEpisodesFromAPIInfoWithMediaID extracts episodes with full media ID prefix for providers like HiAnime
// Format: {mediaID}$episode${episodeID}
func GetEpisodesFromAPIInfoWithMediaID(ir api.InfoResponse, seasonNumber int, mediaID string) []Episode {
	episodes := make([]Episode, 0)

	for _, ep := range ir.Episodes {
		epSeasonNum := ep.Season
		if epSeasonNum == 0 {
			epSeasonNum = 1 // Default to season 1
		}

		if epSeasonNum == seasonNumber {
			// For HiAnime, construct the full episode ID: {mediaID}$episode${episodeID}
			episodeID := fmt.Sprintf("%s$episode$%s", mediaID, ep.ID)

			episodes = append(episodes, Episode{
				ID:     episodeID,
				Number: ep.Number,
				Season: epSeasonNum,
				Title:  ep.Title,
			})
		}
	}

	return episodes
}

// GetChaptersAsEpisodesFromAPIInfo converts manga chapters to episodes
func GetChaptersAsEpisodesFromAPIInfo(ir api.InfoResponse) []Episode {
	episodes := make([]Episode, 0, len(ir.Chapters))

	for _, chapter := range ir.Chapters {
		// Parse chapter number from string (could be "1", "1.5", etc.)
		var chapterNum int
		_, _ = fmt.Sscanf(chapter.Number, "%d", &chapterNum)

		episodes = append(episodes, Episode{
			ID:     chapter.ID,
			Number: chapterNum,
			Title:  chapter.Title,
			Season: 1, // Manga uses single season
		})
	}

	return episodes
}

// APISourceToStreamURL converts a Source to StreamURL
func APISourceToStreamURL(s api.Source, referer, origin string) *StreamURL {
	streamType := StreamTypeMP4
	if s.IsM3U8 {
		streamType = StreamTypeHLS
	}

	// Parse quality
	quality := parseQuality(s.Quality)

	// Prefer source-level referer/origin (e.g., from HiAnime API)
	// over headers-level referer/origin
	finalReferer := s.Referer
	if finalReferer == "" {
		finalReferer = referer
	}

	finalOrigin := s.Origin
	if finalOrigin == "" {
		finalOrigin = origin
	}

	headers := make(map[string]string)
	if finalOrigin != "" {
		headers["Origin"] = finalOrigin
	}

	return &StreamURL{
		URL:     s.URL,
		Quality: quality,
		Type:    streamType,
		Headers: headers,
		Referer: finalReferer, // Set Referer field directly (mpv uses --referrer option)
	}
}

// parseQuality converts quality string to Quality
func parseQuality(qualityStr string) Quality {
	switch strings.ToLower(qualityStr) {
	case "360p":
		return Quality360p
	case "480p":
		return Quality480p
	case "720p", "hd":
		return Quality720p
	case "1080p", "fhd", "full hd":
		return Quality1080p
	case "1440p", "2k":
		return Quality1440p
	case "2160p", "4k", "uhd":
		return Quality4K
	case "auto", "default":
		return QualityAuto
	default:
		return QualityAuto
	}
}

// APISubtitlesToSubtitles converts API subtitles to Subtitle
func APISubtitlesToSubtitles(apiSubtitles []api.Subtitle) []Subtitle {
	subtitles := make([]Subtitle, 0, len(apiSubtitles))
	for _, sub := range apiSubtitles {
		// Infer format from URL extension
		format := "vtt" // Default to VTT
		if strings.HasSuffix(strings.ToLower(sub.URL), ".srt") {
			format = "srt"
		} else if strings.HasSuffix(strings.ToLower(sub.URL), ".ass") {
			format = "ass"
		}

		subtitles = append(subtitles, Subtitle{
			Language: sub.Lang,
			URL:      sub.URL,
			Format:   format,
		})
	}
	return subtitles
}
