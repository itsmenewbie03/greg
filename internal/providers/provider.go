package providers

import (
	"context"
	"time"
)

// Provider defines the interface for streaming providers
type Provider interface {
	// Metadata
	Name() string
	Type() MediaType

	// Search and discovery
	Search(ctx context.Context, query string) ([]Media, error)
	GetTrending(ctx context.Context) ([]Media, error)
	GetRecent(ctx context.Context) ([]Media, error)

	// Media details
	GetMediaDetails(ctx context.Context, id string) (*MediaDetails, error)
	GetSeasons(ctx context.Context, mediaID string) ([]Season, error)
	GetEpisodes(ctx context.Context, seasonID string) ([]Episode, error)

	// Stream URLs
	GetStreamURL(ctx context.Context, episodeID string, quality Quality) (*StreamURL, error)
	GetAvailableQualities(ctx context.Context, episodeID string) ([]Quality, error)

	// Health check
	HealthCheck(ctx context.Context) error
}

// MediaType represents the type of media content
type MediaType string

const (
	MediaTypeAnime        MediaType = "anime"
	MediaTypeMovie        MediaType = "movie"
	MediaTypeTV           MediaType = "tv"
	MediaTypeMovieTV      MediaType = "movie_tv"       // Supports both movies and TV
	MediaTypeAnimeMovieTV MediaType = "anime_movie_tv" // Supports anime, movies, and TV
	MediaTypeManga        MediaType = "manga"          // Supports manga
	MediaTypeAll          MediaType = "all"            // Supports all types
)

// MangaProvider defines the interface for manga providers
type MangaProvider interface {
	Provider
	GetMangaPages(ctx context.Context, chapterID string) ([]string, error)
}

// Media represents a single media item
type Media struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Type          MediaType `json:"type"`
	Year          int       `json:"year"`
	Synopsis      string    `json:"synopsis"`
	PosterURL     string    `json:"poster_url"`
	Rating        float64   `json:"rating"`
	Genres        []string  `json:"genres"`
	TotalEpisodes int       `json:"total_episodes"`
	Status        string    `json:"status"` // "Ongoing", "Completed", etc.
}

// MediaDetails provides extended information about a media item
type MediaDetails struct {
	Media
	Seasons   []Season `json:"seasons"`
	Cast      []string `json:"cast"`
	Studio    string   `json:"studio"`   // For anime
	Director  string   `json:"director"` // For movies
	AniListID int      `json:"anilist_id,omitempty"`
	IMDBID    string   `json:"imdb_id,omitempty"`
}

// Season represents a season of a TV show
type Season struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
}

// Episode represents a single episode or movie
type Episode struct {
	ID           string        `json:"id"`
	Number       int           `json:"number"`
	Season       int           `json:"season"`
	Title        string        `json:"title"`
	Synopsis     string        `json:"synopsis"`
	ThumbnailURL string        `json:"thumbnail_url"`
	Duration     time.Duration `json:"duration"`
	ReleaseDate  time.Time     `json:"release_date"`
}

// StreamURL contains streaming information
type StreamURL struct {
	URL         string            `json:"url"`
	Quality     Quality           `json:"quality"`
	Type        StreamType        `json:"type"`
	Headers     map[string]string `json:"headers,omitempty"`
	Subtitles   []Subtitle        `json:"subtitles,omitempty"`
	AudioTracks []AudioTrack      `json:"audio_tracks,omitempty"`
	Referer     string            `json:"referer,omitempty"`
}

// Subtitle represents a subtitle track
type Subtitle struct {
	Language string `json:"language"`
	URL      string `json:"url"`
	Format   string `json:"format"` // srt, vtt, ass
}

// AudioTrack represents an audio track option
type AudioTrack struct {
	Index    int    `json:"index"`    // mpv track index (1-based)
	Language string `json:"language"` // "en", "ja", etc.
	Label    string `json:"label"`    // Provider's original label: "English (Dub)", "Japanese (Original)"
	Type     string `json:"type"`     // "dub", "sub", "original", "unknown"
}

// Quality represents video quality levels
type Quality string

const (
	Quality360p  Quality = "360p"
	Quality480p  Quality = "480p"
	Quality720p  Quality = "720p"
	Quality1080p Quality = "1080p"
	Quality1440p Quality = "1440p"
	Quality4K    Quality = "2160p"
	QualityAuto  Quality = "auto"
)

// StreamType represents the type of stream
type StreamType string

const (
	StreamTypeHLS  StreamType = "hls"  // HTTP Live Streaming (.m3u8)
	StreamTypeDASH StreamType = "dash" // MPEG-DASH (.mpd)
	StreamTypeMP4  StreamType = "mp4"  // Direct MP4
	StreamTypeMKV  StreamType = "mkv"  // Direct MKV
)

// HealthCheckResult holds detailed health check information
type HealthCheckResult struct {
	URL         string
	CurlCommand string
	StatusCode  int
	Duration    time.Duration
	Error       string
	CheckedAt   time.Time
}

// ProviderStatus holds the health status of a provider
type ProviderStatus struct {
	ProviderName string
	Healthy      bool
	Status       string // e.g., "Online", "Offline", "Error: ...", "Checking..."
	LastCheck    time.Time
	LastResult   *HealthCheckResult
}

// ParseQuality parses a quality string into a Quality type
func ParseQuality(s string) (Quality, error) {
	switch s {
	case "360p":
		return Quality360p, nil
	case "480p":
		return Quality480p, nil
	case "720p":
		return Quality720p, nil
	case "1080p":
		return Quality1080p, nil
	case "1440p":
		return Quality1440p, nil
	case "2160p", "4k", "4K":
		return Quality4K, nil
	case "auto":
		return QualityAuto, nil
	default:
		return "", &ErrInvalidQuality{Quality: s}
	}
}

// String returns the string representation of Quality
func (q Quality) String() string {
	return string(q)
}

// ResolutionHeight returns the vertical resolution in pixels
func (q Quality) ResolutionHeight() int {
	switch q {
	case Quality360p:
		return 360
	case Quality480p:
		return 480
	case Quality720p:
		return 720
	case Quality1080p:
		return 1080
	case Quality1440p:
		return 1440
	case Quality4K:
		return 2160
	default:
		return 0
	}
}

// ErrInvalidQuality is returned when an invalid quality string is provided
type ErrInvalidQuality struct {
	Quality string
}

func (e *ErrInvalidQuality) Error() string {
	return "invalid quality: " + e.Quality
}
