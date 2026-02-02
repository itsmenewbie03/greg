package downloader

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// worker represents a download worker
type worker struct {
	id               int
	manager          *Manager
	logger           *slog.Logger
	currentTask      *DownloadTask
	nativeDownloader *NativeDownloader
}

// newWorker creates a new worker
func newWorker(id int, manager *Manager) *worker {
	// Create worker-scoped logger with worker_id context
	logger := manager.logger.With("worker_id", id)

	// Pass logger to native downloader
	nativeDownloader, err := NewNativeDownloader(manager.db, manager.config, logger)
	if err != nil {
		logger.Error("failed to create native downloader", "error", err)
		nativeDownloader = nil
	}

	return &worker{
		id:               id,
		manager:          manager,
		logger:           logger,
		nativeDownloader: nativeDownloader,
	}
}

// run starts the worker loop
func (w *worker) run(ctx context.Context, queue <-chan *DownloadTask) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-queue:
			if !ok {
				// Queue closed
				return
			}

			// Process the task
			w.currentTask = task
			if err := w.processTask(ctx, task); err != nil {
				task.Status = StatusFailed
				task.Error = err.Error()
				_ = w.manager.updateTaskInDB(*task)
				w.manager.triggerErrorCallback(*task, err)
			}
			w.currentTask = nil
		}
	}
}

// processTask processes a single download task
func (w *worker) processTask(ctx context.Context, task *DownloadTask) error {
	// Create cancellable context for this task
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Register as active download
	w.manager.mu.Lock()
	w.manager.active[task.ID] = &activeDownload{
		task:     task,
		workerID: w.id,
		cancel:   cancel,
	}
	w.manager.mu.Unlock()

	// Cleanup on exit
	defer func() {
		w.manager.mu.Lock()
		delete(w.manager.active, task.ID)
		w.manager.mu.Unlock()
	}()

	// Update status to downloading
	task.Status = StatusDownloading
	now := time.Now()
	task.StartedAt = &now
	_ = w.manager.updateTaskInDB(*task)
	w.manager.triggerProgressCallback(*task)

	// Attempt the download with retries for network-related failures
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			w.logger.Info("retrying download", "attempt", attempt, "max_retries", maxRetries, "task_id", task.ID)
			// Wait a bit before retrying
			select {
			case <-time.After(2 * time.Second):
			case <-taskCtx.Done():
				return taskCtx.Err()
			}
		}

		// Use the native downloader implementation
		if w.nativeDownloader != nil {
			if err := w.nativeDownloader.Download(taskCtx, task); err != nil {
				w.logger.Warn("native download failed, trying external tools", "error", err)
				lastErr = err
			} else {
				// Download succeeded
				break
			}
		} else {
			// Fallback to external tools if native downloader failed to initialize
			w.logger.Info("using external download tools", "reason", "native downloader unavailable")

			// Choose download method - prefer yt-dlp for all streams (it handles HLS/protected streams better)
			// Only use ffmpeg if yt-dlp is unavailable or fails

			// Strategy: Try yt-dlp first for everything
			if w.manager.ytdlp.Available {
				if err := w.downloadWithYTDLP(taskCtx, task); err != nil {
					w.logger.Warn("yt-dlp download failed", "error", err)
					lastErr = err

					// If yt-dlp fails and ffmpeg is available, try ffmpeg as fallback
					if w.manager.ffmpeg.Available {
						w.logger.Info("trying ffmpeg fallback")
						if ffmpegErr := w.downloadWithFFmpeg(taskCtx, task); ffmpegErr != nil {
							// If both fail with 403, try mpv as last resort (works for protected CDNs)
							if strings.Contains(err.Error(), "403") || strings.Contains(ffmpegErr.Error(), "403") {
								w.logger.Warn("both yt-dlp and ffmpeg failed with 403, trying mpv fallback")
								if mpvErr := w.downloadWithMPV(taskCtx, task); mpvErr != nil {
									lastErr = fmt.Errorf("all download methods failed: yt-dlp=%w, ffmpeg=%v, mpv=%v", err, ffmpegErr, mpvErr)
								} else {
									// MPV succeeded
									break
								}
							} else {
								lastErr = fmt.Errorf("both yt-dlp and ffmpeg failed: yt-dlp=%w, ffmpeg=%v", err, ffmpegErr)
							}
						} else {
							// FFmpeg succeeded
							break
						}
					} else {
						return fmt.Errorf("yt-dlp failed and ffmpeg not available: %w", err)
					}
				} else {
					// yt-dlp succeeded
					break
				}
			} else if w.manager.ffmpeg.Available {
				// Only ffmpeg available (yt-dlp not installed)
				w.logger.Info("using ffmpeg", "reason", "yt-dlp not available")
				if err := w.downloadWithFFmpeg(taskCtx, task); err != nil {
					lastErr = fmt.Errorf("ffmpeg download failed: %w", err)
				} else {
					// FFmpeg succeeded
					break
				}
			} else {
				return fmt.Errorf("no download tools available (install yt-dlp or ffmpeg)")
			}
		}

		// If we reach here, the download failed, and we'll retry on the next iteration
		if attempt < maxRetries {
			// Update progress to show retry
			task.Error = fmt.Sprintf("Attempt %d failed: %v. Retrying...", attempt+1, lastErr)
			_ = w.manager.updateTaskInDB(*task)
			w.manager.triggerProgressCallback(*task)
		}
	}

	// Check if all attempts failed
	if lastErr != nil {
		return lastErr
	}

	// Embed subtitles if requested and available
	if task.EmbedSubs && len(task.Subtitles) > 0 {
		task.Status = StatusProcessing
		_ = w.manager.updateTaskInDB(*task)
		w.manager.triggerProgressCallback(*task)

		if err := w.embedSubtitles(taskCtx, task); err != nil {
			w.logger.Warn("failed to embed subtitles", "error", err)
			// Don't fail the entire download, subtitles are optional
		}
	}

	// Mark as completed
	task.Status = StatusCompleted
	task.Progress = 100.0
	completedAt := time.Now()
	task.CompletedAt = &completedAt
	_ = w.manager.updateTaskInDB(*task)
	w.manager.triggerCompleteCallback(*task)

	return nil
}

