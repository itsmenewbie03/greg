package downloader

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/justchokingaround/greg/internal/database"
	"github.com/justchokingaround/greg/internal/providers"
)

// MangaDownloadTask represents a manga chapter download task
type MangaDownloadTask struct {
	ID           string
	MediaID      string
	MediaTitle   string
	ChapterID    string
	ChapterTitle string
	ChapterNum   int
	Provider     string
	Pages        []string
	Headers      map[string]string
	Referer      string
	OutputPath   string
	Format       MangaFormat
	Status       DownloadStatus
	Progress     float64
	Error        string
	CreatedAt    time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

// MangaFormat represents manga output format
type MangaFormat string

const (
	FormatCBZ MangaFormat = "cbz" // Comic Book Archive
	FormatPDF MangaFormat = "pdf" // PDF format
	FormatDir MangaFormat = "dir" // Directory with images
)

// DownloadMangaChapter downloads a manga chapter and saves it in the specified format
func (m *Manager) DownloadMangaChapter(ctx context.Context, provider providers.MangaProvider, task *MangaDownloadTask) error {
	m.logger.Info("downloading manga chapter", "chapter", task.ChapterTitle, "pages", len(task.Pages))

	// Create temp directory for images
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("greg-manga-%s", task.ID))
	defer func() { _ = os.RemoveAll(tempDir) }()

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Download all pages concurrently
	type downloadResult struct {
		index int
		path  string
		err   error
	}

	results := make(chan downloadResult, len(task.Pages))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // Limit concurrent downloads to 5

	for i, pageURL := range task.Pages {
		wg.Add(1)
		go func(index int, url string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Download page
			pagePath := filepath.Join(tempDir, fmt.Sprintf("page_%04d.jpg", index+1))
			err := m.downloadMangaPage(ctx, url, pagePath, task.Headers, task.Referer)

			results <- downloadResult{
				index: index,
				path:  pagePath,
				err:   err,
			}

			// Update progress
			progress := float64(index+1) / float64(len(task.Pages)) * 100
			task.Progress = progress

			if m.onProgress != nil {
				// Convert MangaDownloadTask to DownloadTask for callback
				dt := m.mangaTaskToDownloadTask(task)
				m.onProgress(dt)
			}
		}(i, pageURL)
	}

	wg.Wait()
	close(results)

	// Check for errors
	downloadedPages := make([]string, len(task.Pages))
	for result := range results {
		if result.err != nil {
			return fmt.Errorf("failed to download page %d: %w", result.index+1, result.err)
		}
		downloadedPages[result.index] = result.path
	}

	// Create output based on format
	switch task.Format {
	case FormatCBZ:
		return m.createCBZ(downloadedPages, task.OutputPath)
	case FormatDir:
		return m.createImageDir(downloadedPages, task.OutputPath)
	case FormatPDF:
		return fmt.Errorf("PDF format not yet implemented")
	default:
		return fmt.Errorf("unsupported format: %s", task.Format)
	}
}

// downloadMangaPage downloads a single manga page
func (m *Manager) downloadMangaPage(ctx context.Context, url, outputPath string, headers map[string]string, referer string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	// Add headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Create output file
	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	return err
}

// createCBZ creates a CBZ (Comic Book Archive) file from images
func (m *Manager) createCBZ(imagePaths []string, outputPath string) error {
	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create CBZ file (which is just a ZIP file)
	zipFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create CBZ file: %w", err)
	}
	defer func() { _ = zipFile.Close() }()

	zipWriter := zip.NewWriter(zipFile)
	defer func() { _ = zipWriter.Close() }()

	// Add each image to the archive
	for _, imgPath := range imagePaths {
		if err := m.addFileToZip(zipWriter, imgPath); err != nil {
			return fmt.Errorf("failed to add image to CBZ: %w", err)
		}
	}

	return nil
}

// addFileToZip adds a file to a zip archive
func (m *Manager) addFileToZip(zipWriter *zip.Writer, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	fileName := filepath.Base(filePath)
	writer, err := zipWriter.Create(fileName)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}

// createImageDir creates a directory with all downloaded images
func (m *Manager) createImageDir(imagePaths []string, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	for _, srcPath := range imagePaths {
		fileName := filepath.Base(srcPath)
		dstPath := filepath.Join(outputDir, fileName)

		if err := m.copyFile(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy image: %w", err)
		}
	}

	return nil
}

// copyFile copies a file from src to dst
func (m *Manager) copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = destination.Close() }()

	_, err = io.Copy(destination, source)
	return err
}

// mangaTaskToDownloadTask converts a MangaDownloadTask to DownloadTask for callbacks
func (m *Manager) mangaTaskToDownloadTask(mt *MangaDownloadTask) DownloadTask {
	return DownloadTask{
		ID:              mt.ID,
		MediaID:         mt.MediaID,
		MediaTitle:      mt.MediaTitle,
		MediaType:       providers.MediaTypeManga,
		Episode:         mt.ChapterNum,
		Provider:        mt.Provider,
		OutputPath:      mt.OutputPath,
		Status:          mt.Status,
		Progress:        mt.Progress,
		Error:           mt.Error,
		CreatedAt:       mt.CreatedAt,
		StartedAt:       mt.StartedAt,
		CompletedAt:     mt.CompletedAt,
		Headers:         mt.Headers,
		Referer:         mt.Referer,
		BytesDownloaded: 0,
		TotalBytes:      int64(len(mt.Pages)),
	}
}

// AddMangaToQueue adds a manga chapter download to the queue
func (m *Manager) AddMangaToQueue(ctx context.Context, provider providers.MangaProvider, task *MangaDownloadTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Set default format if not specified
	if task.Format == "" {
		task.Format = FormatCBZ
	}

	// Generate output path if not set
	if task.OutputPath == "" {
		sanitizedTitle := sanitizeFilename(task.MediaTitle)
		filename := fmt.Sprintf("%s - Chapter %d.cbz", sanitizedTitle, task.ChapterNum)
		task.OutputPath = filepath.Join(m.config.Path, "manga", sanitizedTitle, filename)
	}

	// Set status to queued if not already set (for new tasks)
	// Don't override if already completed (for already downloaded tasks)
	if task.Status == "" {
		task.Status = StatusQueued
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}

	// Store in database
	dbTask := database.Download{
		ID:          task.ID,
		MediaID:     task.MediaID,
		MediaTitle:  task.MediaTitle,
		MediaType:   string(providers.MediaTypeManga),
		Episode:     task.ChapterNum,
		Provider:    task.Provider,
		FilePath:    task.OutputPath,
		Status:      string(task.Status),
		Progress:    task.Progress,
		TotalBytes:  int64(len(task.Pages)), // Number of pages
		CreatedAt:   task.CreatedAt,
		StartedAt:   task.StartedAt,
		CompletedAt: task.CompletedAt,
	}

	if err := m.db.Create(&dbTask).Error; err != nil {
		return fmt.Errorf("failed to save task to database: %w", err)
	}

	m.logger.Info("added manga chapter to queue", "chapter", task.ChapterTitle)
	return nil
}

// sanitizeFilename removes invalid characters from filenames
func sanitizeFilename(name string) string {
	// Replace invalid characters
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "",
		"?", "",
		"\"", "",
		"<", "",
		">", "",
		"|", "",
	)
	return replacer.Replace(name)
}
