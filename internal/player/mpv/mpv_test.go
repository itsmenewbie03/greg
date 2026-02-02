package mpv

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/justchokingaround/greg/internal/player"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMPVPlayer(t *testing.T) {
	p, err := NewMPVPlayer()
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, player.StateStopped, p.state)
	assert.NotNil(t, p.done)
}

func TestGenerateIPCConfig(t *testing.T) {
	// Test Unix socket generation (Linux/Mac)
	config1, err := GetIPCConfig(PlatformLinux)
	require.NoError(t, err)
	assert.NotNil(t, config1)
	assert.Equal(t, IPCUnixSocket, config1.Type)
	assert.True(t, config1.IsSocket)
	assert.Contains(t, config1.Address, "greg-mpv-")
	assert.Contains(t, config1.Address, ".sock")

	// Generate another and ensure it's unique
	config2, err := GetIPCConfig(PlatformLinux)
	require.NoError(t, err)
	assert.NotEqual(t, config1.Address, config2.Address)

	// Ensure socket path is in temp directory
	assert.Contains(t, config1.Address, os.TempDir())

	// Test Unix socket generation for WSL (as documented, WSL uses Linux mpv for better compatibility)
	wslConfig, err := GetIPCConfig(PlatformWSL)
	require.NoError(t, err)
	assert.NotNil(t, wslConfig)
	assert.Equal(t, IPCUnixSocket, wslConfig.Type)
	assert.True(t, wslConfig.IsSocket)
	assert.Contains(t, wslConfig.Address, "greg-mpv-")
	assert.Contains(t, wslConfig.Address, ".sock")
	assert.Contains(t, wslConfig.Address, os.TempDir())

	// Test named pipe generation (Windows)
	winConfig, err := GetIPCConfig(PlatformWindows)
	require.NoError(t, err)
	assert.NotNil(t, winConfig)
	assert.Equal(t, IPCNamedPipe, winConfig.Type)
	assert.False(t, winConfig.IsSocket)
	assert.Contains(t, winConfig.Address, `\\.\pipe\greg-mpv-`)
}

