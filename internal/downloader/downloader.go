package downloader

import (
	"context"
	"time"

	"github.com/justchokingaround/greg/internal/providers"
)

// Downloader defines the interface for download managers
type Downloader interface {
	// Queue management
	AddToQueue(ctx context.Context, task DownloadTask) error
	RemoveFromQueue(ctx context.Context, id string) error
	GetQueue(ctx context.Context) ([]DownloadTask, error)
	ClearQueue(ctx context.Context) error

	// Download control
	Start(ctx context.Context) error
	Pause(ctx context.Context, id string) error
	Resume(ctx context.Context, id string) error
	Cancel(ctx context.Context, id string) error
	PauseAll(ctx context.Context) error
	ResumeAll(ctx context.Context) error
	Retry(ctx context.Context, id string) error
	DeleteTaskAndFile(ctx context.Context, id string) error

	// Progress monitoring
	OnProgressUpdate(callback func(task DownloadTask))
	OnDownloadComplete(callback func(task DownloadTask))
	OnDownloadError(callback func(task DownloadTask, err error))

	// Settings
	SetConcurrency(workers int)
	SetOutputDir(dir string)
	SetMaxSpeed(bytesPerSecond int64) // 0 = unlimited
}

// DownloadTask represents a single download task
type DownloadTask struct {
	ID              string               `json:"id"`
	MediaID         string               `json:"media_id"`
	MediaTitle      string               `json:"media_title"`
	MediaType       providers.MediaType  `json:"media_type"`
	Episode         int                  `json:"episode"`
	Season          int                  `json:"season,omitempty"`
	Quality         providers.Quality    `json:"quality"`
	Provider        string               `json:"provider"`
	StreamURL       string               `json:"stream_url"`
	StreamType      providers.StreamType `json:"stream_type"`
	Headers         map[string]string    `json:"headers,omitempty"`
	Referer         string               `json:"referer,omitempty"`
	OutputPath      string               `json:"output_path"`
	Subtitles       []providers.Subtitle `json:"subtitles,omitempty"`
	EmbedSubs       bool                 `json:"embed_subs"`
	Status          DownloadStatus       `json:"status"`
	Progress        float64              `json:"progress"` // 0.0 - 100.0
	BytesDownloaded int64                `json:"bytes_downloaded"`
	TotalBytes      int64                `json:"total_bytes"`
	Speed           int64                `json:"speed"` // bytes per second
	ETA             time.Duration        `json:"eta"`
	Error           string               `json:"error,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
	StartedAt       *time.Time           `json:"started_at,omitempty"`
	CompletedAt     *time.Time           `json:"completed_at,omitempty"`
}

// DownloadStatus represents the status of a download task
type DownloadStatus string

const (
	StatusQueued      DownloadStatus = "queued"
	StatusDownloading DownloadStatus = "downloading"
	StatusPaused      DownloadStatus = "paused"
	StatusCompleted   DownloadStatus = "completed"
	StatusFailed      DownloadStatus = "failed"
	StatusCancelled   DownloadStatus = "cancelled"
	StatusProcessing  DownloadStatus = "processing" // Converting/embedding subtitles
)

// String returns the string representation of DownloadStatus
func (s DownloadStatus) String() string {
	return string(s)
}

// IsActive returns true if the download is in an active state
func (s DownloadStatus) IsActive() bool {
	return s == StatusDownloading || s == StatusProcessing
}

// IsComplete returns true if the download is in a terminal state
func (s DownloadStatus) IsComplete() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}

// DownloadOptions contains options for downloading
type DownloadOptions struct {
	OutputDir         string            `json:"output_dir"`
	FilenameTemplate  string            `json:"filename_template"`
	Quality           providers.Quality `json:"quality"`
	EmbedSubtitles    bool              `json:"embed_subtitles"`
	SubtitleLanguages []string          `json:"subtitle_languages"`
	MaxSpeed          int64             `json:"max_speed"` // bytes per second, 0 = unlimited
	ConcurrentWorkers int               `json:"concurrent_workers"`
	KeepPartial       bool              `json:"keep_partial"` // Keep partial downloads on error
}

// DownloadStats provides statistics about download operations
type DownloadStats struct {
	TotalDownloads       int           `json:"total_downloads"`
	Completed            int           `json:"completed"`
	Failed               int           `json:"failed"`
	InProgress           int           `json:"in_progress"`
	Queued               int           `json:"queued"`
	TotalBytesDownloaded int64         `json:"total_bytes_downloaded"`
	AverageSpeed         int64         `json:"average_speed"`
	TotalTime            time.Duration `json:"total_time"`
}

// SegmentInfo represents information about a video segment (for HLS/DASH)
type SegmentInfo struct {
	Index      int           `json:"index"`
	URL        string        `json:"url"`
	ByteRange  string        `json:"byte_range,omitempty"`
	Duration   time.Duration `json:"duration"`
	Downloaded bool          `json:"downloaded"`
	FilePath   string        `json:"file_path"`
}
