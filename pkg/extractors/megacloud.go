package extractors

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/justchokingaround/greg/pkg/types"
)

// MegaCloudExtractor handles extraction from MegaCloud/VidCloud/UpCloud servers using crawlr.cc
type MegaCloudExtractor struct {
	client *http.Client
}

// crawlrResponse represents the JSON response from crawlr.cc
type crawlrResponse struct {
	Sources []struct {
		URL     string `json:"url"`  // crawlr.cc uses "url" field
		File    string `json:"file"` // fallback for other formats
		Quality string `json:"quality"`
		Type    string `json:"type"`
	} `json:"sources"`
	Tracks []struct {
		URL     string `json:"url"`  // crawlr.cc uses "url" field
		File    string `json:"file"` // fallback for other formats
		Lang    string `json:"lang"` // crawlr.cc uses "lang" field
		Kind    string `json:"kind"` // fallback for "captions"/"subtitles"
		Label   string `json:"label"`
		Default bool   `json:"default"`
	} `json:"tracks"`
	Headers struct {
		Referer   string `json:"Referer"`
		UserAgent string `json:"User-Agent"`
	} `json:"headers"`
}

// NewMegaCloudExtractor creates a new MegaCloud extractor instance
func NewMegaCloudExtractor() *MegaCloudExtractor {
	return &MegaCloudExtractor{
		client: &http.Client{
			Timeout: 30 * time.Second, // Increase timeout for crawlr.cc
		},
	}
}

// Extract extracts video sources and subtitles from a MegaCloud embed URL using crawlr.cc
// This uses the external crawlr.cc service with provider ID mapping
func (m *MegaCloudExtractor) Extract(targetURL string) (*types.VideoSources, error) {
	// Get the provider ID for megacloud
	// Hardcoded here to avoid dependency on config package
	providerID := "9D7F1B3E8" // megacloud ID

	// Construct the crawlr.cc URL
	crawlrURL := fmt.Sprintf("https://crawlr.cc/%s?url=%s", providerID, url.QueryEscape(targetURL))

	// Make request to crawlr.cc
	req, err := http.NewRequest("GET", crawlrURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed creating crawlr request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed fetching from crawlr.cc: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crawlr.cc returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading crawlr response: %w", err)
	}

	// Parse the JSON response
	var data crawlrResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed parsing crawlr response: %w", err)
	}

	// Convert to our standard format
	var videoSources []types.Source
	var subtitles []types.Subtitle

	// Extract referer from the embed URL (e.g., https://megacloud.blog/embed-2/... -> https://megacloud.blog/)
	// The video server requires the embed host as referer, not the source site
	referer := ""
	parsedURL, err := url.Parse(targetURL)
	if err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
		referer = fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host)
	}

	// Extract video sources
	for _, s := range data.Sources {
		// Try URL field first (crawlr.cc format), then File field (other formats)
		sourceURL := s.URL
		if sourceURL == "" {
			sourceURL = s.File
		}

		if sourceURL == "" {
			continue
		}

		isM3U8 := strings.HasSuffix(sourceURL, ".m3u8")
		quality := s.Quality
		if quality == "" {
			quality = "auto"
		}

		videoSources = append(videoSources, types.Source{
			URL:     sourceURL,
			Quality: quality,
			IsM3U8:  isM3U8,
			Referer: referer,
		})
	}

	// Extract subtitle tracks (only captions, not thumbnails)
	for _, t := range data.Tracks {
		// Get subtitle URL (try URL field first, then File field)
		subtitleURL := t.URL
		if subtitleURL == "" {
			subtitleURL = t.File
		}

		// Skip if no URL found or if it's a thumbnail
		if subtitleURL == "" || t.Kind == "thumbnails" {
			continue
		}

		// Accept tracks with lang field (crawlr format) or kind field (other formats)
		hasLang := t.Lang != ""
		isSubtitle := t.Kind == "captions" || t.Kind == "subtitles"

		if hasLang || isSubtitle {
			// Prefer lang field, fallback to label
			lang := t.Lang
			if lang == "" {
				lang = t.Label
			}

			subtitles = append(subtitles, types.Subtitle{
				URL:  subtitleURL,
				Lang: strings.ToLower(lang),
			})
		}
	}

	if len(videoSources) == 0 {
		return nil, fmt.Errorf("no video sources found in crawlr.cc response")
	}

	return &types.VideoSources{
		Sources:   videoSources,
		Subtitles: subtitles,
	}, nil
}