// downloadWithYTDLP downloads using yt-dlp (with proper headers)
func (w *worker) downloadWithYTDLP(ctx context.Context, task *DownloadTask) error {
	outputPath := task.OutputPath

	// Start with basic flags
	args := []string{
		task.StreamURL,
		"--no-skip-unavailable-fragments",
		"--fragment-retries", "infinite",
		"--retries", "infinite",
		"--hls-use-mpegts", // Use MPEG-TS for HLS to handle live streams better
		"-N", "32",         // Use 32 connections for maximum speed
		"--concurrent-fragments", "16", // Download 16 fragments concurrently
		"-o", outputPath,
	}

	// Add headers from the task
	if task.Referer != "" {
		args = append(args, "--add-header", "Referer:"+task.Referer)
	}

	// Add custom headers
	for key, value := range task.Headers {
		// Make sure the header key is properly formatted
		properKey := capitalizeHeader(key)
		args = append(args, "--add-header", fmt.Sprintf("%s:%s", properKey, value))
	}

	// Add user agent if not already in custom headers
	userAgentSet := false
	for key := range task.Headers {
		if strings.ToLower(key) == "user-agent" {
			userAgentSet = true
			break
		}
	}
	if !userAgentSet && task.Referer == "" {
		// Use a default user agent if no custom one was provided
		args = append(args, "--user-agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	w.logger.Debug("invoking yt-dlp", "stream_url", task.StreamURL, "args", args)

	cmd := exec.CommandContext(ctx, w.manager.ytdlp.Binary, args...)

	// Get stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start yt-dlp: %w", err)
	}

	// Capture stderr
	var stderrBuf strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			stderrBuf.WriteString(line + "\n")
			w.logger.Debug("yt-dlp output", "line", line)
		}
	}()

	// Monitor progress
	done := make(chan bool)
	go func() {
		w.monitorYTDLPProgress(stdout, task)
		done <- true
	}()

	// Wait for completion
	cmdErr := cmd.Wait()
	<-done

	if cmdErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if stderrBuf.Len() > 0 {
			return fmt.Errorf("yt-dlp failed: %w\n%s", cmdErr, stderrBuf.String())
		}
		return fmt.Errorf("yt-dlp failed: %w", cmdErr)
	}

	// Verify file
	info, err := os.Stat(task.OutputPath)
	if err != nil {
		return fmt.Errorf("output file not found: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("downloaded file is empty")
	}

	task.TotalBytes = info.Size()
	task.BytesDownloaded = info.Size()

	return nil
}

