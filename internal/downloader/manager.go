package downloader

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/justchokingaround/greg/internal/config"
	"github.com/justchokingaround/greg/internal/database"
	"github.com/justchokingaround/greg/internal/downloader/tools"
	"github.com/justchokingaround/greg/internal/providers"
	"gorm.io/gorm"
)

// Manager implements the Downloader interface
type Manager struct {
	mu sync.RWMutex

	// Worker pool
	workers  []*worker
	queue    chan *DownloadTask
	active   map[string]*activeDownload // task ID -> active download info
	workerWg sync.WaitGroup             // Wait group for workers

	// State
	running bool
	ctx     context.Context
	cancel  context.CancelFunc

	// Callbacks
	onProgress func(DownloadTask)
	onComplete func(DownloadTask)
	onError    func(DownloadTask, error)

	// Configuration
	config *config.DownloadsConfig

	// Logger
	logger *slog.Logger

	// Database
	db *gorm.DB

	// Tools (still maintained for backward compatibility)
	ytdlp  *tools.ToolInfo
	ffmpeg *tools.ToolInfo
}

// activeDownload tracks an in-progress download
type activeDownload struct {
	task     *DownloadTask
	workerID int
	cancel   context.CancelFunc
}

// NewManager creates a new download manager
func NewManager(db *gorm.DB, cfg *config.DownloadsConfig, logger *slog.Logger) (*Manager, error) {
	if db == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Use default logger if none provided (backward compatibility)
	if logger == nil {
		logger = slog.Default()
	}

	// Detect available tools (for backward compatibility)
	ytdlp, ffmpeg, err := tools.DetectTools()
	if err != nil {
		// Log the error but don't fail - we can use native implementation
		logger.Warn("failed to detect download tools, using native implementation", "error", err)
		// Create empty tool info objects
		ytdlp = &tools.ToolInfo{Type: tools.ToolYTDLP, Available: false}
		ffmpeg = &tools.ToolInfo{Type: tools.ToolFFmpeg, Available: false}
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(cfg.Path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		queue:  make(chan *DownloadTask, 100), // Buffered queue
		active: make(map[string]*activeDownload),
		config: cfg,
		logger: logger,
		db:     db,
		ytdlp:  ytdlp,
		ffmpeg: ffmpeg,
		ctx:    ctx,
		cancel: cancel,
	}

	// Load existing queued/paused downloads from database
	if err := m.loadQueueFromDB(); err != nil {
		return nil, fmt.Errorf("failed to load queue from database: %w", err)
	}

	return m, nil
}

// Start starts the download manager and worker pool
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("manager already running")
	}

	m.running = true
	m.startWorkerPool()

	return nil
}

// Stop stops the download manager and all workers
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	m.running = false

	// Cancel all active downloads
	for _, ad := range m.active {
		if ad.cancel != nil {
			ad.cancel()
		}
	}

	// Cancel context to stop workers
	if m.cancel != nil {
		m.cancel()
	}

	// Close queue channel
	close(m.queue)

	// Wait for all workers to finish
	m.workerWg.Wait()

	// Update all active downloads to paused in database
	for _, ad := range m.active {
		ad.task.Status = StatusPaused
		_ = m.updateTaskInDB(*ad.task)
	}

	return nil
}

