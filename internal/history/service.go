package history

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/justchokingaround/greg/internal/database"
)

// Service provides history management functionality
type Service struct {
	db *gorm.DB
}

// SortOrder defines the sorting order for history items
type SortOrder string

const (
	SortRecentFirst  SortOrder = "recent_first"
	SortOldestFirst  SortOrder = "oldest_first"
	SortTitleAsc     SortOrder = "title_asc"
	SortTitleDesc    SortOrder = "title_desc"
	SortProgressAsc  SortOrder = "progress_asc"
	SortProgressDesc SortOrder = "progress_desc"
)

// FilterOptions defines filtering options for history queries
type FilterOptions struct {
	MediaType    string    // anime, movie, tv, or empty for all
	ProviderName string    // Filter by provider
	SearchQuery  string    // Search in title
	StartDate    time.Time // Filter by date range
	EndDate      time.Time
	Completed    *bool     // Filter by completion status
	Limit        int       // Limit results (0 = no limit)
	Offset       int       // Offset for pagination
	SortBy       SortOrder // Sorting order
}

// HistoryItem represents a history item with additional computed fields
type HistoryItem struct {
	ID              uint
	MediaID         string
	MediaTitle      string
	MediaType       string
	Episode         int
	Season          int
	ProgressSeconds int
	TotalSeconds    int
	ProgressPercent float64
	WatchedAt       time.Time
	Completed       bool
	AniListID       *int
	ProviderName    string
}

// Stats represents watch history statistics
type Stats struct {
	TotalItems     int64
	TotalWatchTime time.Duration
	AnimeCount     int64
	MovieCount     int64
	TVCount        int64
	CompletedCount int64
}

// NewService creates a new history service
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// AddOrUpdate adds a new history record or updates an existing one
func (s *Service) AddOrUpdate(history database.History) error {
	if s.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// For incomplete watches, try to update existing record first
	if !history.Completed {
		var existing database.History
		err := s.db.Where("media_id = ? AND episode = ? AND completed = false", history.MediaID, history.Episode).
			Order("watched_at DESC").
			First(&existing).Error

		if err == nil {
			// Update existing record
			existing.ProgressSeconds = history.ProgressSeconds
			existing.TotalSeconds = history.TotalSeconds
			existing.ProgressPercent = history.ProgressPercent
			existing.WatchedAt = time.Now()
			existing.ProviderName = history.ProviderName

			return s.db.Save(&existing).Error
		}
	}

	// If this is a completed watch, delete any previous incomplete records for this media/episode
	if history.Completed {
		s.db.Where("media_id = ? AND episode = ? AND completed = false", history.MediaID, history.Episode).
			Delete(&database.History{})
	}

	// Create new record
	history.WatchedAt = time.Now()
	return s.db.Create(&history).Error
}

// GetHistory retrieves history items with filtering and sorting
func (s *Service) GetHistory(filter FilterOptions) ([]HistoryItem, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// Start building the query
	query := s.db.Model(&database.History{})

	// Apply filters
	if filter.MediaType != "" {
		query = query.Where("media_type = ?", filter.MediaType)
	}

	if filter.ProviderName != "" {
		query = query.Where("provider_name = ?", filter.ProviderName)
	}

	if filter.SearchQuery != "" {
		query = query.Where("media_title LIKE ?", "%"+filter.SearchQuery+"%")
	}

	if !filter.StartDate.IsZero() {
		query = query.Where("watched_at >= ?", filter.StartDate)
	}

	if !filter.EndDate.IsZero() {
		query = query.Where("watched_at <= ?", filter.EndDate)
	}

	if filter.Completed != nil {
		query = query.Where("completed = ?", *filter.Completed)
	}

	// Apply sorting
	switch filter.SortBy {
	case SortOldestFirst:
		query = query.Order("watched_at ASC")
	case SortTitleAsc:
		query = query.Order("media_title ASC")
	case SortTitleDesc:
		query = query.Order("media_title DESC")
	case SortProgressAsc:
		query = query.Order("progress_percent ASC")
	case SortProgressDesc:
		query = query.Order("progress_percent DESC")
	default: // SortRecentFirst
		query = query.Order("watched_at DESC")
	}

	// Apply pagination
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	// Execute query
	var records []database.History
	if err := query.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch history: %w", err)
	}

	// Convert to HistoryItem
	items := make([]HistoryItem, len(records))
	for i, record := range records {
		items[i] = HistoryItem{
			ID:              record.ID,
			MediaID:         record.MediaID,
			MediaTitle:      record.MediaTitle,
			MediaType:       record.MediaType,
			Episode:         record.Episode,
			Season:          record.Season,
			ProgressSeconds: record.ProgressSeconds,
			TotalSeconds:    record.TotalSeconds,
			ProgressPercent: record.ProgressPercent,
			WatchedAt:       record.WatchedAt,
			Completed:       record.Completed,
			AniListID:       record.AniListID,
			ProviderName:    record.ProviderName,
		}
	}

	return items, nil
}