// capitalizeHeader converts header names to proper capitalization (e.g., "user-agent" -> "User-Agent")
func capitalizeHeader(header string) string {
	parts := strings.Split(strings.ToLower(header), "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(string(part[0])) + strings.ToLower(part[1:])
		}
	}
	return strings.Join(parts, "-")
}

// monitorYTDLPProgress monitors yt-dlp stdout and updates progress
func (w *worker) monitorYTDLPProgress(stdout io.Reader, task *DownloadTask) {
	scanner := bufio.NewScanner(stdout)

	lastUpdate := time.Now()
	updateInterval := 500 * time.Millisecond

	for scanner.Scan() {
		line := scanner.Text()

		// Parse progress from yt-dlp output
		// Format: [download]  45.0% of 123.45MiB at 1.23MiB/s ETA 00:12
		if strings.Contains(line, "[download]") {
			if err := w.parseYTDLPProgress(line, task); err != nil {
				w.logger.Debug("failed to parse yt-dlp progress", "error", err, "line", line)
				continue
			}

			// Trigger progress callback (rate limited)
			if time.Since(lastUpdate) >= updateInterval {
				w.manager.triggerProgressCallback(*task)
				// Update database every update for real-time progress
				_ = w.manager.updateTaskInDB(*task)
				lastUpdate = time.Now()
			}
		}
	}
}

// parseYTDLPProgress parses progress information from yt-dlp output
func (w *worker) parseYTDLPProgress(line string, task *DownloadTask) error {
	// Extract percentage
	percentPattern := regexp.MustCompile(`(\d+\.?\d*)%`)
	if matches := percentPattern.FindStringSubmatch(line); len(matches) > 1 {
		percent, err := strconv.ParseFloat(matches[1], 64)
		if err == nil {
			task.Progress = percent
		}
	}

	// Extract speed (e.g., "1.23MiB/s" or "456.78KiB/s")
	speedPattern := regexp.MustCompile(`(\d+\.?\d*)(K|M|G)iB/s`)
	if matches := speedPattern.FindStringSubmatch(line); len(matches) > 2 {
		speed, err := strconv.ParseFloat(matches[1], 64)
		if err == nil {
			unit := matches[2]
			switch unit {
			case "K":
				task.Speed = int64(speed * 1024)
			case "M":
				task.Speed = int64(speed * 1024 * 1024)
			case "G":
				task.Speed = int64(speed * 1024 * 1024 * 1024)
			}
		}
	}

	// Extract ETA (e.g., "00:12" or "01:23:45")
	etaPattern := regexp.MustCompile(`ETA\s+(\d{2}):(\d{2})(?::(\d{2}))?`)
	if matches := etaPattern.FindStringSubmatch(line); len(matches) >= 3 {
		hours, _ := strconv.Atoi(matches[1])
		minutes, _ := strconv.Atoi(matches[2])
		seconds := 0
		if len(matches) > 3 && matches[3] != "" {
			seconds, _ = strconv.Atoi(matches[3])
		}
		task.ETA = time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
	}

	return nil
}

// downloadWithFFmpeg downloads using direct HTTP with ffmpeg
func (w *worker) downloadWithFFmpeg(ctx context.Context, task *DownloadTask) error {
	// Check if URL is m3u8 playlist (HLS)
	if strings.Contains(task.StreamURL, ".m3u8") || strings.Contains(task.StreamURL, "m3u8") {
		return w.downloadHLSWithFFmpeg(ctx, task)
	}

	// Direct HTTP download
	outputPath := task.OutputPath + ".part"
	return w.downloadHTTPDirect(ctx, task, outputPath)
}

