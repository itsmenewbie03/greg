package mpv

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/diniamo/gopv"
	"github.com/justchokingaround/greg/internal/config"
	"github.com/justchokingaround/greg/internal/player"
	"github.com/spf13/viper"
)

// MPVPlayer implements the Player interface using mpv with IPC
type MPVPlayer struct {
	mu sync.RWMutex

	// mpv process and IPC
	client    *gopv.Client
	cmd       *exec.Cmd
	ipcConfig *IPCConfig
	platform  Platform

	// State
	state      player.PlaybackState
	currentURL string
	options    player.PlayOptions

	// Callbacks
	onProgress func(player.PlaybackProgress)
	onEnd      func()
	onError    func(error)

	// Control
	ctx          context.Context
	cancel       context.CancelFunc
	done         chan struct{}
	clientClosed bool

	// Configuration
	debug          bool
	loadUserConfig bool
}

// NewMPVPlayer creates a new mpv player instance
func NewMPVPlayer() (*MPVPlayer, error) {
	return NewMPVPlayerWithDebug(false)
}

// NewMPVPlayerWithConfig creates a new mpv player instance with configuration
func NewMPVPlayerWithConfig(cfg *config.Config, debug bool) (*MPVPlayer, error) {
	// Detect platform
	platform := DetectPlatform()

	// Verify mpv executable is available
	if _, err := FindMPVExecutable(platform); err != nil {
		return nil, fmt.Errorf("mpv not found: %w", err)
	}

	player := &MPVPlayer{
		state:          player.StateStopped,
		done:           make(chan struct{}),
		platform:       platform,
		debug:          debug,
		loadUserConfig: cfg.Player.LoadUserConfig,
	}

	return player, nil
}

// NewMPVPlayerWithDebug creates a new mpv player instance with debug flag
func NewMPVPlayerWithDebug(debug bool) (*MPVPlayer, error) {
	// Create a default config to use defaults
	defaultConfig := &config.Config{}
	// Set the defaults using viper
	v := viper.New()
	config.SetDefaults(v) // Use the public function instead
	// Unmarshal the defaults into the config struct
	if err := v.Unmarshal(defaultConfig); err != nil {
		// Fallback to false if there's an error
		defaultConfig.Player.LoadUserConfig = false
	}

	// Detect platform
	platform := DetectPlatform()

	// Verify mpv executable is available
	if _, err := FindMPVExecutable(platform); err != nil {
		return nil, fmt.Errorf("mpv not found: %w", err)
	}

	return &MPVPlayer{
		state:          player.StateStopped,
		done:           make(chan struct{}),
		platform:       platform,
		debug:          debug,
		loadUserConfig: defaultConfig.Player.LoadUserConfig,
	}, nil
}

// Play starts playback of the given URL with options
// Returns immediately after validating prerequisites and starting the launch process
// Launch failures will be reported via the OnError callback
func (p *MPVPlayer) Play(ctx context.Context, url string, options player.PlayOptions) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop any existing playback
	if p.state != player.StateStopped {
		if err := p.stopLocked(); err != nil {
			return fmt.Errorf("failed to stop existing playback: %w", err)
		}
	}

	// Get mpv executable for platform and validate it exists
	mpvExec := GetMPVExecutable(p.platform)
	if _, err := exec.LookPath(mpvExec); err != nil {
		return fmt.Errorf("mpv executable not found in PATH (%s): %w\nPlease install mpv and ensure it's in your system PATH", mpvExec, err)
	}

	// Generate IPC configuration for platform
	ipcConfig, err := GetIPCConfig(p.platform)
	if err != nil {
		return fmt.Errorf("failed to generate IPC config: %w", err)
	}
	p.ipcConfig = ipcConfig

	// Build mpv arguments
	args := p.buildMPVArgs(url, options)

	// Start mpv process
	p.cmd = exec.Command(mpvExec, args...)

	// Detach mpv completely from terminal to prevent it from interfering with TUI
	// - Stdin: prevents mpv from stealing keyboard input
	// - Stdout/Stderr: prevents mpv output from corrupting TUI display
	// On Windows, this is especially important as console handles are shared differently
	p.cmd.Stdin = nil
	p.cmd.Stdout = nil
	p.cmd.Stderr = nil

	// Setup platform-specific process attributes (e.g., CREATE_NEW_PROCESS_GROUP on Windows)
	setupProcessAttributes(p.cmd)

	if err := p.cmd.Start(); err != nil {
		p.cleanupIPC()
		return fmt.Errorf("failed to start %s: %w", mpvExec, err)
	}

	// Verify process started successfully
	// On Windows, the process may fail immediately if it can't create the named pipe
	time.Sleep(100 * time.Millisecond)
	if p.cmd.Process == nil {
		p.cleanupIPC()
		return fmt.Errorf("mpv process failed to start")
	}

	// Update state to loading
	p.currentURL = url
	p.options = options
	p.state = player.StateLoading

	// Start async initialization - this will handle IPC connection and state updates
	p.ctx, p.cancel = context.WithCancel(context.Background())
	go p.asyncInitialize(ctx, ipcConfig)

	return nil
}

