package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/justchokingaround/greg/internal/config"
	providerhttp "github.com/justchokingaround/greg/internal/providers/http"
)

// Client handles communication with the streaming API server
type Client struct {
	baseURL    string
	httpClient *providerhttp.Client
	timeout    time.Duration
	debug      bool
	logger     *slog.Logger
}

// NewClient creates a new API client
func NewClient(cfg *config.Config, logger *slog.Logger) *Client {
	if cfg == nil {
		cfg = &config.Config{
			API: config.APIConfig{
				BaseURL: "http://localhost:8080",
				Timeout: 30 * time.Second,
			},
		}
	}

	if logger == nil {
		logger = slog.Default()
	}

	httpClient := providerhttp.NewClient(providerhttp.ClientConfig{
		Timeout:    cfg.API.Timeout,
		MaxRetries: 3,
		UserAgent:  "greg/1.0",
		Debug:      cfg.Advanced.Debug,
		Logger:     logger,
	})

	return &Client{
		baseURL:    cfg.API.BaseURL,
		httpClient: httpClient,
		timeout:    cfg.API.Timeout,
		debug:      cfg.Advanced.Debug,
		logger:     logger,
	}
}

// Search performs a search query on the API
// mediaType: "anime" or "movies"
// provider: provider name (e.g., "allanime", "hianime", "sflix", "flixhq")
func (c *Client) Search(ctx context.Context, mediaType, provider, query string) (*SearchResponse, error) {
	endpoint := fmt.Sprintf("/api/%s/%s/search", mediaType, provider)
	params := map[string]string{
		"query": query,
	}

	var response SearchResponse
	if err := c.get(ctx, endpoint, params, &response); err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return &response, nil
}

// GetInfo retrieves detailed information about a media item
func (c *Client) GetInfo(ctx context.Context, mediaType, provider, id string) (*InfoResponse, error) {
	// Don't escape the ID as it may contain slashes (e.g., "movie/free-fight-club-hd-19651")
	// The ID is part of the path structure, not a query parameter
	endpoint := fmt.Sprintf("/api/%s/%s/info/%s", mediaType, provider, id)

	var response InfoResponse
	if err := c.get(ctx, endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("get info failed: %w", err)
	}

	return &response, nil
}

// GetServers retrieves available streaming servers for an episode
// This is optional - not all providers use it
func (c *Client) GetServers(ctx context.Context, mediaType, provider, episodeID string) ([]Server, error) {
	// Don't escape the episodeID as it may contain slashes or special characters
	endpoint := fmt.Sprintf("/api/%s/%s/servers/%s", mediaType, provider, episodeID)

	var servers []Server
	if err := c.get(ctx, endpoint, nil, &servers); err != nil {
		return nil, fmt.Errorf("get servers failed: %w", err)
	}

	return servers, nil
}

// GetSources retrieves video sources for an episode
func (c *Client) GetSources(ctx context.Context, mediaType, provider, episodeID string) (*SourcesResponse, error) {
	// For backwards compatibility, also handle combined formats in GetSources
	// Format 1: episodeID|mediaID (e.g., from SFlix)
	if strings.Contains(episodeID, "|") {
		parts := strings.SplitN(episodeID, "|", 2)
		if len(parts) == 2 {
			episodeID = parts[0]
			mediaID := parts[1]
			return c.GetSourcesWithMediaID(ctx, mediaType, provider, episodeID, mediaID)
		}
	} else if strings.Contains(episodeID, "$episode$") {
		// Format: mediaID$episode$episodeID
		parts := strings.SplitN(episodeID, "$episode$", 2)
		if len(parts) == 2 {
			mediaID := parts[0]
			episodeID = parts[1]
			return c.GetSourcesWithMediaID(ctx, mediaType, provider, episodeID, mediaID)
		}
	}

	return c.GetSourcesWithMediaID(ctx, mediaType, provider, episodeID, "")
}

// GetSourcesWithMediaID retrieves video sources for an episode with optional mediaID
func (c *Client) GetSourcesWithMediaID(ctx context.Context, mediaType, provider, episodeID, mediaID string) (*SourcesResponse, error) {
	// Handle combined episode ID formats
	// Format 1: episodeID|mediaID (e.g., from SFlix)
	// Format 2: mediaID$episode$episodeID (e.g., from HiAnime)

	originalEpisodeID := episodeID

	// Check if episodeID is in combined format
	if strings.Contains(originalEpisodeID, "|") {
		// Format: episodeID|mediaID
		parts := strings.SplitN(originalEpisodeID, "|", 2)
		if len(parts) == 2 {
			episodeID = parts[0]
			mediaID = parts[1] // Override provided mediaID if not already set
		}
	} else if strings.Contains(originalEpisodeID, "$episode$") {
		// Format: mediaID$episode$episodeID
		parts := strings.SplitN(originalEpisodeID, "$episode$", 2)
		if len(parts) == 2 {
			mediaID = parts[0]
			episodeID = parts[1] // Override provided episodeID
		}
	}

	// Don't escape the episodeID as it may contain slashes or special characters
	endpoint := fmt.Sprintf("/api/%s/%s/sources/%s", mediaType, provider, episodeID)

	params := make(map[string]string)
	if mediaID != "" {
		params["mediaId"] = mediaID
	}

	fullURL := fmt.Sprintf("%s%s", c.baseURL, endpoint)
	if len(params) > 0 {
		queryParams := url.Values{}
		for k, v := range params {
			queryParams.Add(k, v)
		}
		fullURL = fmt.Sprintf("%s?%s", fullURL, queryParams.Encode())
	}
	if c.debug {
		c.logger.Debug("getting sources", "url", fullURL, "episodeID", episodeID, "mediaID", mediaID)
	}

	var response SourcesResponse
	if err := c.get(ctx, endpoint, params, &response); err != nil {
		return nil, fmt.Errorf("get sources failed: %w", err)
	}

	if c.debug {
		c.logger.Debug("get sources response", "sources", len(response.Sources), "subtitles", len(response.Subtitles))
	}

	return &response, nil
}