// downloadHLSWithFFmpeg downloads HLS stream using ffmpeg
func (w *worker) downloadHLSWithFFmpeg(ctx context.Context, task *DownloadTask) error {
	outputPath := task.OutputPath + ".part"

	args := []string{
		"-reconnect", "1", // Enable reconnection for HTTP
		"-reconnect_streamed", "1", // Reconnect streamed data
		"-reconnect_delay_max", "10", // Maximum reconnect delay
		"-timeout", "30000000", // 30 seconds timeout for network operations
	}

	// Add headers if present
	if len(task.Headers) > 0 || task.Referer != "" {
		// Add individual headers using -headers option multiple times
		for key, value := range task.Headers {
			headerStr := fmt.Sprintf("%s: %s", capitalizeHeader(key), value)
			args = append(args, "-headers", headerStr)
		}
		if task.Referer != "" {
			args = append(args, "-headers", fmt.Sprintf("Referer: %s", task.Referer))
		}
	}

	// Determine output format based on file extension (without .part)
	format := "matroska" // default to mkv
	if strings.HasSuffix(task.OutputPath, ".mp4") {
		format = "mp4"
	} else if strings.HasSuffix(task.OutputPath, ".mkv") {
		format = "matroska"
	}

	args = append(args,
		"-i", task.StreamURL,
		"-c", "copy", // Copy streams without re-encoding
		"-bsf:a", "aac_adtstoasc", // Fix AAC streams
		"-movflags", "+faststart", // Enable fast start for better streaming
		"-f", format, // Explicitly specify output format (fixes .part extension issue)
		"-y", // Overwrite output file
		outputPath,
	)

	// Add progress monitoring
	args = append([]string{"-progress", "pipe:1", "-loglevel", "warning"}, args...)

	// Log the command for debugging
	w.logger.Debug("invoking ffmpeg",
		"binary", w.manager.ffmpeg.Binary,
		"args", args,
		"stream_url", task.StreamURL,
		"output", outputPath)

	cmd := exec.CommandContext(ctx, w.manager.ffmpeg.Binary, args...)

	// Get stdout for progress
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Get stderr for error messages
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Capture stderr for error reporting
	var stderrBuf strings.Builder
	stderrChan := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			stderrBuf.WriteString(line + "\n")
			w.logger.Debug("ffmpeg output", "line", line)
		}
		stderrChan <- stderrBuf.String()
	}()

	// Monitor progress
	go w.monitorFFmpegProgress(stdout, task)

	// Wait for completion
	cmdErr := cmd.Wait()
	stderrOutput := <-stderrChan

	if cmdErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Include stderr in error message
		if stderrOutput != "" {
			return fmt.Errorf("ffmpeg exited with error: %w\nOutput: %s", cmdErr, strings.TrimSpace(stderrOutput))
		}
		return fmt.Errorf("ffmpeg exited with error: %w", cmdErr)
	}

	// Rename .part to final file
	if err := os.Rename(outputPath, task.OutputPath); err != nil {
		return fmt.Errorf("failed to rename output file: %w", err)
	}

	return nil
}

// downloadHTTPDirect downloads via direct HTTP request
func (w *worker) downloadHTTPDirect(ctx context.Context, task *DownloadTask, outputPath string) error {
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
		Timeout: 0, // No timeout for downloads
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
	out, err := os.Create(outputPath)
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
				w.manager.triggerProgressCallback(*task)

				// Update database frequently for real-time progress
				_ = w.manager.updateTaskInDB(*task)
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading response: %w", err)
		}
	}

	// Rename .part to final file
	if err := os.Rename(outputPath, task.OutputPath); err != nil {
		return fmt.Errorf("failed to rename output file: %w", err)
	}

	task.Progress = 100.0
	task.BytesDownloaded = downloaded

	return nil
}

// monitorFFmpegProgress monitors ffmpeg progress output
func (w *worker) monitorFFmpegProgress(stdout io.Reader, task *DownloadTask) {
	scanner := bufio.NewScanner(stdout)
	lastUpdate := time.Now()
	updateInterval := 500 * time.Millisecond

	// Track if we've seen any progress updates
	progressSeen := false

	for scanner.Scan() {
		line := scanner.Text()

		// ffmpeg progress format: key=value
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				switch key {
				case "out_time":
					// Update to show progress is happening - convert time to an approximate progress
					// We can't get exact percentage from HLS, so we'll just show indeterminate progress
					// that it's actively downloading
					if !progressSeen {
						task.Progress = 1.0 // Start at 1% to show it's actually started
						progressSeen = true
					} else {
						// Alternate between 1-5% to show it's progressing
						if task.Progress < 5.0 {
							task.Progress += 1.0
						} else {
							task.Progress = 2.0 // Cycle back down to show activity
						}
					}
				case "bitrate":
					// Parse bitrate and potentially calculate file size, though this is approximate
					if bitrateStr := regexp.MustCompile(`(\d+\.?\d*)kbits/s`).FindStringSubmatch(value); len(bitrateStr) > 1 {
						if _, err := strconv.ParseFloat(bitrateStr[1], 64); err == nil {
							// This is just to show that we're getting data, we won't calculate precise progress
							if task.Progress < 10.0 {
								task.Progress += 0.1
							}
						}
					}
				case "total_size":
					if size, err := strconv.ParseInt(value, 10, 64); err == nil && size > 0 {
						task.TotalBytes = size
					}
				case "out_time_us":
					// Update bytes downloaded based on time (rough estimation)
					if timeUs, err := strconv.ParseInt(value, 10, 64); err == nil {
						// For HLS, we can't know the exact size, but we can show it's progressing
						// Just update to show activity
						task.BytesDownloaded = timeUs / 1000000 // Convert microseconds to seconds as rough proxy
					}
				case "progress":
					if value == "continue" {
						// Still downloading, just ensure we're showing activity
						if task.Progress < 5.0 {
							task.Progress += 0.1
						}
					}
				}

				// Update database periodically for real-time progress
				if time.Since(lastUpdate) >= updateInterval {
					w.manager.triggerProgressCallback(*task)
					_ = w.manager.updateTaskInDB(*task)
					lastUpdate = time.Now()
				}
			}
		}
	}
}

