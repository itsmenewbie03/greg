// Package downloader provides download functionality using native Go implementation
package downloader

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/justchokingaround/greg/internal/config"
	"github.com/justchokingaround/greg/internal/database"
	"github.com/justchokingaround/greg/internal/downloader/hls"
	"gorm.io/gorm"
)

// NativeDownloader is a download implementation that doesn't rely on external tools
type NativeDownloader struct {
	db     *gorm.DB
	config *config.DownloadsConfig
	logger *slog.Logger

	// Progress and callbacks
	onProgress func(DownloadTask)
	onComplete func(DownloadTask)
	onError    func(DownloadTask, error)
}

// NewNativeDownloader creates a new downloader that uses native Go implementations
func NewNativeDownloader(db *gorm.DB, cfg *config.DownloadsConfig, logger *slog.Logger) (*NativeDownloader, error) {
	if db == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Use default logger if none provided
	if logger == nil {
		logger = slog.Default()
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(cfg.Path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	d := &NativeDownloader{
		db:     db,
		config: cfg,
		logger: logger,
	}

	return d, nil
}

// Download downloads a task using native implementations
func (d *NativeDownloader) Download(ctx context.Context, task *DownloadTask) error {
	// Update task status
	task.Status = StatusDownloading
	now := time.Now()
	task.StartedAt = &now
	_ = d.updateTaskInDB(*task)
	d.triggerProgressCallback(*task)

	// Check if URL is an HLS stream (contains .m3u8)
	if strings.Contains(task.StreamURL, ".m3u8") {
		return d.downloadHLS(ctx, task)
	}

	// For non-HLS content, download directly
	return d.downloadDirect(ctx, task)
}

// downloadHLS downloads HLS content using our native HLS implementation
func (d *NativeDownloader) downloadHLS(ctx context.Context, task *DownloadTask) error {
	// Create a cancellable context for this download
	downloadCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Prepare headers including referer if provided
	requestHeaders := make(map[string]string)
	for k, v := range task.Headers {
		requestHeaders[k] = v
	}

	// Add referer if provided separately
	if task.Referer != "" {
		requestHeaders["Referer"] = task.Referer
	}

	// Create HLS downloader with progress reporting
	hlsDownloader := hls.NewDownloader()

	// Download the HLS stream with progress reporting
	if err := hlsDownloader.DownloadWithProgress(downloadCtx, task.StreamURL, task.OutputPath, requestHeaders, func(downloaded, total int) {
		if total > 0 {
			progress := float64(downloaded) / float64(total) * 100.0
			task.Progress = progress
			task.BytesDownloaded = int64(downloaded) // Approximate
			task.TotalBytes = int64(total)           // Approximate total segments
			d.triggerProgressCallback(*task)
			_ = d.updateTaskInDB(*task)
		}
	}); err != nil {
		return fmt.Errorf("HLS download failed: %w", err)
	}

	// After successful download, update with actual file size
	if info, err := os.Stat(task.OutputPath); err == nil {
		task.TotalBytes = info.Size()
		task.BytesDownloaded = info.Size()
		task.Progress = 100.0
		_ = d.updateTaskInDB(*task)
	}

	return nil
}

// downloadDirect downloads non-HLS content, choosing between concurrent and single-threaded
func (d *NativeDownloader) downloadDirect(ctx context.Context, task *DownloadTask) error {
	// Check if server supports ranges
	req, err := http.NewRequestWithContext(ctx, "HEAD", task.StreamURL, nil)
	if err != nil {
		return d.downloadDirectSingle(ctx, task)
	}

	// Set headers
	if len(task.Headers) > 0 {
		for key, value := range task.Headers {
			req.Header.Set(key, value)
		}
	}
	if task.Referer != "" {
		req.Header.Set("Referer", task.Referer)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return d.downloadDirectSingle(ctx, task)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return d.downloadDirectSingle(ctx, task)
	}

	acceptRanges := resp.Header.Get("Accept-Ranges")
	contentLength := resp.ContentLength

	// Use concurrent download if ranges supported and file is big enough (>10MB)
	if contentLength > 10*1024*1024 && acceptRanges == "bytes" {
		d.logger.Info("using concurrent download", "size", contentLength, "parts", 8)
		return d.downloadDirectConcurrent(ctx, task, contentLength)
	}

	return d.downloadDirectSingle(ctx, task)
}

// downloadDirectConcurrent downloads content using multiple connections
func (d *NativeDownloader) downloadDirectConcurrent(ctx context.Context, task *DownloadTask, totalBytes int64) error {
	// Create output file
	f, err := os.Create(task.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Pre-allocate file size
	if err := f.Truncate(totalBytes); err != nil {
		return fmt.Errorf("failed to allocate file: %w", err)
	}

	task.TotalBytes = totalBytes

	// Configure concurrency
	const numParts = 8
	partSize := totalBytes / numParts

	var wg sync.WaitGroup
	errChan := make(chan error, numParts)
	var downloadedBytes int64 // Atomic counter

	// Create a cancellable context for all parts
	partCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start progress monitor
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		lastUpdate := time.Now()
		lastDownloaded := int64(0)

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-partCtx.Done():
				return
			case <-ticker.C:
				current := atomic.LoadInt64(&downloadedBytes)
				task.BytesDownloaded = current
				task.Progress = float64(current) / float64(totalBytes) * 100.0

				// Calculate speed
				elapsed := time.Since(lastUpdate)
				if elapsed.Seconds() > 0 {
					bytesPerSec := float64(current-lastDownloaded) / elapsed.Seconds()
					task.Speed = int64(bytesPerSec)
				}

				lastUpdate = time.Now()
				lastDownloaded = current

				d.triggerProgressCallback(*task)
				_ = d.updateTaskInDB(*task)
			}
		}
	}()

	// Start workers
	for i := 0; i < numParts; i++ {
		start := int64(i) * partSize
		end := start + partSize - 1
		if i == numParts-1 {
			end = totalBytes - 1
		}

		wg.Add(1)
		go func(partIndex int, start, end int64) {
			defer wg.Done()

			req, err := http.NewRequestWithContext(partCtx, "GET", task.StreamURL, nil)
			if err != nil {
				select {
				case errChan <- err:
				default:
				}
				cancel()
				return
			}

			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

			// Add headers
			if len(task.Headers) > 0 {
				for key, value := range task.Headers {
					req.Header.Set(key, value)
				}
			}
			if task.Referer != "" {
				req.Header.Set("Referer", task.Referer)
			}
			if req.Header.Get("User-Agent") == "" {
				req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
			}

			client := &http.Client{Timeout: 0}
			resp, err := client.Do(req)
			if err != nil {
				select {
				case errChan <- err:
				default:
				}
				cancel()
				return
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
				select {
				case errChan <- fmt.Errorf("unexpected status code: %d", resp.StatusCode):
				default:
				}
				cancel()
				return
			}

			buf := make([]byte, 32*1024)
			offset := start

			for {
				n, err := resp.Body.Read(buf)
				if n > 0 {
					// WriteAt is thread-safe
					if _, wErr := f.WriteAt(buf[:n], offset); wErr != nil {
						select {
						case errChan <- wErr:
						default:
						}
						cancel()
						return
					}
					offset += int64(n)
					atomic.AddInt64(&downloadedBytes, int64(n))
				}
				if err != nil {
					if err == io.EOF {
						break
					}
					select {
					case errChan <- err:
					default:
					}
					cancel()
					return
				}
			}
		}(i, start, end)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		return err
	}

	// Final update
	task.Progress = 100.0
	task.BytesDownloaded = totalBytes
	_ = d.updateTaskInDB(*task)

	return nil
}