// asyncInitialize handles the async parts of player initialization
// Reports errors via OnError callback and updates state when ready
func (p *MPVPlayer) asyncInitialize(ctx context.Context, ipcConfig *IPCConfig) {
	// Create a timeout context for the initialization
	initCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Wait for IPC to be ready
	if err := p.waitForIPC(initCtx); err != nil {
		p.mu.Lock()
		errorCallback := p.onError
		p.mu.Unlock()

		// Kill the process and cleanup
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		p.cleanupIPC()

		if errorCallback != nil {
			if p.platform == PlatformWindows {
				errorCallback(fmt.Errorf("failed to connect to mpv (timeout waiting for named pipe: %s): %w\n"+
					"This may indicate mpv.exe failed to start or lacks permissions", ipcConfig.Address, err))
			} else {
				errorCallback(fmt.Errorf("timeout waiting for mpv IPC at %s: %w", ipcConfig.Address, err))
			}
		}

		p.mu.Lock()
		p.state = player.StateError
		p.mu.Unlock()
		return
	}

	// Connect to mpv IPC
	connStr := GetGopvConnectionString(ipcConfig)
	client, err := gopv.Connect(connStr, func(err error) {
		p.mu.Lock()
		errorCallback := p.onError
		p.mu.Unlock()
		if errorCallback != nil {
			errorCallback(err)
		}
	})
	if err != nil {
		p.mu.Lock()
		errorCallback := p.onError
		p.mu.Unlock()

		// Kill the process and cleanup
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		p.cleanupIPC()

		if errorCallback != nil {
			// Provide platform-specific error messages
			if p.platform == PlatformWindows {
				errorCallback(fmt.Errorf("failed to connect to mpv IPC (Windows named pipe: %s): %w\n"+
					"Make sure mpv.exe is properly installed and in PATH", connStr, err))
			} else {
				errorCallback(fmt.Errorf("failed to connect to mpv IPC at %s: %w", connStr, err))
			}
		}

		p.mu.Lock()
		p.state = player.StateError
		p.mu.Unlock()
		return
	}

	p.mu.Lock()
	p.client = client
	p.clientClosed = false // Reset for new connection
	p.state = player.StatePlaying
	p.mu.Unlock()

	// Start monitoring goroutines
	go p.monitorProgress()
	go p.monitorProcess()
}

// Stop stops playback and cleans up resources
func (p *MPVPlayer) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopLocked()
}