// AddToQueue adds a new download task to the queue
func (m *Manager) AddToQueue(ctx context.Context, task DownloadTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate by MediaID + Episode (before generating new ID)
	// This prevents re-downloading the same episode
	var existingDownload database.Download
	err := m.db.Where("media_id = ? AND episode = ? AND season = ?",
		task.MediaID, task.Episode, task.Season).
		First(&existingDownload).Error
	if err == nil {
		// Episode exists - check if we should skip it
		if existingDownload.Status != string(StatusFailed) &&
			existingDownload.Status != string(StatusCancelled) {
			return fmt.Errorf("episode %d already in queue or downloaded (status: %s)",
				task.Episode, existingDownload.Status)
		}
		// Episode failed/cancelled - delete old entry and re-add
		m.db.Delete(&existingDownload)
	}

	// Generate unique ID if not provided
	if task.ID == "" {
		task.ID = uuid.New().String()
	}

	// Set timestamps
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}

	// Set initial status
	task.Status = StatusQueued
	task.Progress = 0

	// Check if already in queue or active
	if _, exists := m.active[task.ID]; exists {
		return fmt.Errorf("task already in queue: %s", task.ID)
	}

	// Validate stream URL is not empty
	if task.StreamURL == "" {
		return fmt.Errorf("stream URL is empty for episode %d", task.Episode)
	}

	// Check disk space
	if m.config.MinFreeSpace > 0 {
		if err := m.checkDiskSpace(); err != nil {
			return fmt.Errorf("insufficient disk space: %w", err)
		}
	}

	// Generate output path from template
	template := GetTemplateForMediaType(
		task.MediaType,
		m.config.AnimeFilenameTemplate,
		m.config.MovieFilenameTemplate,
		m.config.FilenameTemplate,
	)

	filename, err := ParseTemplate(template, task)
	if err != nil {
		return fmt.Errorf("failed to parse filename template: %w", err)
	}

	// Create proper folder structure based on media type
	var outputPath string
	switch task.MediaType {
	case providers.MediaTypeAnime:
		// anime/Title/Episode.mkv
		showDir := SanitizeFilename(task.MediaTitle)
		outputPath = filepath.Join(m.config.Path, "anime", showDir, filename)

	case providers.MediaTypeMovie:
		// movies/Title (Year).mkv
		outputPath = filepath.Join(m.config.Path, "movies", filename)

	case providers.MediaTypeTV:
		// tv/Title/Season NN/Episode.mkv
		showDir := SanitizeFilename(task.MediaTitle)
		if task.Season > 0 {
			seasonDir := fmt.Sprintf("Season %02d", task.Season)
			outputPath = filepath.Join(m.config.Path, "tv", showDir, seasonDir, filename)
		} else {
			// No season info, just use show folder
			outputPath = filepath.Join(m.config.Path, "tv", showDir, filename)
		}

	case providers.MediaTypeManga:
		// manga/Title/Chapter.cbz
		showDir := SanitizeFilename(task.MediaTitle)
		outputPath = filepath.Join(m.config.Path, "manga", showDir, filename)

	default:
		// Fallback to downloads folder
		outputPath = filepath.Join(m.config.Path, "downloads", filename)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	task.OutputPath = EnsureUniqueFilename(outputPath)

	// Set embed subtitles from config if not explicitly set
	if !task.EmbedSubs {
		task.EmbedSubs = m.config.EmbedSubtitles
	}

	// Save to database
	if err := m.addTaskToDB(task); err != nil {
		return fmt.Errorf("failed to save task to database: %w", err)
	}

	// Add to queue if manager is running
	if m.running {
		select {
		case m.queue <- &task:
			// Task added successfully
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Queue is full, but task is saved to database
			// It will be picked up when queue has space
		}
	}

	return nil
}

// RemoveFromQueue removes a task from the queue
func (m *Manager) RemoveFromQueue(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if task is active
	if ad, exists := m.active[id]; exists {
		// Cancel the download
		if ad.cancel != nil {
			ad.cancel()
		}
		delete(m.active, id)
	}

	// Update database
	return m.deleteTaskFromDB(id)
}

// GetQueue returns all tasks in the queue, sorted by media title and episode number
func (m *Manager) GetQueue(ctx context.Context) ([]DownloadTask, error) {
	var downloads []database.Download
	// Sort by media title, then by season, then by episode number
	if err := m.db.Order("media_title ASC, season ASC, episode ASC").Find(&downloads).Error; err != nil {
		return nil, fmt.Errorf("failed to get queue from database: %w", err)
	}

	tasks := make([]DownloadTask, 0, len(downloads))
	for _, d := range downloads {
		tasks = append(tasks, m.downloadToTask(d))
	}

	return tasks, nil
}

// HasActiveDownloads returns true if there are any downloads currently in progress
func (m *Manager) HasActiveDownloads() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check if there are any active downloads
	if len(m.active) > 0 {
		return true
	}

	// Also check database for downloads that might be in active states
	var count int64
	m.db.Model(&database.Download{}).
		Where("status IN ?", []string{
			string(StatusDownloading),
			string(StatusProcessing),
			string(StatusQueued),
		}).
		Count(&count)

	return count > 0
}

// ClearQueue clears all non-active tasks from the queue
func (m *Manager) ClearQueue(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get all tasks from database
	var downloads []database.Download
	if err := m.db.Find(&downloads).Error; err != nil {
		return fmt.Errorf("failed to get tasks: %w", err)
	}

	// Delete non-active tasks
	for _, d := range downloads {
		if _, isActive := m.active[d.ID]; !isActive {
			// Only delete queued, completed, failed, or cancelled tasks
			status := DownloadStatus(d.Status)
			if status == StatusQueued || status.IsComplete() {
				if err := m.db.Delete(&d).Error; err != nil {
					return fmt.Errorf("failed to delete task %s: %w", d.ID, err)
				}
			}
		}
	}

	return nil
}

// Pause pauses a specific download
func (m *Manager) Pause(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ad, exists := m.active[id]
	if !exists {
		return fmt.Errorf("task not found or not active: %s", id)
	}

	// Cancel the download context
	if ad.cancel != nil {
		ad.cancel()
	}

	// Update status
	ad.task.Status = StatusPaused
	return m.updateTaskInDB(*ad.task)
}

