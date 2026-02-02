package watchparty

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"

	"github.com/justchokingaround/greg/internal/providers"
)

// Config holds the WatchParty configuration
type Config struct {
	Enabled         bool   `json:"enabled" yaml:"enabled"`
	DefaultProxy    string `json:"default_proxy" yaml:"default_proxy"`
	AutoOpenBrowser bool   `json:"auto_open_browser" yaml:"auto_open_browser"`
	DefaultOrigin   string `json:"default_origin" yaml:"default_origin"`
}

// ProxyConfig holds the m3u8 proxy configuration
type ProxyConfig struct {
	ProxyURL string
	Origin   string
	Referer  string
}

// Manager handles WatchParty operations
type Manager struct {
	config Config
}

// NewManager creates a new WatchParty manager
func NewManager(config Config) *Manager {
	return &Manager{
		config: config,
	}
}

// CreateWatchParty creates a WatchParty URL for the given media
func (m *Manager) CreateWatchParty(ctx context.Context, provider providers.Provider, mediaID string, episodeID string, quality providers.Quality, proxyConfig ProxyConfig) (string, error) {
	// Get stream URL from provider
	stream, err := provider.GetStreamURL(ctx, episodeID, quality)
	if err != nil {
		return "", fmt.Errorf("failed to get stream URL: %w", err)
	}

	// Apply proxy if needed
	videoURL := stream.URL
	if proxyConfig.ProxyURL != "" {
		videoURL, err = GenerateProxiedURL(stream.URL, ProxyConfig{
			ProxyURL: proxyConfig.ProxyURL,
			Origin:   proxyConfig.Origin,
			Referer:  stream.Referer, // Use the stream's referer as additional header if needed
		})
		if err != nil {
			return "", fmt.Errorf("failed to generate proxied URL: %w", err)
		}
	}

	// Generate WatchParty URL
	watchPartyURL := GenerateWatchPartyURL(videoURL)
	return watchPartyURL, nil
}

// GenerateProxiedURL creates a proxied URL with origin/referer headers
func GenerateProxiedURL(streamURL string, config ProxyConfig) (string, error) {
	if config.ProxyURL == "" {
		return streamURL, nil // No proxy, return original URL
	}

	// Parse and validate URLs
	proxyBase, err := url.Parse(config.ProxyURL)
	if err != nil {
		return "", fmt.Errorf("invalid proxy URL: %w", err)
	}

	_, err = url.Parse(streamURL)
	if err != nil {
		return "", fmt.Errorf("invalid stream URL: %w", err)
	}

	// Determine the origin - use config origin if provided, otherwise derive from referer
	origin := config.Origin
	if origin == "" && config.Referer != "" {
		// Extract origin from referer by parsing the referer URL and using scheme + host
		refererParsed, err := url.Parse(config.Referer)
		if err == nil {
			origin = fmt.Sprintf("%s://%s", refererParsed.Scheme, refererParsed.Host)
		}
	}

	// Construct proxy query parameters
	params := url.Values{}
	params.Add("url", streamURL) // The original m3u8 URL
	params.Add("origin", origin)

	if config.Referer != "" {
		params.Add("referer", config.Referer) // Referer header (if provided)
	}

	// Build the proxied URL
	proxyBase.RawQuery = params.Encode()
	return proxyBase.String(), nil
}

// GenerateWatchPartyURL creates the WatchParty URL with the proxied stream
func GenerateWatchPartyURL(proxiedStreamURL string) string {
	baseURL := "https://www.watchparty.me/create"
	params := url.Values{}
	params.Add("video", proxiedStreamURL)

	watchPartyURL, _ := url.Parse(baseURL) // Error is unlikely here
	watchPartyURL.RawQuery = params.Encode()
	return watchPartyURL.String()
}

// OpenURL opens the URL in the default browser
func OpenURL(url string) error {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}

	return err
}

// ValidateProxyURL checks if the proxy URL is accessible
func ValidateProxyURL(proxyURL string) error {
	if proxyURL == "" {
		return nil // Empty proxy is valid (no proxy)
	}

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}

	// Make a simple request to check if the proxy is accessible
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(parsedURL.String())
	if err != nil {
		return fmt.Errorf("proxy URL is not accessible: %w", err)
	}
	_ = resp.Body.Close()

	// For a simple validation, we just check if we can reach the proxy
	// A real implementation might validate the proxy's specific behavior
	return nil
}

// CreateDebugInfo creates debug information for stream URLs
type DebugInfo struct {
	MediaTitle    string               `json:"media_title"`
	EpisodeTitle  string               `json:"episode_title"`
	EpisodeNumber int                  `json:"episode_number"`
	StreamURL     string               `json:"stream_url"`
	ProxiedURL    string               `json:"proxied_url"`
	WatchPartyURL string               `json:"watch_party_url"`
	Quality       string               `json:"quality"`
	Type          string               `json:"type"`
	Referer       string               `json:"referer,omitempty"`
	Headers       map[string]string    `json:"headers,omitempty"`
	Subtitles     []providers.Subtitle `json:"subtitles,omitempty"`
}

// GetStreamDebugInfo gets debug information for the stream
func (m *Manager) GetStreamDebugInfo(ctx context.Context, provider providers.Provider, mediaID string, episodeID string, quality providers.Quality, proxyConfig ProxyConfig) (*DebugInfo, error) {
	// Get stream URL from provider
	stream, err := provider.GetStreamURL(ctx, episodeID, quality)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream URL: %w", err)
	}

	// Apply proxy if needed
	proxiedURL := stream.URL
	if proxyConfig.ProxyURL != "" {
		proxiedURL, err = GenerateProxiedURL(stream.URL, ProxyConfig{
			ProxyURL: proxyConfig.ProxyURL,
			Origin:   proxyConfig.Origin,
			Referer:  stream.Referer,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to generate proxied URL: %w", err)
		}
	}

	// Generate WatchParty URL
	watchPartyURL := GenerateWatchPartyURL(proxiedURL)

	// Get media details for debug info
	mediaDetails, err := provider.GetMediaDetails(ctx, mediaID)
	if err != nil {
		return nil, fmt.Errorf("failed to get media details: %w", err)
	}

	// Find episode title
	var episodeTitle string
	var episodeNumber int
	seasons, err := provider.GetSeasons(ctx, mediaID)
	if err == nil && len(seasons) > 0 {
		episodes, err := provider.GetEpisodes(ctx, seasons[0].ID)
		if err == nil {
			for _, ep := range episodes {
				if ep.ID == episodeID {
					episodeTitle = ep.Title
					episodeNumber = ep.Number
					break
				}
			}
		}
	}

	return &DebugInfo{
		MediaTitle:    mediaDetails.Title,
		EpisodeTitle:  episodeTitle,
		EpisodeNumber: episodeNumber,
		StreamURL:     stream.URL,
		ProxiedURL:    proxiedURL,
		WatchPartyURL: watchPartyURL,
		Quality:       string(stream.Quality),
		Type:          string(stream.Type),
		Referer:       stream.Referer,
		Headers:       stream.Headers,
		Subtitles:     stream.Subtitles,
	}, nil
}