// downloadDirectSingle downloads non-HLS content directly (single connection)
func (d *NativeDownloader) downloadDirectSingle(ctx context.Context, task *DownloadTask) error {
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", task.StreamURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers from task
	if len(task.Headers) > 0 {
		for key, value := range task.Headers {
			req.Header.Set(key, value)
		}
	}

	// Set referer if provided
	if task.Referer != "" {
		req.Header.Set("Referer", task.Referer)
	}

	// Set user agent if not already set
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
	}

	// Send request
	client := &http.Client{
		Timeout: 0, // No timeout for downloads (was 30s which caused failures)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Get content length
	task.TotalBytes = resp.ContentLength

	// Create output file
	out, err := os.Create(task.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = out.Close() }()

	// Download with progress tracking
	buffer := make([]byte, 32*1024) // 32KB buffer
	var downloaded int64
	lastUpdate := time.Now()
	lastDownloaded := int64(0)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := out.Write(buffer[:n]); writeErr != nil {
				return fmt.Errorf("failed to write to file: %w", writeErr)
			}
			downloaded += int64(n)
			task.BytesDownloaded = downloaded

			// Update progress
			if task.TotalBytes > 0 {
				task.Progress = float64(downloaded) / float64(task.TotalBytes) * 100.0
			}

			// Calculate speed
			elapsed := time.Since(lastUpdate)
			if elapsed >= 500*time.Millisecond {
				bytesPerSec := float64(downloaded-lastDownloaded) / elapsed.Seconds()
				task.Speed = int64(bytesPerSec)
				lastUpdate = time.Now()
				lastDownloaded = downloaded

				// Trigger progress callback
				d.triggerProgressCallback(*task)

				// Update database frequently for real-time progress
				_ = d.updateTaskInDB(*task)
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading response: %w", err)
		}
	}

	// Close file to ensure data is written
	if err := out.Close(); err != nil {
		return fmt.Errorf("error closing output file: %w", err)
	}

	task.Progress = 100.0
	task.BytesDownloaded = downloaded

	return nil
}

// OnProgressUpdate sets the progress update callback
func (d *NativeDownloader) OnProgressUpdate(callback func(task DownloadTask)) {
	d.onProgress = callback
}

// OnDownloadComplete sets the download complete callback
func (d *NativeDownloader) OnDownloadComplete(callback func(task DownloadTask)) {
	d.onComplete = callback
}

// OnDownloadError sets the download error callback
func (d *NativeDownloader) OnDownloadError(callback func(task DownloadTask, err error)) {
	d.onError = callback
}

// triggerProgressCallback safely triggers the progress callback
func (d *NativeDownloader) triggerProgressCallback(task DownloadTask) {
	if d.onProgress != nil {
		// Run callback in goroutine to avoid blocking
		go d.onProgress(task)
	}
}

// updateTaskInDB updates a task in the database
func (d *NativeDownloader) updateTaskInDB(task DownloadTask) error {
	download := d.taskToDownload(task)
	return d.db.Save(&download).Error
}

// taskToDownload converts a DownloadTask to database.Download
func (d *NativeDownloader) taskToDownload(task DownloadTask) database.Download {
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