// stopLocked stops playback without locking (must be called with lock held)
func (p *MPVPlayer) stopLocked() error {
	if p.state == player.StateStopped {
		return nil
	}

	// Prevent double-close of gopv.Client - check and set atomically
	if p.clientClosed {
		return nil
	}
	p.clientClosed = true // Set immediately to prevent any race

	// Mark as stopped first to prevent double cleanup
	p.state = player.StateStopped

	// Cancel context to stop goroutines
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}

	// Send quit command to mpv and let the process exit naturally
	// When mpv exits, the IPC connection gets EOF, and gopv's read() goroutine
	// will call Close() automatically. We should NOT call Close() ourselves to
	// avoid double-close panics.
	if p.client != nil {
		client := p.client
		p.client = nil // Clear reference immediately
		go func() {
			// Try to send quit, but don't wait long
			// If this fails, the process kill below will clean up anyway
			done := make(chan struct{})
			go func() {
				_, _ = client.Request("quit")
				close(done)
			}()
			// Wait max 500ms for quit command
			select {
			case <-done:
			case <-time.After(500 * time.Millisecond):
			}
			// Don't call client.Close() - let gopv's read() goroutine handle it
			// when it gets EOF from the dead process
		}()
	}

	// Force kill process immediately (monitorProcess will handle Wait())
	if p.cmd != nil && p.cmd.Process != nil {
		// Kill the process - monitorProcess goroutine will receive the exit signal
		_ = p.cmd.Process.Kill()
		// Don't call Wait() here - monitorProcess is already waiting
	}
	p.cmd = nil

	// Cleanup IPC resources
	p.cleanupIPC()

	p.currentURL = ""

	return nil
}

// cleanupIPC cleans up IPC resources (sockets, files, etc.)
func (p *MPVPlayer) cleanupIPC() {
	if p.ipcConfig != nil && p.ipcConfig.IsSocket {
		// Remove Unix socket file
		_ = os.Remove(p.ipcConfig.Address)
	}
	p.ipcConfig = nil
}

// GetProgress returns the current playback progress
func (p *MPVPlayer) GetProgress(ctx context.Context) (*player.PlaybackProgress, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.client == nil {
		return nil, fmt.Errorf("player not initialized")
	}

	// Check if player state indicates it's stopped (Windows IPC may be dead)
	if p.state == player.StateStopped {
		return nil, fmt.Errorf("player is stopped")
	}

	progress, err := p.getProgressLocked()
	if err != nil {
		return nil, fmt.Errorf("mpv IPC error: %w", err)
	}

	return progress, nil
}

// getProgressLocked gets progress without locking (must be called with lock held)
func (p *MPVPlayer) getProgressLocked() (*player.PlaybackProgress, error) {
	var timePos, duration, volume, speed float64
	var paused, eof bool
	var propertyErrors int

	// Get properties from mpv
	// Track errors to detect IPC failures on Windows
	if result, err := p.client.Request("get_property", "time-pos"); err == nil {
		if val, ok := result.(float64); ok {
			timePos = val
		}
	} else {
		propertyErrors++
		// On Windows, IPC errors often manifest as request failures
		if runtime.GOOS == "windows" {
			return nil, fmt.Errorf("Windows IPC error getting time-pos: %w", err)
		}
	}

	if result, err := p.client.Request("get_property", "duration"); err == nil {
		if val, ok := result.(float64); ok {
			duration = val
		}
	} else {
		propertyErrors++
		if runtime.GOOS == "windows" && propertyErrors > 1 {
			// Multiple property failures indicate broken IPC
			return nil, fmt.Errorf("Windows IPC error getting duration (multiple failures): %w", err)
		}
	}

	if result, err := p.client.Request("get_property", "pause"); err == nil {
		if val, ok := result.(bool); ok {
			paused = val
		}
	} else {
		propertyErrors++
	}

	if result, err := p.client.Request("get_property", "eof-reached"); err == nil {
		if val, ok := result.(bool); ok {
			eof = val
		}
	} else {
		propertyErrors++
	}

	if result, err := p.client.Request("get_property", "volume"); err == nil {
		if val, ok := result.(float64); ok {
			volume = val
		} else {
			volume = 100
		}
	}

	if result, err := p.client.Request("get_property", "speed"); err == nil {
		if val, ok := result.(float64); ok {
			speed = val
		} else {
			speed = 1.0
		}
	}

	// If we got too many property errors, the IPC connection is likely dead
	if propertyErrors >= 3 {
		if runtime.GOOS == "windows" {
			return nil, fmt.Errorf("Windows IPC appears dead (failed to get %d properties)", propertyErrors)
		}
		return nil, fmt.Errorf("IPC connection failed (failed to get %d properties)", propertyErrors)
	}

	// Calculate percentage
	var percentage float64
	if duration > 0 {
		percentage = (timePos / duration) * 100
	}

	return &player.PlaybackProgress{
		CurrentTime: time.Duration(timePos * float64(time.Second)),
		Duration:    time.Duration(duration * float64(time.Second)),
		Percentage:  percentage,
		Paused:      paused,
		Volume:      int(volume),
		Speed:       speed,
		EOF:         eof,
	}, nil
}

