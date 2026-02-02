package database

import (
	"time"

	"gorm.io/gorm"
)

// History represents watch history for a media item
type History struct {
	ID              uint      `gorm:"primaryKey"`
	MediaID         string    `gorm:"not null;index"`
	MediaTitle      string    `gorm:"not null"`
	MediaType       string    `gorm:"not null;index"` // anime, movie, tv, manga
	Episode         int       `gorm:"default:0"`
	Season          int       `gorm:"default:0"`
	Page            int       `gorm:"default:0"` // For manga
	TotalPages      int       `gorm:"default:0"` // For manga
	ProgressSeconds int       `gorm:"not null"`
	TotalSeconds    int       `gorm:"not null"`
	ProgressPercent float64   `gorm:"not null"`
	WatchedAt       time.Time `gorm:"index;default:CURRENT_TIMESTAMP"`
	Completed       bool      `gorm:"default:false"`
	AniListID       *int      `gorm:"column:anilist_id;index;default:NULL"` // Optional AniList ID for tracking
	ProviderName    string    `gorm:"default:''"`                           // Provider used for this playback (FlixHQ, AllAnime, etc.)
}

// TableName overrides the table name
func (History) TableName() string {
	return "history"
}

// Statistic represents aggregate viewing statistics for a media item
type Statistic struct {
	ID             uint      `gorm:"primaryKey"`
	MediaID        string    `gorm:"not null;uniqueIndex"`
	MediaType      string    `gorm:"not null;index"`
	TotalWatchTime int       `gorm:"not null;default:0"` // seconds
	WatchCount     int       `gorm:"not null;default:1"`
	FirstWatched   time.Time `gorm:"default:CURRENT_TIMESTAMP"`
	LastWatched    time.Time `gorm:"default:CURRENT_TIMESTAMP"`
	Genre          string    `gorm:""`
}

// TableName overrides the table name
func (Statistic) TableName() string {
	return "statistics"
}

// SyncQueue represents items waiting to be synced to tracking services
type SyncQueue struct {
	ID        uint       `gorm:"primaryKey"`
	MediaID   string     `gorm:"not null"`
	AniListID *int       `gorm:""`
	Episode   int        `gorm:"not null"`
	Progress  float64    `gorm:"not null"`
	Status    string     `gorm:""` // watching, completed, etc.
	Score     *float64   `gorm:""`
	Synced    bool       `gorm:"default:false;index"`
	CreatedAt time.Time  `gorm:"default:CURRENT_TIMESTAMP"`
	SyncedAt  *time.Time `gorm:""`
}

// TableName overrides the table name
func (SyncQueue) TableName() string {
	return "sync_queue"
}

// Setting represents a key-value store for application settings
type Setting struct {
	Key       string    `gorm:"primaryKey"`
	Value     string    `gorm:"not null"`
	UpdatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP"`
}

// TableName overrides the table name
func (Setting) TableName() string {
	return "settings"
}

// Download represents a download task in the queue
type Download struct {
	ID              string     `gorm:"primaryKey"`
	MediaID         string     `gorm:"not null"`
	MediaTitle      string     `gorm:"not null"` // Title of the media
	MediaType       string     `gorm:"not null"` // Type (anime, movie, tv)
	Episode         int        `gorm:"not null"`
	Season          int        `gorm:"default:0"`
	Quality         string     `gorm:"not null"`
	Provider        string     `gorm:"not null"`
	Status          string     `gorm:"not null;index"` // queued, downloading, paused, completed, failed
	Progress        float64    `gorm:"default:0.0"`
	BytesDownloaded int64      `gorm:"default:0"` // Bytes downloaded
	TotalBytes      int64      `gorm:"default:0"` // Total bytes
	Speed           int64      `gorm:"default:0"` // Download speed (bytes/sec)
	Error           string     `gorm:""`          // Error message if failed
	FilePath        string     `gorm:""`
	CreatedAt       time.Time  `gorm:"default:CURRENT_TIMESTAMP"`
	StartedAt       *time.Time `gorm:""` // When download started
	CompletedAt     *time.Time `gorm:""`
}

// TableName overrides the table name
func (Download) TableName() string {
	return "downloads"
}

// AniListMapping represents the mapping between AniList media and provider media
type AniListMapping struct {
	ID              uint      `gorm:"primaryKey"`
	AniListID       int       `gorm:"column:anilist_id;not null;uniqueIndex"`
	ProviderName    string    `gorm:"not null"`
	ProviderMediaID string    `gorm:"not null"`
	Title           string    `gorm:"not null"`
	CreatedAt       time.Time `gorm:"default:CURRENT_TIMESTAMP"`
	UpdatedAt       time.Time `gorm:"default:CURRENT_TIMESTAMP"`
}

// TableName overrides the table name
func (AniListMapping) TableName() string {
	return "anilist_mappings"
}

// AudioPreference stores per-show audio track preferences
type AudioPreference struct {
	ID         uint      `gorm:"primaryKey"`
	AniListID  int       `gorm:"column:anilist_id;not null;uniqueIndex"`
	Preference string    `gorm:"not null"` // "dub" or "sub"
	TrackIndex *int      `gorm:""`         // Optional: last selected mpv track index (advisory only)
	CreatedAt  time.Time `gorm:"default:CURRENT_TIMESTAMP"`
	UpdatedAt  time.Time `gorm:"default:CURRENT_TIMESTAMP"`
}

// TableName overrides the table name
func (AudioPreference) TableName() string {
	return "audio_preferences"
}

// Migrate runs database migrations
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&History{},
		&Statistic{},
		&SyncQueue{},
		&Setting{},
		&Download{},
		&AniListMapping{},
		&AudioPreference{},
	)
}