func TestBuildMPVArgs(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		options  player.PlayOptions
		expected []string
	}{
		{
			name:    "basic playback",
			url:     "https://example.com/video.mp4",
			options: player.PlayOptions{},
			expected: []string{
				"--idle=yes",
				"https://example.com/video.mp4",
			},
		},
		{
			name: "with start time",
			url:  "https://example.com/video.mp4",
			options: player.PlayOptions{
				StartTime: 30 * time.Second,
			},
			expected: []string{
				"--idle=yes",
				"--start=30",
				"https://example.com/video.mp4",
			},
		},
		{
			name: "with volume and speed",
			url:  "https://example.com/video.mp4",
			options: player.PlayOptions{
				Volume: 75,
				Speed:  1.5,
			},
			expected: []string{
				"--idle=yes",
				"--volume=75",
				"--speed=1.5",
				"https://example.com/video.mp4",
			},
		},
		{
			name: "fullscreen",
			url:  "https://example.com/video.mp4",
			options: player.PlayOptions{
				Fullscreen: true,
			},
			expected: []string{
				"--idle=yes",
				"--fullscreen",
				"https://example.com/video.mp4",
			},
		},
		{
			name: "with subtitles",
			url:  "https://example.com/video.mp4",
			options: player.PlayOptions{
				SubtitleURL:  "https://example.com/subs.srt",
				SubtitleLang: "eng",
			},
			expected: []string{
				"--idle=yes",
				"--sub-file=https://example.com/subs.srt",
				"--slang=eng",
				"https://example.com/video.mp4",
			},
		},
		{
			name: "with referer and user agent",
			url:  "https://example.com/video.mp4",
			options: player.PlayOptions{
				Referer:   "https://example.com",
				UserAgent: "Mozilla/5.0",
			},
			expected: []string{
				"--idle=yes",
				"--referrer=https://example.com",
				"--user-agent=Mozilla/5.0",
				"https://example.com/video.mp4",
			},
		},
		{
			name: "with custom headers",
			url:  "https://example.com/video.mp4",
			options: player.PlayOptions{
				Headers: map[string]string{
					"X-Custom": "value",
				},
			},
			expected: []string{
				"--idle=yes",
				"--http-header-fields=X-Custom: value",
				"https://example.com/video.mp4",
			},
		},
		{
			name: "with title",
			url:  "https://example.com/video.mp4",
			options: player.PlayOptions{
				Title: "Test Video",
			},
			expected: []string{
				"--idle=yes",
				"--force-media-title=Test Video",
				"https://example.com/video.mp4",
			},
		},
		{
			name: "with custom mpv args",
			url:  "https://example.com/video.mp4",
			options: player.PlayOptions{
				MPVArgs: []string{"--hwdec=auto", "--cache=yes"},
			},
			expected: []string{
				"--idle=yes",
				"--hwdec=auto",
				"--cache=yes",
				"https://example.com/video.mp4",
			},
		},
		{
			name: "all options",
			url:  "https://example.com/video.mp4",
			options: player.PlayOptions{
				StartTime:     15 * time.Second,
				Volume:        80,
				Speed:         1.25,
				Fullscreen:    true,
				SubtitleURL:   "https://example.com/subs.srt",
				SubtitleLang:  "eng",
				SubtitleDelay: 2 * time.Second,
				AudioTrack:    2,
				Referer:       "https://example.com",
				UserAgent:     "Mozilla/5.0",
				Title:         "Full Test",
				MPVArgs:       []string{"--cache=yes"},
			},
			expected: []string{
				"--idle=yes",
				"--start=15",
				"--volume=80",
				"--speed=1.25",
				"--fullscreen",
				"--sub-file=https://example.com/subs.srt",
				"--slang=eng",
				"--sub-delay=2",
				"--aid=2",
				"--referrer=https://example.com",
				"--user-agent=Mozilla/5.0",
				"--force-media-title=Full Test",
				"--cache=yes",
				"https://example.com/video.mp4",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test IPC config
			ipcConfig := &IPCConfig{
				Type:     IPCUnixSocket,
				Address:  "/tmp/test.sock",
				IsSocket: true,
			}
			p := &MPVPlayer{
				ipcConfig: ipcConfig,
			}

			args := p.buildMPVArgs(tt.url, tt.options)

			// Check that socket path argument is present
			assert.Contains(t, args[0], "--input-ipc-server=")

			// Check expected arguments (skip socket path)
			for _, expected := range tt.expected {
				found := false
				for _, arg := range args {
					if strings.Contains(arg, expected) || arg == expected {
						found = true
						break
					}
				}
				assert.True(t, found, "expected arg %q not found in %v", expected, args)
			}

			// URL should be the last argument
			assert.Equal(t, tt.url, args[len(args)-1])
		})
	}
}

func TestPlayerState(t *testing.T) {
	p, err := NewMPVPlayer()
	require.NoError(t, err)

	// Initial state
	assert.False(t, p.IsPlaying())
	assert.False(t, p.IsPaused())
	assert.Equal(t, player.StateStopped, p.state)

	// Simulate playing state
	p.state = player.StatePlaying
	assert.True(t, p.IsPlaying())
	assert.False(t, p.IsPaused())

	// Simulate paused state
	p.state = player.StatePaused
	assert.False(t, p.IsPlaying())
	assert.True(t, p.IsPaused())

	// Back to stopped
	p.state = player.StateStopped
	assert.False(t, p.IsPlaying())
	assert.False(t, p.IsPaused())
}

func TestCallbacks(t *testing.T) {
	p, err := NewMPVPlayer()
	require.NoError(t, err)

	// Test progress callback
	var progressCalled bool
	p.OnProgressUpdate(func(progress player.PlaybackProgress) {
		progressCalled = true
	})
	assert.NotNil(t, p.onProgress)

	// Test end callback
	var endCalled bool
	p.OnPlaybackEnd(func() {
		endCalled = true
	})
	assert.NotNil(t, p.onEnd)

	// Test error callback
	var errorCalled bool
	p.OnError(func(err error) {
		errorCalled = true
	})
	assert.NotNil(t, p.onError)

	// Trigger callbacks manually to verify they work
	if p.onProgress != nil {
		p.onProgress(player.PlaybackProgress{})
		assert.True(t, progressCalled)
	}

	if p.onEnd != nil {
		p.onEnd()
		assert.True(t, endCalled)
	}

	if p.onError != nil {
		p.onError(assert.AnError)
		assert.True(t, errorCalled)
	}
}

func TestStopWhenAlreadyStopped(t *testing.T) {
	p, err := NewMPVPlayer()
	require.NoError(t, err)

	// Stop when already stopped should not error
	err = p.stopLocked()
	assert.NoError(t, err)
	assert.Equal(t, player.StateStopped, p.state)
}