// Seek seeks to the specified position
func (p *MPVPlayer) Seek(ctx context.Context, position time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client == nil {
		return fmt.Errorf("player not initialized")
	}

	seconds := position.Seconds()
	if _, err := p.client.Request("set_property", "time-pos", seconds); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	return nil
}

// OnProgressUpdate sets the progress update callback
func (p *MPVPlayer) OnProgressUpdate(callback func(progress player.PlaybackProgress)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onProgress = callback
}

// OnPlaybackEnd sets the playback end callback
func (p *MPVPlayer) OnPlaybackEnd(callback func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onEnd = callback
}

// OnError sets the error callback
func (p *MPVPlayer) OnError(callback func(err error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onError = callback
}

// IsPlaying returns true if the player is currently playing
func (p *MPVPlayer) IsPlaying() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state == player.StatePlaying
}

// IsPaused returns true if the player is currently paused
func (p *MPVPlayer) IsPaused() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state == player.StatePaused
}

// monitorProgress monitors playback progress and triggers callbacks
func (p *MPVPlayer) monitorProgress() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.mu.RLock()
			if p.client == nil {
				p.mu.RUnlock()
				return
			}

			progress, err := p.getProgressLocked()
			callback := p.onProgress
			endCallback := p.onEnd
			p.mu.RUnlock()

			if err != nil {
				continue
			}

			// Trigger progress callback
			if callback != nil {
				callback(*progress)
			}

			// Check for end of file
			if progress.EOF && endCallback != nil {
				endCallback()
				return
			}
		}
	}
}

// monitorProcess monitors the mpv process and handles unexpected exits
func (p *MPVPlayer) monitorProcess() {
	// Take a local reference to cmd so we can wait even if p.cmd becomes nil
	p.mu.RLock()
	cmd := p.cmd
	p.mu.RUnlock()

	if cmd == nil {
		return
	}

	// This will block until the process exits (killed or natural exit)
	err := cmd.Wait()

	// Only report error if it wasn't a normal kill signal
	p.mu.Lock()
	errorCallback := p.onError
	currentState := p.state
	p.mu.Unlock()

	// Don't report error if we're already stopped (user requested quit)
	if err != nil && errorCallback != nil && currentState != player.StateStopped {
		errorCallback(fmt.Errorf("mpv process exited unexpectedly: %w", err))
	}

	// Clean up after process exits (safe to call multiple times)
	_ = p.Stop(context.Background())
}

