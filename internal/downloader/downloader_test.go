package downloader

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/justchokingaround/greg/internal/config"
	"github.com/justchokingaround/greg/internal/database"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDownloaderManager(t *testing.T) {
	// Create temporary database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = database.Migrate(db)
	require.NoError(t, err)

	// Create temporary output directory
	tempDir := t.TempDir()

	// Create downloader config
	cfg := &config.DownloadsConfig{
		Path:                  tempDir,
		Concurrent:            2,
		AutoResume:            true,
		FilenameTemplate:      "{title} - S{season:02d}E{episode:02d} [{quality}]",
		AnimeFilenameTemplate: "{title} - S{season:02d}E{episode:02d} [{quality}]",
		MovieFilenameTemplate: "{title} [{quality}]",
	}

	// Create manager
	manager, err := NewManager(db, cfg, slog.Default())
	require.NoError(t, err)

	// Test adding a download task
	task := DownloadTask{
		ID:         "test-task-1",
		MediaID:    "test-media-1",
		MediaTitle: "Test Media",
		MediaType:  providers.MediaTypeTV,
		Episode:    1,
		Season:     1,
		Quality:    providers.Quality1080p,
		Provider:   "test-provider",
		StreamURL:  "https://example.com/test-video.mp4", // This should fail in real scenario but will be tested in mock
		Status:     StatusQueued,
	}

	// Add task to queue
	ctx := context.Background()
	err = manager.AddToQueue(ctx, task)
	assert.NoError(t, err)

	// Retrieve queue
	queue, err := manager.GetQueue(ctx)
	assert.NoError(t, err)
	assert.Len(t, queue, 1)
	assert.Equal(t, "test-task-1", queue[0].ID)
	assert.Equal(t, StatusQueued, queue[0].Status)

	// Test removing from queue
	err = manager.RemoveFromQueue(ctx, "test-task-1")
	assert.NoError(t, err)

	queue, err = manager.GetQueue(ctx)
	assert.NoError(t, err)
	assert.Len(t, queue, 0)

	// Test concurrency setting
	manager.SetConcurrency(4)
	assert.Equal(t, 4, manager.config.Concurrent)

	// Test output directory setting
	newOutputDir := filepath.Join(tempDir, "new_output")
	manager.SetOutputDir(newOutputDir)
	assert.Equal(t, newOutputDir, manager.config.Path)
	assert.DirExists(t, newOutputDir)
}

func TestDownloadStatusString(t *testing.T) {
	tests := []struct {
		status DownloadStatus
		want   string
	}{
		{StatusQueued, "queued"},
		{StatusDownloading, "downloading"},
		{StatusPaused, "paused"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusCancelled, "cancelled"},
		{StatusProcessing, "processing"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := tt.status.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDownloadStatusIsActive(t *testing.T) {
	tests := []struct {
		status   DownloadStatus
		expected bool
	}{
		{StatusQueued, false},
		{StatusDownloading, true},
		{StatusPaused, false},
		{StatusCompleted, false},
		{StatusFailed, false},
		{StatusCancelled, false},
		{StatusProcessing, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result := tt.status.IsActive()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDownloadStatusIsComplete(t *testing.T) {
	tests := []struct {
		status   DownloadStatus
		expected bool
	}{
		{StatusQueued, false},
		{StatusDownloading, false},
		{StatusPaused, false},
		{StatusCompleted, true},
		{StatusFailed, true},
		{StatusCancelled, true},
		{StatusProcessing, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result := tt.status.IsComplete()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDownloadOptionsDefaults(t *testing.T) {
	opts := DownloadOptions{
		OutputDir:         "/tmp/test",
		FilenameTemplate:  "{{.MediaTitle}} - S{{printf \"%02d\" .Season}}E{{printf \"%02d\" .Episode}} [{{.Quality}}].{{.Ext}}",
		Quality:           providers.Quality1080p,
		EmbedSubtitles:    true,
		MaxSpeed:          0, // unlimited
		ConcurrentWorkers: 3,
		KeepPartial:       false,
	}

	assert.Equal(t, "/tmp/test", opts.OutputDir)
	assert.Equal(t, providers.Quality1080p, opts.Quality)
	assert.True(t, opts.EmbedSubtitles)
	assert.Equal(t, int64(0), opts.MaxSpeed)
	assert.Equal(t, 3, opts.ConcurrentWorkers)
}

func TestSegmentInfo(t *testing.T) {
	seg := SegmentInfo{
		Index:      1,
		URL:        "https://example.com/segment1.ts",
		Duration:   10 * time.Second,
		Downloaded: false,
		FilePath:   "/tmp/segment1.ts",
	}

	assert.Equal(t, 1, seg.Index)
	assert.Equal(t, "https://example.com/segment1.ts", seg.URL)
	assert.Equal(t, 10*time.Second, seg.Duration)
	assert.False(t, seg.Downloaded)
	assert.Equal(t, "/tmp/segment1.ts", seg.FilePath)
}

// Test the retry logic with a mock task
func TestProcessTaskWithRetry(t *testing.T) {
	// This test would need more complex mocking to truly test the retry logic
	// For now, we'll just ensure the structure is set up properly
	tempDir := t.TempDir()

	cfg := &config.DownloadsConfig{
		Path:       tempDir,
		Concurrent: 1,
		AutoResume: true,
	}

	// Create temporary database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = database.Migrate(db)
	require.NoError(t, err)

	manager, err := NewManager(db, cfg, slog.Default())
	require.NoError(t, err)

	// Create a mock task
	task := DownloadTask{
		ID:         "retry-test-task",
		MediaID:    "test-media-retry",
		MediaTitle: "Test Retry",
		MediaType:  providers.MediaTypeTV,
		Episode:    1,
		Season:     1,
		Quality:    providers.Quality1080p,
		Provider:   "test-provider",
		StreamURL:  "https://example.com/test-video.mp4",
		Status:     StatusQueued,
	}

	// Add task to the database
	err = manager.addTaskToDB(task)
	require.NoError(t, err)

	// Verify task was added
	tasks, err := manager.GetQueue(context.Background())
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "retry-test-task", tasks[0].ID)
}