// downloadWithMPV downloads using mpv with stream recording (works for CDN-protected streams)
func (w *worker) downloadWithMPV(ctx context.Context, task *DownloadTask) error {
	outputPath := task.OutputPath + ".part"

	args := []string{
		"--no-config",                   // Don't load user config
		"--no-terminal",                 // Don't show terminal output
		"--msg-level=all=error",         // Only show errors
		"--stream-record=" + outputPath, // Record stream to file
		"--no-audio-display",            // Don't show audio visualizations
		"--vo=null",                     // No video output (headless)
		"--ao=null",                     // No audio output (headless)
		"--cache=yes",                   // Enable caching
		"--cache-secs=10",               // Cache 10 seconds
		"--demuxer-max-bytes=50M",       // Max demuxer cache
		"--demuxer-max-back-bytes=50M",  // Max backward cache
	}

	// Add headers if present
	if task.Referer != "" {
		args = append(args, "--http-header-fields=Referer: "+task.Referer)
	}
	for key, value := range task.Headers {
		args = append(args, fmt.Sprintf("--http-header-fields=%s: %s", capitalizeHeader(key), value))
	}

	// Add user agent if not already set
	hasUserAgent := false
	for key := range task.Headers {
		if strings.EqualFold(key, "User-Agent") {
			hasUserAgent = true
			break
		}
	}
	if !hasUserAgent {
		args = append(args, "--user-agent=Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	// Add the stream URL
	args = append(args, task.StreamURL)

	w.logger.Debug("invoking mpv download", "args", args, "stream_url", task.StreamURL)

	cmd := exec.CommandContext(ctx, "mpv", args...)

	// Get stderr for error messages
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start mpv: %w", err)
	}

	// Capture stderr
	var stderrBuf strings.Builder
	stderrChan := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			stderrBuf.WriteString(line + "\n")
			w.logger.Debug("mpv output", "line", line)
		}
		stderrChan <- stderrBuf.String()
	}()

	// Monitor file size for progress
	go func() {
		lastUpdate := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
				if info, err := os.Stat(outputPath); err == nil {
					task.BytesDownloaded = info.Size()

					// Calculate progress (rough estimate since we don't know total size)
					// Just update the UI to show it's downloading
					if time.Since(lastUpdate) >= 500*time.Millisecond {
						w.manager.triggerProgressCallback(*task)
						_ = w.manager.updateTaskInDB(*task)
						lastUpdate = time.Now()
					}
				}
			}
		}
	}()

	// Wait for completion
	cmdErr := cmd.Wait()
	stderrOutput := <-stderrChan

	if cmdErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if stderrOutput != "" {
			return fmt.Errorf("mpv failed: %w\nOutput: %s", cmdErr, strings.TrimSpace(stderrOutput))
		}
		return fmt.Errorf("mpv exited with error: %w", cmdErr)
	}

	// Rename .part to final file
	if err := os.Rename(outputPath, task.OutputPath); err != nil {
		return fmt.Errorf("failed to rename output file: %w", err)
	}

	// Get final file size
	if info, err := os.Stat(task.OutputPath); err == nil {
		task.TotalBytes = info.Size()
		task.BytesDownloaded = info.Size()
	}

	return nil
}