// buildMPVArgs builds the command-line arguments for mpv
func (p *MPVPlayer) buildMPVArgs(url string, opts player.PlayOptions) []string {
	args := []string{
		GetMPVIPCArgument(p.ipcConfig),
		"--idle=yes", // Keep mpv running even after playback ends
		"--no-ytdl",  // Disable youtube-dl/yt-dlp hook for direct streams
	}

	// Only add --no-config if loadUserConfig is false
	if !p.loadUserConfig {
		args = append(args, "--no-config") // Don't load user config that might interfere
	}

	// Add quiet flags when not in debug mode
	if !p.debug {
		// Hide verbose track info but keep important messages
		args = append(args, "--msg-level=all=warn")
	}

	// Start time
	if opts.StartTime > 0 {
		args = append(args, fmt.Sprintf("--start=%f", opts.StartTime.Seconds()))
	}

	// Volume
	if opts.Volume > 0 {
		args = append(args, fmt.Sprintf("--volume=%d", opts.Volume))
	}

	// Speed
	if opts.Speed > 0 {
		args = append(args, fmt.Sprintf("--speed=%f", opts.Speed))
	}

	// Fullscreen
	if opts.Fullscreen {
		args = append(args, "--fullscreen")
	}

	// Subtitles
	if opts.SubtitleURL != "" {
		args = append(args, fmt.Sprintf("--sub-file=%s", opts.SubtitleURL))
	}

	if opts.SubtitleLang != "" {
		args = append(args, fmt.Sprintf("--slang=%s", opts.SubtitleLang))
	}

	if opts.SubtitleDelay > 0 {
		args = append(args, fmt.Sprintf("--sub-delay=%f", opts.SubtitleDelay.Seconds()))
	}

	// Audio track
	if opts.AudioTrack > 0 {
		args = append(args, fmt.Sprintf("--aid=%d", opts.AudioTrack))
	}

	// User-Agent
	if opts.UserAgent != "" {
		args = append(args, fmt.Sprintf("--user-agent=%s", opts.UserAgent))
	} else {
		// Use a default user agent to avoid 403s
		args = append(args, "--user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	// Referer - use dedicated --referrer option (more reliable than http-header-fields)
	if opts.Referer != "" {
		args = append(args, fmt.Sprintf("--referrer=%s", opts.Referer))
	}

	// Additional HTTP headers (Origin, etc)
	headersList := []string{}
	for key, value := range opts.Headers {
		if key != "User-Agent" && key != "Referer" { // Already handled above
			headersList = append(headersList, fmt.Sprintf("%s: %s", key, value))
		}
	}

	// Add other headers as comma-separated list in a single argument
	if len(headersList) > 0 {
		headersStr := strings.Join(headersList, ",")
		args = append(args, fmt.Sprintf("--http-header-fields=%s", headersStr))
	}

	// Title - use force-media-title to ensure it's displayed in mpv
	if opts.Title != "" {
		args = append(args, fmt.Sprintf("--force-media-title=%s", opts.Title))
	}

	// Custom mpv args
	args = append(args, opts.MPVArgs...)

	// URL must be last
	args = append(args, url)

	return args
}

// waitForIPC waits for the IPC connection to be ready
func (p *MPVPlayer) waitForIPC(ctx context.Context) error {
	// Use longer timeout for named pipes and TCP (mpv.exe takes longer to start from WSL)
	timeoutDuration := 5 * time.Second
	if p.ipcConfig.Type == IPCTCP || p.ipcConfig.Type == IPCNamedPipe {
		timeoutDuration = 10 * time.Second
	}

	timeout := time.After(timeoutDuration)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Give mpv a moment to start before checking
	time.Sleep(300 * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for IPC at %s after %v", p.ipcConfig.Address, timeoutDuration)
		case <-ticker.C:
			if p.ipcConfig.IsSocket {
				// For Unix sockets, check if file exists
				if _, err := os.Stat(p.ipcConfig.Address); err == nil {
					// Socket exists, wait a bit more for it to be ready
					time.Sleep(200 * time.Millisecond)
					return nil
				}
			} else if p.ipcConfig.Type == IPCTCP {
				// For TCP, try to connect
				conn, err := net.DialTimeout("tcp", p.ipcConfig.Address, 200*time.Millisecond)
				if err == nil {
					_ = conn.Close()
					// Wait a bit longer to ensure mpv IPC is fully ready
					time.Sleep(300 * time.Millisecond)
					return nil
				}
			} else if p.ipcConfig.Type == IPCNamedPipe {
				// For Windows named pipes, try to check if pipe is accessible
				if isPipeReady(p.ipcConfig.Address) {
					// Pipe is ready, wait a bit more for mpv to be fully initialized
					time.Sleep(200 * time.Millisecond)
					return nil
				}
			}
		}
	}
}