// Resume resumes a paused download
func (m *Manager) Resume(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get task from database
	var download database.Download
	if err := m.db.First(&download, "id = ?", id).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	task := m.downloadToTask(download)
	if task.Status != StatusPaused {
		return fmt.Errorf("task is not paused: %s", id)
	}

	// Update status and re-queue
	task.Status = StatusQueued
	if err := m.updateTaskInDB(task); err != nil {
		return err
	}

	// Add back to queue if running
	if m.running {
		select {
		case m.queue <- &task:
			// Task re-queued successfully
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// PauseAll pauses all active downloads
func (m *Manager) PauseAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, ad := range m.active {
		if ad.cancel != nil {
			ad.cancel()
		}
		ad.task.Status = StatusPaused
		_ = m.updateTaskInDB(*ad.task)
		delete(m.active, id)
	}

	return nil
}

// ResumeAll resumes all paused downloads
func (m *Manager) ResumeAll(ctx context.Context) error {
	var downloads []database.Download
	if err := m.db.Find(&downloads, "status = ?", string(StatusPaused)).Error; err != nil {
		return fmt.Errorf("failed to get paused tasks: %w", err)
	}

	for _, d := range downloads {
		task := m.downloadToTask(d)
		task.Status = StatusQueued
		_ = m.updateTaskInDB(task)

		if m.running {
			select {
			case m.queue <- &task:
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
	}

	return nil
}

// Cancel cancels a download by ID
func (m *Manager) Cancel(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove from active if present
	if ad, exists := m.active[id]; exists {
		if ad.cancel != nil {
			ad.cancel()
		}
		delete(m.active, id)
	}

	// Get from database
	var download database.Download
	if err := m.db.First(&download, "id = ?", id).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	download.Status = string(StatusCancelled)
	if err := m.db.Save(&download).Error; err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	return nil
}

// Retry retries a failed or cancelled download
func (m *Manager) Retry(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get task from database
	var download database.Download
	if err := m.db.First(&download, "id = ?", id).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	// Only retry if failed or cancelled
	if download.Status != string(StatusFailed) && download.Status != string(StatusCancelled) {
		return fmt.Errorf("can only retry failed or cancelled tasks")
	}

	// Reset status to queued
	download.Status = string(StatusQueued)
	if err := m.db.Save(&download).Error; err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	task := m.downloadToTask(download)

	// Add back to queue if running
	if m.running {
		select {
		case m.queue <- &task:
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// DeleteTaskAndFile deletes a task and its associated file
func (m *Manager) DeleteTaskAndFile(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// First cancel if active
	if ad, exists := m.active[id]; exists {
		if ad.cancel != nil {
			ad.cancel()
		}
		delete(m.active, id)
	}

	// Get task to find file path
	var download database.Download
	if err := m.db.First(&download, "id = ?", id).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	// Delete file if exists
	if download.FilePath != "" {
		if err := os.Remove(download.FilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete file: %w", err)
		}
	}

	// Delete from database
	if err := m.db.Delete(&download).Error; err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	return nil
}

// OnProgressUpdate sets the progress update callback
func (m *Manager) OnProgressUpdate(callback func(task DownloadTask)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onProgress = callback
}

// OnDownloadComplete sets the download complete callback
func (m *Manager) OnDownloadComplete(callback func(task DownloadTask)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onComplete = callback
}

// OnDownloadError sets the download error callback
func (m *Manager) OnDownloadError(callback func(task DownloadTask, err error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onError = callback
}

// SetConcurrency sets the number of concurrent download workers
func (m *Manager) SetConcurrency(workers int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if workers < 1 {
		workers = 1
	}
	if workers > 10 {
		workers = 10 // Reasonable maximum
	}

	m.config.Concurrent = workers

	// If running, config takes effect on next Start()
	_ = m.running
}

// SetOutputDir sets the output directory for downloads
func (m *Manager) SetOutputDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config.Path = dir

	// Create directory if it doesn't exist
	_ = os.MkdirAll(dir, 0755)
}

// SetMaxSpeed sets the maximum download speed in bytes per second
func (m *Manager) SetMaxSpeed(bytesPerSecond int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config.MaxSpeed = bytesPerSecond
}

// startWorkerPool starts the worker goroutines
func (m *Manager) startWorkerPool() {
	workerCount := m.config.Concurrent
	if workerCount < 1 {
		workerCount = 3 // Default
	}

	m.workers = make([]*worker, workerCount)
	for i := 0; i < workerCount; i++ {
		w := newWorker(i, m)
		m.workers[i] = w

		m.workerWg.Add(1)
		go func() {
			defer m.workerWg.Done()
			w.run(m.ctx, m.queue)
		}()
	}
}

// loadQueueFromDB loads pending/paused tasks from database
func (m *Manager) loadQueueFromDB() error {
	var downloads []database.Download
	// Load queued, paused, and downloading tasks (in case of crash)
	statuses := []string{
		string(StatusQueued),
		string(StatusPaused),
		string(StatusDownloading),
	}

	if err := m.db.Where("status IN ?", statuses).Find(&downloads).Error; err != nil {
		return fmt.Errorf("failed to load downloads: %w", err)
	}

	// Convert to tasks and optionally auto-resume
	for _, d := range downloads {
		task := m.downloadToTask(d)

		// If auto-resume is enabled and task was downloading, mark as queued
		if m.config.AutoResume && task.Status == StatusDownloading {
			task.Status = StatusQueued
			_ = m.updateTaskInDB(task)
		}

		// Add to queue if status is queued and auto-resume is enabled
		if m.config.AutoResume && task.Status == StatusQueued {
			// Will be picked up by workers when started
			select {
			case m.queue <- &task:
			default:
				// Queue is full, task will remain in database
			}
		}
	}

	return nil
}

// addTaskToDB adds a task to the database
func (m *Manager) addTaskToDB(task DownloadTask) error {
	download := m.taskToDownload(task)
	if err := m.db.Create(&download).Error; err != nil {
		m.logger.Error("FAILED TO CREATE DOWNLOAD IN DB", "error", err, "task_id", task.ID)
		return err
	}
	m.logger.Info("SUCCESSFULLY ADDED TO DB", "task_id", task.ID, "media_title", task.MediaTitle)
	return nil
}

// updateTaskInDB updates a task in the database
func (m *Manager) updateTaskInDB(task DownloadTask) error {
	download := m.taskToDownload(task)
	if err := m.db.Save(&download).Error; err != nil {
		m.logger.Error("FAILED TO UPDATE DOWNLOAD IN DB", "error", err, "task_id", task.ID)
		return err
	}
	m.logger.Debug("updated task in db", "task_id", task.ID, "status", task.Status, "progress", task.Progress)
	return nil
}

// deleteTaskFromDB deletes a task from the database
func (m *Manager) deleteTaskFromDB(id string) error {
	return m.db.Delete(&database.Download{}, "id = ?", id).Error
}

// taskToDownload converts a DownloadTask to database.Download
func (m *Manager) taskToDownload(task DownloadTask) database.Download {
	return database.Download{
		ID:              task.ID,
		MediaID:         task.MediaID,
		MediaTitle:      task.MediaTitle,
		MediaType:       string(task.MediaType),
		Episode:         task.Episode,
		Season:          task.Season,
		Quality:         string(task.Quality),
		Provider:        task.Provider,
		Status:          string(task.Status),
		Progress:        task.Progress,
		BytesDownloaded: task.BytesDownloaded,
		TotalBytes:      task.TotalBytes,
		Speed:           task.Speed,
		Error:           task.Error,
		FilePath:        task.OutputPath,
		CreatedAt:       task.CreatedAt,
		StartedAt:       task.StartedAt,
		CompletedAt:     task.CompletedAt,
	}
}

// downloadToTask converts a database.Download to DownloadTask
func (m *Manager) downloadToTask(download database.Download) DownloadTask {
	return DownloadTask{
		ID:              download.ID,
		MediaID:         download.MediaID,
		MediaTitle:      download.MediaTitle,
		MediaType:       providers.MediaType(download.MediaType),
		Episode:         download.Episode,
		Season:          download.Season,
		Quality:         providers.Quality(download.Quality),
		Provider:        download.Provider,
		Status:          DownloadStatus(download.Status),
		Progress:        download.Progress,
		BytesDownloaded: download.BytesDownloaded,
		TotalBytes:      download.TotalBytes,
		Speed:           download.Speed,
		Error:           download.Error,
		OutputPath:      download.FilePath,
		CreatedAt:       download.CreatedAt,
		StartedAt:       download.StartedAt,
		CompletedAt:     download.CompletedAt,
	}
}

// checkDiskSpace checks if there's enough free disk space
// Platform-specific implementations in diskspace_*.go files

// triggerProgressCallback safely triggers the progress callback
func (m *Manager) triggerProgressCallback(task DownloadTask) {
	m.mu.RLock()
	callback := m.onProgress
	m.mu.RUnlock()

	if callback != nil {
		// Run callback in goroutine to avoid blocking
		go callback(task)
	}
}

// triggerCompleteCallback safely triggers the complete callback
func (m *Manager) triggerCompleteCallback(task DownloadTask) {
	m.mu.RLock()
	callback := m.onComplete
	m.mu.RUnlock()

	if callback != nil {
		go callback(task)
	}
}

// triggerErrorCallback safely triggers the error callback
func (m *Manager) triggerErrorCallback(task DownloadTask, err error) {
	m.mu.RLock()
	callback := m.onError
	m.mu.RUnlock()

	if callback != nil {
		go callback(task, err)
	}
}
