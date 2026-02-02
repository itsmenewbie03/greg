package home

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/justchokingaround/greg/internal/database"
	"github.com/justchokingaround/greg/internal/providers"
)

// RecentItem represents a recently watched item from history
type RecentItem struct {
	ID              uint
	MediaID         string
	MediaTitle      string
	MediaType       string
	Episode         int
	Season          int
	Page            int
	TotalPages      int
	ProgressPercent float64
	ProgressSeconds int
	TotalSeconds    int
	WatchedAt       time.Time
	ProviderName    string
}

// FetchRecentHistory fetches incomplete watches from the database
// Returns up to 'limit' items that are:
// - Not marked as fully completed
// - Have some progress (>= 0%, including 0% placeholders for next episode)
// - Optionally filtered by provider name to avoid cross-provider ID conflicts
// - Ordered by most recently watched
func FetchRecentHistory(db *gorm.DB, mediaType string, providerName string, limit int) ([]RecentItem, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}

	// First, get all matching records ordered by watched_at DESC
	var allRecords []database.History
	query := db.Where("completed = ?", false).
		Where("progress_percent >= ?", 0).
		Order("watched_at DESC")

	// Filter by media type if specified
	if mediaType != "" {
		switch providers.MediaType(mediaType) {
		case providers.MediaTypeAnime:
			query = query.Where("media_type = ?", "anime")
		case providers.MediaTypeManga:
			query = query.Where("media_type = ?", "manga")
		case providers.MediaTypeMovieTV:
			// Include both movie and tv
			query = query.Where("media_type IN (?)", []string{"movie", "tv"})
		}
	}

	// Filter by provider name if specified to avoid cross-provider incompatibilities
	if providerName != "" {
		query = query.Where("provider_name = ?", providerName)
	}
	// else: no filter needed, return all results

	if err := query.Find(&allRecords).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch recent history: %w", err)
	}

	// Deduplicate by media_title - keep only the most recent episode per media
	seen := make(map[string]bool)
	var historyRecords []database.History
	for _, record := range allRecords {
		if !seen[record.MediaTitle] {
			seen[record.MediaTitle] = true
			historyRecords = append(historyRecords, record)
			if len(historyRecords) >= limit {
				break
			}
		}
	}

	// Convert to RecentItem
	items := make([]RecentItem, 0, len(historyRecords))
	for _, h := range historyRecords {
		items = append(items, RecentItem{
			ID:              h.ID,
			MediaID:         h.MediaID,
			MediaTitle:      h.MediaTitle,
			MediaType:       h.MediaType,
			Episode:         h.Episode,
			Season:          h.Season,
			Page:            h.Page,
			TotalPages:      h.TotalPages,
			ProgressPercent: h.ProgressPercent,
			ProgressSeconds: h.ProgressSeconds,
			TotalSeconds:    h.TotalSeconds,
			WatchedAt:       h.WatchedAt,
			ProviderName:    h.ProviderName,
		})
	}

	return items, nil
}

// FormatTimeAgo formats a time as a human-readable "time ago" string
func FormatTimeAgo(t time.Time) string {
	duration := time.Since(t)

	// Less than a minute
	if duration < time.Minute {
		return "just now"
	}

	// Less than an hour
	if duration < time.Hour {
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	}

	// Less than a day
	if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}

	// Less than a week
	if duration < 7*24*time.Hour {
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}

	// Less than a month
	if duration < 30*24*time.Hour {
		weeks := int(duration.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	}

	// More than a month
	months := int(duration.Hours() / 24 / 30)
	if months == 1 {
		return "1 month ago"
	}
	return fmt.Sprintf("%d months ago", months)
}

// FormatDuration formats seconds as HH:MM:SS or MM:SS
func FormatDuration(seconds int) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%d:%02d", minutes, secs)
}

// FormatEpisodeTitle returns a formatted episode title
func FormatEpisodeTitle(item RecentItem) string {
	if item.Episode == 0 {
		// Movie
		return item.MediaTitle
	}

	// For Manga
	if item.MediaType == "manga" {
		return fmt.Sprintf("%s - Chapter %d", item.MediaTitle, item.Episode)
	}

	// For TV/Anime with episodes
	if item.Season > 0 {
		return fmt.Sprintf("%s - S%d E%d", item.MediaTitle, item.Season, item.Episode)
	}
	return fmt.Sprintf("%s - Episode %d", item.MediaTitle, item.Episode)
}