// GetSourcesWithParams retrieves video sources with custom parameters (for HiAnime server/category)
func (c *Client) GetSourcesWithParams(ctx context.Context, mediaType, provider, episodeID string, params map[string]string) (*SourcesResponse, error) {
	// Don't escape the episodeID as it may contain slashes or special characters
	endpoint := fmt.Sprintf("/api/%s/%s/sources/%s", mediaType, provider, episodeID)

	fullURL := fmt.Sprintf("%s%s", c.baseURL, endpoint)
	if len(params) > 0 {
		queryParams := url.Values{}
		for k, v := range params {
			queryParams.Add(k, v)
		}
		fullURL = fmt.Sprintf("%s?%s", fullURL, queryParams.Encode())
	}
	if c.debug {
		c.logger.Debug("getting sources with params", "url", fullURL)
	}

	var response SourcesResponse
	if err := c.get(ctx, endpoint, params, &response); err != nil {
		return nil, fmt.Errorf("get sources failed: %w", err)
	}

	if c.debug {
		c.logger.Debug("get sources with params response",
			"sources", len(response.Sources),
			"subtitles", len(response.Subtitles),
			"has_headers", response.Headers != nil)
		if response.Headers != nil {
			c.logger.Debug("response headers",
				"referer", response.Headers.Referer,
				"origin", response.Headers.Origin)
		}
		for i, sub := range response.Subtitles {
			c.logger.Debug("subtitle", "index", i, "lang", sub.Lang, "url", sub.URL)
		}
	}

	return &response, nil
}

// GetMangaPages retrieves pages for a manga chapter
func (c *Client) GetMangaPages(ctx context.Context, mediaType, provider, chapterID string) (*MangaPagesResponse, error) {
	// Don't escape the chapterID as it may contain colons (e.g., "pvry::one-piece::5498414::1")
	endpoint := fmt.Sprintf("/api/%s/%s/chapter/%s", mediaType, provider, chapterID)

	var response MangaPagesResponse
	if err := c.get(ctx, endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("get manga pages failed: %w", err)
	}

	return &response, nil
}

// HealthCheck checks if a provider is available
func (c *Client) HealthCheck(ctx context.Context, provider, mediaType string) error {
	endpoint := fmt.Sprintf("/api/health/%s", provider)
	params := map[string]string{
		"type": mediaType,
	}

	var response map[string]interface{}
	if err := c.get(ctx, endpoint, params, &response); err != nil {
		return err
	}

	if healthy, ok := response["healthy"].(bool); ok && healthy {
		return nil // Everything is fine
	}

	if msg, ok := response["message"].(string); ok && msg != "" {
		return fmt.Errorf("provider is not healthy: %s", msg)
	}

	return fmt.Errorf("provider is not healthy")
}

// get performs a GET request to the API
func (c *Client) get(ctx context.Context, endpoint string, params map[string]string, result interface{}) error {
	// Build full URL
	fullURL := c.baseURL + endpoint

	// Add query parameters
	if len(params) > 0 {
		u, err := url.Parse(fullURL)
		if err != nil {
			return fmt.Errorf("invalid URL: %w", err)
		}

		q := u.Query()
		for key, value := range params {
			q.Set(key, value)
		}
		u.RawQuery = q.Encode()
		fullURL = u.String()
	}

	// Make request
	resp, err := c.httpClient.Get(ctx, fullURL, nil)
	if err != nil {
		return fmt.Errorf("HTTP request failed (is API server running at %s?): %w", c.baseURL, err)
	}

	// Check for error response
	if resp.StatusCode() >= 400 {
		var errorResp ErrorResponse
		if err := json.Unmarshal(resp.Body(), &errorResp); err == nil && errorResp.Error != "" {
			return fmt.Errorf("API error (%d): %s", resp.StatusCode(), errorResp.Error)
		}
		return fmt.Errorf("API error: HTTP %d", resp.StatusCode())
	}

	// Parse response
	if err := json.Unmarshal(resp.Body(), result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return nil
}
