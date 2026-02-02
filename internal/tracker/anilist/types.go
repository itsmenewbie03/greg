package anilist

import (
	"fmt"
	"strconv"
	"time"

	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tracker"
)

// anilistMedia represents a media item from AniList
type anilistMedia struct {
	ID                int                    `json:"id"`
	Title             anilistTitle           `json:"title"`
	Episodes          int                    `json:"episodes"`
	Chapters          int                    `json:"chapters"`
	Description       string                 `json:"description"`
	CoverImage        anilistImage           `json:"coverImage"`
	Type              string                 `json:"type"`
	Format            string                 `json:"format"`
	Status            string                 `json:"status"`
	Duration          int                    `json:"duration"`
	Genres            []string               `json:"genres"`
	AverageScore      int                    `json:"averageScore"`
	Popularity        int                    `json:"popularity"`
	StartDate         anilistDate            `json:"startDate"`
	EndDate           anilistDate            `json:"endDate"`
	BannerImage       string                 `json:"bannerImage"`
	Season            string                 `json:"season"`
	SeasonYear        int                    `json:"seasonYear"`
	IsAdult           bool                   `json:"isAdult"`
	NextAiringEpisode *anilistAiringEpisode  `json:"nextAiringEpisode"`
	MediaListEntry    *anilistMediaListEntry `json:"mediaListEntry"`
	Studios           anilistStudios         `json:"studios"`
}

// anilistAiringEpisode represents next airing episode information
type anilistAiringEpisode struct {
	AiringAt        int64 `json:"airingAt"`
	TimeUntilAiring int   `json:"timeUntilAiring"`
	Episode         int   `json:"episode"`
}

// anilistMediaListEntry represents media list entry
type anilistMediaListEntry struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
}

// anilistStudios represents studios associated with the media
type anilistStudios struct {
	Edges []anilistStudioEdge `json:"edges"`
}

// anilistStudioEdge represents a studio edge
type anilistStudioEdge struct {
	IsMain bool          `json:"isMain"`
	Node   anilistStudio `json:"node"`
}

// anilistStudio represents a studio
type anilistStudio struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// anilistTitle represents the title of a media item
type anilistTitle struct {
	Romaji        string `json:"romaji"`
	English       string `json:"english"`
	Native        string `json:"native"`
	UserPreferred string `json:"userPreferred"`
}

// anilistImage represents an image from AniList
type anilistImage struct {
	Large      string `json:"large"`
	ExtraLarge string `json:"extraLarge"`
	Color      string `json:"color"`
	Medium     string `json:"medium"`
}

// anilistDate represents a fuzzy date from AniList
type anilistDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

// ToTime converts an AniList fuzzy date to a time.Time
func (d *anilistDate) ToTime() *time.Time {
	if d == nil || d.Year == 0 {
		return nil
	}

	month := time.Month(d.Month)
	if month == 0 {
		month = 1
	}
	day := d.Day
	if day == 0 {
		day = 1
	}

	t := time.Date(d.Year, month, day, 0, 0, 0, 0, time.UTC)
	return &t
}

// anilistEntry represents a media list entry from AniList
type anilistEntry struct {
	ID          int          `json:"id"` // This is the MediaListEntry ID - use this for deletion
	Status      string       `json:"status"`
	Score       float64      `json:"score"`
	Progress    int          `json:"progress"`
	StartedAt   anilistDate  `json:"startedAt"`
	CompletedAt anilistDate  `json:"completedAt"`
	UpdatedAt   int64        `json:"updatedAt"`
	Media       anilistMedia `json:"media"`
}

// entryToTrackedMedia converts an AniList entry to a TrackedMedia
func entryToTrackedMedia(entry anilistEntry) tracker.TrackedMedia {
	status, _ := tracker.ParseWatchStatus(mapAniListStatus(entry.Status))

	totalUnits := entry.Media.Episodes
	if entry.Media.Type == "MANGA" {
		totalUnits = entry.Media.Chapters
	}

	return tracker.TrackedMedia{
		ServiceID:     fmt.Sprintf("%d", entry.Media.ID),
		Title:         getBestTitle(entry.Media.Title),
		Type:          mapMediaType(entry.Media.Type),
		Progress:      entry.Progress,
		TotalEpisodes: totalUnits,
		Status:        status,
		Score:         entry.Score,
		StartDate:     entry.StartedAt.ToTime(),
		EndDate:       entry.CompletedAt.ToTime(),
		Synopsis:      entry.Media.Description,
		PosterURL:     entry.Media.CoverImage.Large,
		UpdatedAt:     time.Unix(entry.UpdatedAt, 0),
		ListEntryID:   entry.ID, // Store the MediaListEntry ID for deletion
	}
}

// getBestTitle returns the best available title (prefers user-preferred, then English, falls back to Romaji)
func getBestTitle(title anilistTitle) string {
	if title.UserPreferred != "" {
		return title.UserPreferred
	}
	if title.English != "" {
		return title.English
	}
	if title.Romaji != "" {
		return title.Romaji
	}
	return title.Native
}

// mapMediaType converts an AniList media type to a provider media type
func mapMediaType(anilistType string) providers.MediaType {
	switch anilistType {
	case "ANIME":
		return providers.MediaTypeAnime
	case "MANGA":
		return providers.MediaTypeManga
	default:
		return providers.MediaTypeAnime
	}
}

// anilistMediaType converts a provider media type to an AniList media type
func anilistMediaType(mediaType providers.MediaType) string {
	switch mediaType {
	case providers.MediaTypeAnime:
		return "ANIME"
	case providers.MediaTypeMovie:
		return "ANIME" // Movies are also anime in AniList
	case providers.MediaTypeTV:
		return "ANIME"
	case providers.MediaTypeManga:
		return "MANGA"
	default:
		return "ANIME"
	}
}

// anilistStatus converts a tracker status to an AniList status
func anilistStatus(status tracker.WatchStatus) string {
	switch status {
	case tracker.StatusWatching:
		return "CURRENT"
	case tracker.StatusCompleted:
		return "COMPLETED"
	case tracker.StatusOnHold:
		return "PAUSED"
	case tracker.StatusDropped:
		return "DROPPED"
	case tracker.StatusPlanToWatch:
		return "PLANNING"
	case tracker.StatusRewatching:
		return "REPEATING"
	default:
		return "CURRENT"
	}
}

// mapAniListStatus converts an AniList status to a tracker status string
func mapAniListStatus(status string) string {
	switch status {
	case "CURRENT":
		return "watching"
	case "COMPLETED":
		return "completed"
	case "PAUSED":
		return "on_hold"
	case "DROPPED":
		return "dropped"
	case "PLANNING":
		return "plan_to_watch"
	case "REPEATING":
		return "rewatching"
	default:
		return "watching"
	}
}

// mustParseInt parses a string to an int, panicking on error
func mustParseInt(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		// Try to handle if it's already an int in string form
		return 0
	}
	return i
}