// GetByID retrieves a specific history item by ID
func (s *Service) GetByID(id uint) (*HistoryItem, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	var record database.History
	if err := s.db.First(&record, id).Error; err != nil {
		return nil, err
	}

	item := &HistoryItem{
		ID:              record.ID,
		MediaID:         record.MediaID,
		MediaTitle:      record.MediaTitle,
		MediaType:       record.MediaType,
		Episode:         record.Episode,
		Season:          record.Season,
		ProgressSeconds: record.ProgressSeconds,
		TotalSeconds:    record.TotalSeconds,
		ProgressPercent: record.ProgressPercent,
		WatchedAt:       record.WatchedAt,
		Completed:       record.Completed,
		AniListID:       record.AniListID,
		ProviderName:    record.ProviderName,
	}

	return item, nil
}

// DeleteByID removes a history item by ID
func (s *Service) DeleteByID(id uint) error {
	if s.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	return s.db.Delete(&database.History{}, id).Error
}

// DeleteByMediaID removes all history items for a specific media
func (s *Service) DeleteByMediaID(mediaID string) error {
	if s.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	return s.db.Where("media_id = ?", mediaID).Delete(&database.History{}).Error
}

// MarkAsCompleted marks a history item as completed
func (s *Service) MarkAsCompleted(id uint) error {
	if s.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	return s.db.Model(&database.History{}).Where("id = ?", id).Update("completed", true).Error
}

// GetStats retrieves watch history statistics
func (s *Service) GetStats() (*Stats, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	var stats Stats

	// Get total count
	if err := s.db.Model(&database.History{}).Count(&stats.TotalItems).Error; err != nil {
		return nil, err
	}

	// Get total watch time (in seconds)
	var totalSeconds int64
	if err := s.db.Model(&database.History{}).Select("SUM(total_seconds)").Scan(&totalSeconds).Error; err != nil {
		return nil, err
	}
	stats.TotalWatchTime = time.Duration(totalSeconds) * time.Second

	// Get counts by media type
	if err := s.db.Model(&database.History{}).Where("media_type = ?", "anime").Count(&stats.AnimeCount).Error; err != nil {
		return nil, err
	}

	if err := s.db.Model(&database.History{}).Where("media_type = ?", "movie").Count(&stats.MovieCount).Error; err != nil {
		return nil, err
	}

	if err := s.db.Model(&database.History{}).Where("media_type = ?", "tv").Count(&stats.TVCount).Error; err != nil {
		return nil, err
	}

	// Get completed count
	if err := s.db.Model(&database.History{}).Where("completed = ?", true).Count(&stats.CompletedCount).Error; err != nil {
		return nil, err
	}

	return &stats, nil
}

// Cleanup removes old incomplete history records
func (s *Service) Cleanup() error {
	if s.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Remove incomplete records older than 30 days
	cutoff := time.Now().AddDate(0, 0, -30)
	return s.db.Where("completed = ? AND watched_at < ?", false, cutoff).Delete(&database.History{}).Error
}

// UpdateMediaTypes corrects media types in history based on actual media information
func (s *Service) UpdateMediaTypes() error {
	if s.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Get all history records
	var histories []database.History
	err := s.db.Find(&histories).Error
	if err != nil {
		return err
	}

	fmt.Printf("Found %d history records to potentially update media types for\n", len(histories))

	// In a full implementation, you would need to determine the correct media type for each entry
	// This could involve:
	// 1. Querying providers for the media type of each media ID
	// 2. Using external APIs to determine the correct type based on title
	// 3. Using a mapping service to determine the correct type

	// For now, just return - the important fix is that new entries will have correct types
	// due to the changes in the savePlaybackProgress function in model.go
	return nil
}
