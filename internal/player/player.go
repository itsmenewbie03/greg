package player

import (
	"context"
	"time"
)

// Player defines the interface for video players
type Player interface {
	// Playback control
	Play(ctx context.Context, url string, options PlayOptions) error
	Stop(ctx context.Context) error

	// Progress monitoring
	GetProgress(ctx context.Context) (*PlaybackProgress, error)
	Seek(ctx context.Context, position time.Duration) error

	// Callbacks
	OnProgressUpdate(callback func(progress PlaybackProgress))
	OnPlaybackEnd(callback func())
	OnError(callback func(err error))

	// Status
	IsPlaying() bool
	IsPaused() bool
}

// PlayOptions contains options for starting playback
type PlayOptions struct {
	// Playback options
	StartTime  time.Duration `json:"start_time,omitempty"`
	Volume     int           `json:"volume,omitempty"` // 0-100
	Speed      float64       `json:"speed,omitempty"`  // Playback speed (1.0 = normal)
	Fullscreen bool          `json:"fullscreen"`

	// Subtitle options
	SubtitleURL   string        `json:"subtitle_url,omitempty"`
	SubtitleLang  string        `json:"subtitle_lang,omitempty"`
	SubtitleDelay time.Duration `json:"subtitle_delay,omitempty"`

	// Audio options
	AudioTrack int `json:"audio_track,omitempty"`

	// mpv-specific options
	MPVArgs []string `json:"mpv_args,omitempty"`

	// Headers for HTTP requests
	Headers   map[string]string `json:"headers,omitempty"`
	Referer   string            `json:"referer,omitempty"`
	UserAgent string            `json:"user_agent,omitempty"`

	// Metadata for display/tracking
	Title   string `json:"title,omitempty"`
	Episode int    `json:"episode,omitempty"`
	Season  int    `json:"season,omitempty"`
}

// PlaybackProgress represents the current playback state
type PlaybackProgress struct {
	CurrentTime time.Duration `json:"current_time"`
	Duration    time.Duration `json:"duration"`
	Percentage  float64       `json:"percentage"` // 0.0 - 100.0
	Paused      bool          `json:"paused"`
	Volume      int           `json:"volume"`
	Speed       float64       `json:"speed"`
	EOF         bool          `json:"eof"` // End of file reached
}

// PlaybackState represents the state of the player
type PlaybackState string

const (
	StatePlaying PlaybackState = "playing"
	StatePaused  PlaybackState = "paused"
	StateStopped PlaybackState = "stopped"
	StateLoading PlaybackState = "loading"
	StateError   PlaybackState = "error"
)

// String returns the string representation of PlaybackState
func (s PlaybackState) String() string {
	return string(s)
}

// PlayerCallbacks contains callback functions for player events
type PlayerCallbacks struct {
	OnProgress func(PlaybackProgress)
	OnComplete func()
	OnError    func(error)
	OnPause    func()
	OnResume   func()
	OnSeek     func(position time.Duration)
}

// PlayerInfo contains information about the player binary
type PlayerInfo struct {
	Name    string `json:"name"` // mpv, vlc, iina
	Version string `json:"version"`
	Path    string `json:"path"` // Full path to binary
}