// embedSubtitles embeds subtitles into the video file using ffmpeg
func (w *worker) embedSubtitles(ctx context.Context, task *DownloadTask) error {
	w.logger.Info("EMBEDDING SUBTITLES STARTED",
		"subtitle_count", len(task.Subtitles),
		"output_path", task.OutputPath,
		"embed_subs", task.EmbedSubs)

	if !w.manager.ffmpeg.Available {
		w.logger.Warn("ffmpeg not available for subtitle embedding")
		return fmt.Errorf("ffmpeg not available for subtitle embedding")
	}

	// Download subtitle files
	subFiles := make([]string, 0, len(task.Subtitles))
	defer func() {
		// Cleanup subtitle files
		for _, f := range subFiles {
			_ = os.Remove(f)
		}
	}()

	for i, sub := range task.Subtitles {
		// Download subtitle file
		subPath := filepath.Join(os.TempDir(), fmt.Sprintf("sub_%s_%d.%s", task.ID, i, getSubtitleExtension(sub.URL)))
		w.logger.Info("downloading subtitle",
			"index", i,
			"language", sub.Language,
			"url", sub.URL,
			"path", subPath)
		if err := w.downloadSubtitle(ctx, sub.URL, subPath); err != nil {
			w.logger.Warn("failed to download subtitle", "error", err, "subtitle_index", i, "url", sub.URL)
			continue
		}
		w.logger.Info("subtitle downloaded successfully", "index", i, "path", subPath)
		subFiles = append(subFiles, subPath)
	}

	if len(subFiles) == 0 {
		w.logger.Error("NO SUBTITLES DOWNLOADED - all subtitle downloads failed")
		return fmt.Errorf("no subtitles downloaded")
	}

	w.logger.Info("downloaded subtitle files", "count", len(subFiles))

	// Build ffmpeg command to embed subtitles
	tempOutput := task.OutputPath + ".temp.mkv"

	args := []string{
		"-i", task.OutputPath, // Input video
	}

	// Add subtitle inputs
	for _, subFile := range subFiles {
		args = append(args, "-i", subFile)
	}

	// Map video and audio streams from first input
	args = append(args, "-map", "0:v", "-map", "0:a")

	// Map subtitle files (they're separate text files, not streams)
	for i, subFile := range subFiles {
		args = append(args, "-map", fmt.Sprintf("%d:0", i+1))
		// Add metadata for subtitle language if available
		if i < len(task.Subtitles) {
			lang := task.Subtitles[i].Language
			if lang == "" {
				lang = "eng" // Default to English
			}
			args = append(args, fmt.Sprintf("-metadata:s:s:%d", i), fmt.Sprintf("language=%s", lang))
			args = append(args, fmt.Sprintf("-metadata:s:s:%d", i), fmt.Sprintf("title=%s", task.Subtitles[i].Language))
		}
		_ = subFile // Keep for reference in loop
	}

	// Copy video/audio, but convert subtitles to SRT format for MKV compatibility
	args = append(args, "-c:v", "copy", "-c:a", "copy", "-c:s", "srt")

	// Output
	args = append(args, "-y", tempOutput)

	cmd := exec.CommandContext(ctx, w.manager.ffmpeg.Binary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		w.logger.Error("FFMPEG SUBTITLE EMBEDDING FAILED",
			"error", err,
			"output", string(output),
			"command", fmt.Sprintf("%s %v", w.manager.ffmpeg.Binary, args))
		return fmt.Errorf("ffmpeg subtitle embedding failed: %w, output: %s", err, string(output))
	}

	w.logger.Info("FFMPEG SUBTITLE EMBEDDING SUCCESS")

	// Replace original file with new file
	if err := os.Remove(task.OutputPath); err != nil {
		w.logger.Error("failed to remove original file", "error", err)
		return fmt.Errorf("failed to remove original file: %w", err)
	}
	if err := os.Rename(tempOutput, task.OutputPath); err != nil {
		w.logger.Error("failed to rename temp file", "error", err)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	w.logger.Info("SUBTITLE EMBEDDING COMPLETE", "output_path", task.OutputPath)

	return nil
}

// downloadSubtitle downloads a subtitle file
func (w *worker) downloadSubtitle(ctx context.Context, url, outputPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	return err
}

// getSubtitleExtension returns the file extension for a subtitle URL
func getSubtitleExtension(url string) string {
	if strings.Contains(url, ".vtt") {
		return "vtt"
	}
	if strings.Contains(url, ".ass") {
		return "ass"
	}
	return "srt"
}
