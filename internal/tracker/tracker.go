package tracker

import (
	"context"
	"time"

	"github.com/justchokingaround/greg/internal/providers"
)

// Tracker defines the interface for progress tracking services (AniList, MyAnimeList, Trakt, etc.)
type Tracker interface {
	// Authentication
	Authenticate(ctx context.Context) error
	IsAuthenticated() bool
	Logout() error

	// Media library
	GetUserLibrary(ctx context.Context, mediaType providers.MediaType) ([]TrackedMedia, error)
	SearchMedia(ctx context.Context, query string, mediaType providers.MediaType) ([]TrackedMedia, error)

	// Progress tracking
	UpdateProgress(ctx context.Context, mediaID string, episode int, progress float64) error
	GetProgress(ctx context.Context, mediaID string) (*Progress, error)

	// Metadata updates
	UpdateStatus(ctx context.Context, mediaID string, status WatchStatus) error
	UpdateScore(ctx context.Context, mediaID string, score float64) error
	UpdateDates(ctx context.Context, mediaID string, startDate, endDate *time.Time) error

	// Sync
	SyncHistory(ctx context.Context) error

	// Deletion (specific to services that support it)
	DeleteFromList(ctx context.Context, mediaListID int) error
}

// TrackedMedia represents a media item in a tracking service
type TrackedMedia struct {
	ServiceID     string              `json:"service_id"` // ID in tracking service (AniList, MAL, etc.)
	Title         string              `json:"title"`
	Type          providers.MediaType `json:"type"`
	Progress      int                 `json:"progress"` // Episodes watched
	TotalEpisodes int                 `json:"total_episodes"`
	Status        WatchStatus         `json:"status"`
	Score         float64             `json:"score"`
	StartDate     *time.Time          `json:"start_date,omitempty"`
	EndDate       *time.Time          `json:"end_date,omitempty"`
	Synopsis      string              `json:"synopsis"`
	PosterURL     string              `json:"poster_url"`
	UpdatedAt     time.Time           `json:"updated_at"`
	ListEntryID   int                 `json:"list_entry_id,omitempty"` // ID of the list entry (AniList MediaListEntry ID)
}

// Progress represents viewing progress for a media item
type Progress struct {
	MediaID       string        `json:"media_id"`
	Episode       int           `json:"episode"`
	Percentage    float64       `json:"percentage"`   // 0.0 - 1.0
	TimeWatched   time.Duration `json:"time_watched"` // Seconds watched in current episode
	TotalTime     time.Duration `json:"total_time"`   // Total episode duration
	LastWatchedAt time.Time     `json:"last_watched_at"`
	Completed     bool          `json:"completed"`
}

// WatchStatus represents the watching status of a media item
type WatchStatus string

const (
	StatusWatching    WatchStatus = "watching"
	StatusCompleted   WatchStatus = "completed"
	StatusOnHold      WatchStatus = "on_hold"
	StatusDropped     WatchStatus = "dropped"
	StatusPlanToWatch WatchStatus = "plan_to_watch"
	StatusRewatching  WatchStatus = "rewatching"
)

// String returns the string representation of WatchStatus
func (s WatchStatus) String() string {
	return string(s)
}

// ParseWatchStatus parses a string into a WatchStatus
func ParseWatchStatus(s string) (WatchStatus, error) {
	switch s {
	case "watching", "current":
		return StatusWatching, nil
	case "completed":
		return StatusCompleted, nil
	case "on_hold", "paused":
		return StatusOnHold, nil
	case "dropped":
		return StatusDropped, nil
	case "plan_to_watch", "planning":
		return StatusPlanToWatch, nil
	case "rewatching", "repeating":
		return StatusRewatching, nil
	default:
		return "", &ErrInvalidStatus{Status: s}
	}
}

// ErrInvalidStatus is returned when an invalid status string is provided
type ErrInvalidStatus struct {
	Status string
}

func (e *ErrInvalidStatus) Error() string {
	return "invalid watch status: " + e.Status
}

// SyncStatus represents the sync state of a media item
type SyncStatus struct {
	MediaID       string    `json:"media_id"`
	LocalProgress int       `json:"local_progress"`
	CloudProgress int       `json:"cloud_progress"`
	LastSyncedAt  time.Time `json:"last_synced_at"`
	NeedsSync     bool      `json:"needs_sync"`
	SyncError     string    `json:"sync_error,omitempty"`
}
