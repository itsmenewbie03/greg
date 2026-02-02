// Package hls provides HLS (HTTP Live Streaming) download functionality
package hls

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Segment represents a single HLS segment
type Segment struct {
	URL      string
	Index    int
	Duration float64
	Title    string
}

// M3U8Playlist represents the HLS playlist structure
type M3U8Playlist struct {
	Version        string
	TargetDuration float64
	MediaSequence  int
	Segments       []Segment
	EndList        bool
	PlaylistType   string
}

// Downloader handles HLS downloads
type Downloader struct {
	client *http.Client
}

// NewDownloader creates a new HLS downloader
func NewDownloader() *Downloader {
	return &Downloader{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Download downloads an HLS stream to the specified output file with concurrent segment downloads
func (d *Downloader) Download(ctx context.Context, url, output string, headers map[string]string) error {
	// Use DownloadWithProgress with a no-op callback for consistency and performance
	return d.DownloadWithProgress(ctx, url, output, headers, nil)
}

// parsePlaylist downloads and parses the M3U8 playlist
func (d *Downloader) parsePlaylist(ctx context.Context, url string, headers map[string]string) (*M3U8Playlist, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Some HLS streams require a proper User-Agent
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)

	// Check if this is a master playlist by looking for STREAM-INF tags
	isMasterPlaylist := false
	var masterPlaylistLines []string

	// First pass: collect all lines and determine if it's a master playlist
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		masterPlaylistLines = append(masterPlaylistLines, line)

		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			isMasterPlaylist = true
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if isMasterPlaylist {
		// For master playlists, select the highest quality stream (largest bandwidth)
		selectedMediaPlaylistURL := d.selectBestStream(masterPlaylistLines, url)
		if selectedMediaPlaylistURL != "" {
			// Recursively parse the selected media playlist
			return d.parseMediaPlaylist(ctx, selectedMediaPlaylistURL, headers)
		} else {
			return nil, fmt.Errorf("no suitable stream found in master playlist")
		}
	} else {
		// It's a media playlist, parse it directly
		return d.parseMediaPlaylistLines(masterPlaylistLines, url, headers)
	}
}

// selectBestStream finds the highest quality stream from a master playlist
func (d *Downloader) selectBestStream(lines []string, baseURL string) string {
	type StreamInfo struct {
		URL       string
		Bandwidth int
	}

	var streams []StreamInfo

	for i, line := range lines {
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			// Parse bandwidth from the tag
			bandwidth := 0
			if bwMatch := regexp.MustCompile(`BANDWIDTH=(\d+)`).FindStringSubmatch(line); len(bwMatch) > 1 {
				if bw, err := strconv.Atoi(bwMatch[1]); err == nil {
					bandwidth = bw
				}
			}

			// Next non-tag line should be the URL
			if i+1 < len(lines) {
				urlLine := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(urlLine, "http") {
					streams = append(streams, StreamInfo{URL: urlLine, Bandwidth: bandwidth})
				} else {
					// Handle relative URL
					if idx := strings.LastIndex(baseURL, "/"); idx != -1 {
						streams = append(streams, StreamInfo{
							URL:       baseURL[:idx+1] + urlLine,
							Bandwidth: bandwidth,
						})
					}
				}
			}
		}
	}

	// Select the stream with the highest bandwidth
	if len(streams) > 0 {
		best := streams[0]
		for _, s := range streams[1:] {
			if s.Bandwidth > best.Bandwidth {
				best = s
			}
		}
		return best.URL
	}

	return ""
}

// parseMediaPlaylist fetches and parses a media playlist (not master)
func (d *Downloader) parseMediaPlaylist(ctx context.Context, url string, headers map[string]string) (*M3U8Playlist, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Some HLS streams require a proper User-Agent
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, strings.TrimSpace(scanner.Text()))
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return d.parseMediaPlaylistLines(lines, url, headers)
}

// parseMediaPlaylistLines parses lines from a media playlist
func (d *Downloader) parseMediaPlaylistLines(lines []string, url string, headers map[string]string) (*M3U8Playlist, error) {
	playlist := &M3U8Playlist{
		Segments: make([]Segment, 0),
	}

	segmentIndex := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "#EXTM3U") {
			continue // Header
		} else if strings.HasPrefix(line, "#EXT-X-VERSION:") {
			playlist.Version = strings.TrimPrefix(line, "#EXT-X-VERSION:")
		} else if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
			durationStr := strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:")
			duration, err := strconv.ParseFloat(durationStr, 64)
			if err == nil {
				playlist.TargetDuration = duration
			}
		} else if strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:") {
			seqStr := strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:")
			seq, err := strconv.Atoi(seqStr)
			if err == nil {
				playlist.MediaSequence = seq
			}
		} else if strings.HasPrefix(line, "#EXT-X-PLAYLIST-TYPE:") {
			playlist.PlaylistType = strings.TrimPrefix(line, "#EXT-X-PLAYLIST-TYPE:")
		} else if strings.HasPrefix(line, "#EXT-X-ENDLIST") {
			playlist.EndList = true
		} else if strings.HasPrefix(line, "#EXTINF:") {
			// Parse duration and title
			infLine := strings.TrimPrefix(line, "#EXTINF:")
			parts := strings.SplitN(infLine, ",", 2)
			var duration float64
			if len(parts) > 0 {
				duration, _ = strconv.ParseFloat(strings.TrimRight(parts[0], ", "), 64)
			}

			var title string
			if len(parts) > 1 {
				title = strings.TrimSpace(parts[1])
			}

			// Next line should be the URL
			if i+1 < len(lines) {
				segmentURL := strings.TrimSpace(lines[i+1])
				if segmentURL != "" {
					// Handle relative URLs
					if !strings.HasPrefix(segmentURL, "http") {
						// If it's a relative URL, make it absolute using the playlist URL as base
						baseURL := url
						if idx := strings.LastIndex(baseURL, "/"); idx != -1 {
							baseURL = baseURL[:idx+1]
						} else {
							baseURL = baseURL + "/"
						}
						segmentURL = baseURL + segmentURL
					}

					playlist.Segments = append(playlist.Segments, Segment{
						URL:      segmentURL,
						Index:    segmentIndex,
						Duration: duration,
						Title:    title,
					})
					segmentIndex++
				}
			}
		}
	}

	return playlist, nil
}

// downloadSegment downloads a single segment
func (d *Downloader) downloadSegment(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	// Try multiple attempts for segment download
	maxRetries := 2 // Lower number of retries per segment to avoid long delays
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}

		// Add custom headers
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		// Some HLS streams require a proper User-Agent
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		}

		resp, err := d.client.Do(req)
		if err != nil {
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond) // Exponential backoff
				continue
			}
			return nil, err
		}

		// Process the response
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close() // Close immediately after reading

		if err != nil {
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond) // Exponential backoff
				continue
			}
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond) // Exponential backoff
				continue
			}
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
		}

		return body, nil
	}

	return nil, fmt.Errorf("failed to download segment after %d attempts", maxRetries+1)
}

// DownloadToTask downloads HLS content to a downloader task
// Instead of importing the downloader package, we'll pass necessary values directly
func DownloadToTask(ctx context.Context, streamURL, outputPath string, headers map[string]string) error {
	downloader := NewDownloader()

	// Create a temporary file for the download
	tempPath := outputPath + ".tmp"

	// Download the HLS stream
	err := downloader.Download(ctx, streamURL, tempPath, headers)
	if err != nil {
		// Clean up the temporary file (ignore errors - file may not exist)
		_ = os.Remove(tempPath)
		return err
	}

	// Rename the temporary file to the final output
	if err := os.Rename(tempPath, outputPath); err != nil {
		// Clean up the temporary file (ignore errors - file may not exist)
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename download: %w", err)
	}

	return nil
}

// ProgressCallback is a function that reports download progress
type ProgressCallback func(downloaded, total int)

// DownloadWithProgress downloads HLS content with progress reporting
func (d *Downloader) DownloadWithProgress(ctx context.Context, url, output string, headers map[string]string, progressCallback ProgressCallback) error {
	playlist, err := d.parsePlaylist(ctx, url, headers)
	if err != nil {
		return fmt.Errorf("failed to parse playlist: %w", err)
	}

	if len(playlist.Segments) == 0 {
		return fmt.Errorf("playlist has no segments to download")
	}

	// Create output directory if it doesn't exist
	if err = os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Open the output file for writing/appending
	outFile, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create/open output file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	// Set up progress tracking
	totalSegments := len(playlist.Segments)
	var downloadedSegments int32

	// Report initial progress
	if progressCallback != nil {
		progressCallback(0, totalSegments)
	}

	// Concurrent download configuration
	const maxWorkers = 8

	// Channel for segments to download
	type job struct {
		index   int
		segment Segment
	}
	jobs := make(chan job, totalSegments)

	// Channel for results
	type result struct {
		index int
		data  []byte
		err   error
	}
	results := make(chan result, totalSegments)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				// Check context
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Handle relative URLs
				segmentURL := j.segment.URL
				if !strings.HasPrefix(segmentURL, "http") {
					baseURL := url
					if idx := strings.LastIndex(baseURL, "/"); idx != -1 {
						baseURL = baseURL[:idx+1]
					} else {
						baseURL = baseURL + "/"
					}
					segmentURL = baseURL + segmentURL
				}

				data, err := d.downloadSegment(ctx, segmentURL, headers)
				results <- result{index: j.index, data: data, err: err}
			}
		}()
	}

	// Fill job queue
	for i, segment := range playlist.Segments {
		jobs <- job{index: i, segment: segment}
	}
	close(jobs)

	// Collect results and write in order
	// We need to buffer results because they might arrive out of order
	segmentBuffer := make(map[int][]byte)
	nextIndex := 0
	var firstErr error

	// Process results
	for i := 0; i < totalSegments; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res := <-results:
			if res.err != nil {
				if firstErr == nil {
					firstErr = res.err
				}
				continue
			}

			segmentBuffer[res.index] = res.data
			atomic.AddInt32(&downloadedSegments, 1)

			// Write available sequential segments
			for {
				data, ok := segmentBuffer[nextIndex]
				if !ok {
					break
				}

				if _, err := outFile.Write(data); err != nil {
					if firstErr == nil {
						firstErr = fmt.Errorf("failed to write segment %d: %w", nextIndex, err)
					}
				}

				// Free memory
				delete(segmentBuffer, nextIndex)
				nextIndex++
			}

			// Report progress
			if progressCallback != nil {
				progressCallback(int(downloadedSegments), totalSegments)
			}
		}
	}

	// Wait for workers to finish (they should be done as we consumed all results)
	wg.Wait()

	if firstErr != nil {
		return firstErr
	}

	return nil
}
