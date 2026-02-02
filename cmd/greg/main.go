package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/justchokingaround/greg/internal/clipboard"
	"github.com/justchokingaround/greg/internal/config"
	"github.com/justchokingaround/greg/internal/database"
	"github.com/justchokingaround/greg/internal/downloader"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/registry"
	"github.com/justchokingaround/greg/internal/tracker"
	"github.com/justchokingaround/greg/internal/tracker/anilist"
	"github.com/justchokingaround/greg/internal/tui"
	"github.com/justchokingaround/greg/internal/watchparty"
)

var (
	// Version information (set via ldflags during build)
	version = "dev"
	commit  = "none"
	date    = "unknown"
	// Global flags
	cfgFile    string
	logLevel   string
	noColor    bool
	debugLinks bool
	debugMode  bool
	dubFlag    bool
	subFlag    bool

	// Global config and logger
	cfg    *config.Config
	logger *slog.Logger
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "greg",
	Short: "A unified TUI for streaming anime, movies, TV shows as well as reading manga",
	Long: `greg is a TUI application that combines anime, movie, and TV show streaming
with progress tracking, downloads, and social features.

Built with Go, it provides a fast, cross-platform alternative to web-based
streaming with powerful features like AniList integration, WatchParty support,
and concurrent downloads.`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config loading for config init command
		if cmd.Name() == "init" && cmd.Parent().Name() == "config" {
			return nil
		}

		// Initialize directories before config load
		if err := config.InitializeDirs(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize directories: %v\n", err)
			os.Exit(1)
		}

		// Load configuration for all other commands
		var err error
		var v *viper.Viper
		cfg, v, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Enable debug mode if flag is set
		if debugMode {
			cfg.Advanced.Debug = true
			if logLevel == "" {
				cfg.Logging.Level = "debug"
			}
		}

		// Override log level if specified
		if logLevel != "" {
			cfg.Logging.Level = logLevel
		}

		// Override color setting if specified
		if noColor {
			cfg.Logging.Color = false
		}

		// Initialize logger
		logger, err = config.InitLogger(&cfg.Logging)
		if err != nil {
			return fmt.Errorf("failed to initialize logger: %w", err)
		}

		// Initialize database
		if err := database.Init(&cfg.Database); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}

		// Initialize new registry and load providers
		reg := registry.New()
		reg.Load(cfg)

		// Register providers directly
		providers.Clear()
		for _, name := range reg.List() {
			p, err := reg.Get(name)
			if err != nil {
				continue
			}

			if err := providers.Register(p); err != nil {
				logger.Warn("failed to register provider", "name", name, "error", err)
			} else {
				logger.Debug("registered provider", "name", name)
			}
		}

		// Setup hot reload
		v.WatchConfig()
		v.OnConfigChange(func(e fsnotify.Event) {
			logger.Info("Config file changed", "name", e.Name)
			// Reload config
			if err := v.Unmarshal(&cfg); err != nil {
				logger.Error("Failed to reload config", "error", err)
				return
			}
			// Reload registry
			reg.Load(cfg)
			// Re-register providers
			providers.Clear()
			for _, name := range reg.List() {
				p, err := reg.Get(name)
				if err != nil {
					continue
				}
				if err := providers.Register(p); err != nil {
					logger.Warn("failed to register provider", "name", name, "error", err)
				}
			}
			logger.Info("Providers reloaded")
		})

		// Run provider health checks in the background
		go func() {
			logger.Info("Running provider health checks...")
			providers.CheckAllProviders(context.Background())
			logger.Info("Provider health checks complete.")
		}()

		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// Cleanup
		if err := database.Close(); err != nil {
			logger.Error("failed to close database", "error", err)
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default behavior: launch TUI
		logger.Info("greg starting...", "version", version)

		providerMap := make(map[providers.MediaType]providers.Provider)

		// Get anime provider - use configured default
		animeProvider, err := providers.Get(cfg.Providers.Default.Anime)
		if err != nil {
			// Fallback to first available anime provider
			animeProviders := providers.GetByType(providers.MediaTypeAnime)
			if len(animeProviders) > 0 {
				animeProvider = animeProviders[0]
				logger.Warn("default anime provider not available, using fallback", "default", cfg.Providers.Default.Anime, "fallback", animeProvider.Name())
			} else {
				logger.Warn("no anime providers available")
			}
		}
		if animeProvider != nil {
			providerMap[providers.MediaTypeAnime] = animeProvider
			logger.Info("using anime provider", "provider", animeProvider.Name())
		}

		// Get movie provider - use configured default
		movieProvider, err := providers.Get(cfg.Providers.Default.MoviesAndTV)
		if err != nil {
			// Fallback to first available movie/tv provider
			movieProviders := providers.GetByType(providers.MediaTypeMovieTV)
			if len(movieProviders) > 0 {
				movieProvider = movieProviders[0]
				logger.Warn("default movie provider not available, using fallback", "default", cfg.Providers.Default.MoviesAndTV, "fallback", movieProvider.Name())
			} else {
				logger.Warn("no movie/tv providers available")
			}
		}
		if movieProvider != nil {
			providerMap[providers.MediaTypeMovieTV] = movieProvider
			providerMap[providers.MediaTypeMovie] = movieProvider
			providerMap[providers.MediaTypeTV] = movieProvider
			logger.Info("using movie provider", "provider", movieProvider.Name())
		}

		// Get manga provider
		mangaProviders := providers.GetByType(providers.MediaTypeManga)
		if len(mangaProviders) > 0 {
			providerMap[providers.MediaTypeManga] = mangaProviders[0]
			logger.Info("using manga provider", "provider", mangaProviders[0].Name())
		}

		if len(providerMap) == 0 {
			return fmt.Errorf("no providers available for TUI mode")
		}

		// Initialize tracker manager
		trackerMgr := tracker.NewManager(cfg, database.DB)

		// Initialize AniList if enabled
		if cfg.Tracker.AniList.Enabled {
			tokenStorage := anilist.NewTokenStorage(database.DB)
			anilistClient := anilist.NewClient(anilist.Config{
				ClientID:    anilist.AuthBrowserClientID,
				RedirectURI: anilist.AuthBrowserRedirectURI,
				SaveToken:   tokenStorage.SaveToken,
				LoadToken:   tokenStorage.LoadToken,
			})
			trackerMgr.SetAniListClient(anilistClient)

			if anilistClient.IsAuthenticated() {
				logger.Info("AniList authenticated")
			} else {
				logger.Info("AniList not authenticated (run 'greg auth anilist' to authenticate)")
			}
		}

		// Determine audio preference from CLI flags
		audioPreference := cfg.Player.AudioPreference // Config default
		if dubFlag {
			audioPreference = "dub"
		} else if subFlag {
			audioPreference = "sub"
		}

		var debugInfo *tui.DebugInfo
		if debugLinks {
			debugInfo = tui.StartDebugLinks(providerMap, trackerMgr, database.DB, cfg, logger, audioPreference)
		} else {
			debugInfo = tui.Start(providerMap, trackerMgr, database.DB, cfg, logger, audioPreference)
		}

		// Print the debug info after TUI exits (if in debug mode)
		if debugInfo != nil {
			if debugInfo.Error != nil {
				fmt.Printf("\nError getting stream URL: %v\n", debugInfo.Error)
			} else {
				// Print JSON if --debug flag is used, otherwise human-readable
				if debugMode {
					tui.PrintDebugJSON(debugInfo)
				} else {
					// Print the stream info to clean terminal after TUI exit
					fmt.Printf("\nMedia: %s\n", debugInfo.MediaTitle)
					fmt.Printf("Episode: %s (Number: %d)\n", debugInfo.EpisodeTitle, debugInfo.EpisodeNumber)
					fmt.Printf("Stream URL: %s\n", debugInfo.StreamURL)
					fmt.Printf("Quality: %s\n", debugInfo.Quality)
					fmt.Printf("Type: %s\n", debugInfo.Type)
					if debugInfo.Referer != "" {
						fmt.Printf("Referer: %s\n", debugInfo.Referer)
					}

					fmt.Println("\nHeaders:")
					if len(debugInfo.Headers) == 0 {
						fmt.Println("  (none)")
					} else {
						for key, value := range debugInfo.Headers {
							fmt.Printf("  %s: %s\n", key, value)
						}
					}

					fmt.Println("\nSubtitles:")
					if len(debugInfo.Subtitles) == 0 {
						fmt.Println("  (none available)")
					} else {
						for i, subtitle := range debugInfo.Subtitles {
							fmt.Printf("  %d. Language: %s, Format: %s\n", i+1, subtitle.Language, subtitle.Format)
							fmt.Printf("     URL: %s\n", subtitle.URL)
						}
					}
					fmt.Println() // Extra newline for cleaner output
				}
			}
		}

		return nil
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: $XDG_CONFIG_HOME/greg/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
	rootCmd.PersistentFlags().BoolVar(&debugLinks, "debug-links", false, "launch TUI in debug links mode")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug mode (verbose HTTP logging, skip playback, print JSON output)")
	rootCmd.PersistentFlags().BoolVar(&dubFlag, "dub", false, "use dubbed audio track (overrides config)")
	rootCmd.PersistentFlags().BoolVar(&subFlag, "sub", false, "use subbed audio track (overrides config)")

	// Mark as mutually exclusive
	rootCmd.MarkFlagsMutuallyExclusive("dub", "sub")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(providersCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(watchpartyCmd)
}

// versionCmd displays version information
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("greg version %s\n", version)
		fmt.Printf("Commit: %s\n", commit)
		fmt.Printf("Built: %s\n", date)
	},
}

// configCmd handles configuration operations
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate default configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Determine config path
		configPath := cfgFile
		if configPath == "" {
			// Use default location if not specified
			configPath = filepath.Join(config.GetConfigDir(), "config.yaml")
		}

		// Check if config file already exists
		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("configuration file already exists: %s", configPath)
		}

		// Create directory if it doesn't exist
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}

		// Generate and save default configuration
		if err := config.SaveDefaultConfig(configPath); err != nil {
			return fmt.Errorf("failed to save default configuration: %w", err)
		}

		fmt.Printf("Default configuration generated successfully at: %s\n", configPath)
		fmt.Printf("You can now edit this file to customize greg's settings.\n")
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Config file: %s\n", cfgFile)
		fmt.Printf("Log level: %s\n", cfg.Logging.Level)
		fmt.Printf("Database: %s\n", cfg.Database.Path)
		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Display configuration file path",
	Run: func(cmd *cobra.Command, args []string) {
		if cfgFile != "" {
			fmt.Println(cfgFile)
		} else {
			fmt.Println(config.GetConfigDir())
		}
	},
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathCmd)
}

// searchCmd searches for media
var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for anime, movies, or TV shows",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		providerName, _ := cmd.Flags().GetString("provider")
		mediaType, _ := cmd.Flags().GetString("type")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get provider
		var provider providers.Provider
		var err error

		if providerName != "" {
			provider, err = providers.Get(providerName)
			if err != nil {
				return fmt.Errorf("provider %s not found: %w", providerName, err)
			}
		} else {
			// Determine which provider to use based on type
			switch mediaType {
			case "anime":
				provider, err = providers.Get(cfg.Providers.Default.Anime)
				if err != nil {
					allProviders := providers.GetByType(providers.MediaTypeAnime)
					if len(allProviders) == 0 {
						return fmt.Errorf("no anime providers available")
					}
					provider = allProviders[0]
				}
			case "movie", "movies", "tv", "shows":
				provider, err = providers.Get(cfg.Providers.Default.MoviesAndTV)
				if err != nil {
					allProviders := providers.GetByType(providers.MediaTypeMovieTV)
					if len(allProviders) == 0 {
						return fmt.Errorf("no movie/TV providers available")
					}
					provider = allProviders[0]
				}
			default:
				// Default to anime if not specified
				provider, err = providers.Get(cfg.Providers.Default.Anime)
				if err != nil {
					allProviders := providers.GetByType(providers.MediaTypeAnime)
					if len(allProviders) == 0 {
						return fmt.Errorf("no providers available")
					}
					provider = allProviders[0]
				}
			}
		}

		logger.Info("searching", "query", query, "provider", provider.Name())

		// Search
		results, err := provider.Search(ctx, query)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		// Filter results by type if specified
		var filteredResults []providers.Media
		if mediaType != "" {
			for _, result := range results {
				switch mediaType {
				case "anime":
					if result.Type == providers.MediaTypeAnime {
						filteredResults = append(filteredResults, result)
					}
				case "movie", "movies":
					if result.Type == providers.MediaTypeMovie {
						filteredResults = append(filteredResults, result)
					}
				case "tv", "shows":
					if result.Type == providers.MediaTypeTV {
						filteredResults = append(filteredResults, result)
					}
				}
			}

			// Apply filter if we matched a supported type
			if mediaType == "anime" || mediaType == "movie" || mediaType == "movies" || mediaType == "tv" || mediaType == "shows" {
				results = filteredResults
			}
		}

		// Display results
		fmt.Printf("Found %d results from %s:\n\n", len(results), provider.Name())
		for i, media := range results {
			fmt.Printf("%d. %s (%d)\n", i+1, media.Title, media.Year)
			fmt.Printf("   ID: %s\n", media.ID)
			fmt.Printf("   Type: %s\n", media.Type)
			if media.Rating > 0 {
				fmt.Printf("   Rating: %.1f/10\n", media.Rating)
			}
			if len(media.Genres) > 0 {
				fmt.Printf("   Genres: %v\n", media.Genres)
			}
			if media.Status != "" {
				fmt.Printf("   Status: %s\n", media.Status)
			}
			if media.TotalEpisodes > 0 {
				fmt.Printf("   Episodes: %d\n", media.TotalEpisodes)
			}
			fmt.Println()
		}

		return nil
	},
}

// downloadCmd handles downloading media
var downloadCmd = &cobra.Command{
	Use:   "download <media-id>",
	Short: "Download anime, movies, or TV shows",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mediaID := args[0]
		providerName, _ := cmd.Flags().GetString("provider")
		mediaType, _ := cmd.Flags().GetString("type")
		episodeRange, _ := cmd.Flags().GetString("episode")
		quality, _ := cmd.Flags().GetString("quality")
		outputDir, _ := cmd.Flags().GetString("output")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get provider
		var provider providers.Provider
		var err error

		if providerName != "" {
			provider, err = providers.Get(providerName)
			if err != nil {
				return fmt.Errorf("provider %s not found: %w", providerName, err)
			}
		} else {
			// Determine which provider to use based on type
			switch mediaType {
			case "anime":
				provider, err = providers.Get(cfg.Providers.Default.Anime)
				if err != nil {
					allProviders := providers.GetByType(providers.MediaTypeAnime)
					if len(allProviders) == 0 {
						return fmt.Errorf("no anime providers available")
					}
					provider = allProviders[0]
				}
			case "movie", "movies", "tv", "shows":
				provider, err = providers.Get(cfg.Providers.Default.MoviesAndTV)
				if err != nil {
					allProviders := providers.GetByType(providers.MediaTypeMovieTV)
					if len(allProviders) == 0 {
						return fmt.Errorf("no movie/TV providers available")
					}
					provider = allProviders[0]
				}
			default:
				// Default to anime if not specified
				provider, err = providers.Get(cfg.Providers.Default.Anime)
				if err != nil {
					allProviders := providers.GetByType(providers.MediaTypeAnime)
					if len(allProviders) == 0 {
						return fmt.Errorf("no providers available")
					}
					provider = allProviders[0]
				}
			}
		}

		logger.Info("downloading", "media_id", mediaID, "provider", provider.Name())

		// Check if it's a movie (no episodes) or TV/anime (with episodes)
		mediaDetails, err := provider.GetMediaDetails(ctx, mediaID)
		if err != nil {
			return fmt.Errorf("failed to get media details: %w", err)
		}

		// If it's a movie, get the movie episode ID directly
		if len(mediaDetails.Seasons) == 0 && mediaDetails.Type == providers.MediaTypeMovie {
			// This is a movie, so get the episode ID directly
			type movieEpisodeIDGetter interface {
				GetMovieEpisodeID(ctx context.Context, mediaID string) (string, error)
			}

			episodeIDGetter, ok := provider.(movieEpisodeIDGetter)
			if !ok {
				return fmt.Errorf("provider does not support direct movie playback/download")
			}

			episodeID, err := episodeIDGetter.GetMovieEpisodeID(context.Background(), mediaID)
			if err != nil {
				return fmt.Errorf("failed to get movie episode ID: %w", err)
			}

			// Get the stream URL
			parsedQuality := providers.Quality1080p
			if quality != "" {
				if q, err := providers.ParseQuality(quality); err == nil {
					parsedQuality = q
				} else {
					fmt.Fprintf(os.Stderr, "Invalid quality %s, using default 1080p\n", quality)
				}
			}

			stream, err := provider.GetStreamURL(ctx, episodeID, parsedQuality)
			if err != nil {
				return fmt.Errorf("failed to get stream URL: %w", err)
			}

			// Initialize download manager
			downloadMgr, err := downloader.NewManager(database.DB, &cfg.Downloads, logger)
			if err != nil {
				return fmt.Errorf("failed to initialize download manager: %w", err)
			}

			// Start the download manager
			if err := downloadMgr.Start(ctx); err != nil {
				return fmt.Errorf("failed to start download manager: %w", err)
			}
			defer func() { _ = downloadMgr.Stop() }()

			// Create download task
			task := downloader.DownloadTask{
				MediaID:    mediaID,
				MediaTitle: mediaDetails.Title,
				MediaType:  mediaDetails.Type,
				Episode:    1, // For movies
				Quality:    parsedQuality,
				Provider:   provider.Name(),
				StreamURL:  stream.URL,
				StreamType: stream.Type,
				Headers:    stream.Headers,
				Referer:    stream.Referer,
				Subtitles:  stream.Subtitles,
				EmbedSubs:  cfg.Downloads.EmbedSubtitles,
			}

			// Set output directory if specified
			if outputDir != "" {
				downloadMgr.SetOutputDir(outputDir)
			}

			// Add to download queue
			if err := downloadMgr.AddToQueue(ctx, task); err != nil {
				return fmt.Errorf("failed to add download to queue: %w", err)
			}

			// Wait for download to complete
			fmt.Printf("Downloading %s...\n", mediaDetails.Title)
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()

			for {
				<-ticker.C
				queue, err := downloadMgr.GetQueue(ctx)
				if err != nil {
					return fmt.Errorf("failed to get download queue: %w", err)
				}

				if len(queue) == 0 {
					break
				}

				task := queue[0]
				if task.Status.IsComplete() {
					if task.Status == downloader.StatusCompleted {
						fmt.Printf("Download completed: %s\n", task.OutputPath)
					} else {
						fmt.Printf("Download failed: %s\n", task.Error)
					}
					break
				}

				fmt.Printf("\rProgress: %.1f%%", task.Progress)
			}

			return nil
		} else {
			// This is TV/anime with episodes
			// For now, just download the first season
			if len(mediaDetails.Seasons) == 0 {
				return fmt.Errorf("no seasons found for %s", mediaDetails.Title)
			}

			// Get episodes for the first season
			episodes, err := provider.GetEpisodes(ctx, mediaDetails.Seasons[0].ID)
			if err != nil {
				return fmt.Errorf("failed to get episodes: %w", err)
			}

			// Determine which episodes to download based on range
			var targetEpisodes []providers.Episode
			if episodeRange != "" {
				// Parse episode range (e.g., "1-5", "1,3,5", "1-5,7,9")
				targetEpisodes, err = providers.ParseEpisodeRange(episodes, episodeRange)
				if err != nil {
					return fmt.Errorf("failed to parse episode range: %w", err)
				}
			} else {
				// Default to all episodes if no range specified
				targetEpisodes = episodes
			}

			// Initialize download manager
			downloadMgr, err := downloader.NewManager(database.DB, &cfg.Downloads, logger)
			if err != nil {
				return fmt.Errorf("failed to initialize download manager: %w", err)
			}

			// Start the download manager
			if err := downloadMgr.Start(ctx); err != nil {
				return fmt.Errorf("failed to start download manager: %w", err)
			}
			defer func() { _ = downloadMgr.Stop() }()

			// Set output directory if specified
			if outputDir != "" {
				downloadMgr.SetOutputDir(outputDir)
			}

			// Download each episode
			parsedQuality := providers.Quality1080p
			if quality != "" {
				if q, err := providers.ParseQuality(quality); err == nil {
					parsedQuality = q
				} else {
					fmt.Fprintf(os.Stderr, "Invalid quality %s, using default 1080p\n", quality)
				}
			}

			for _, episode := range targetEpisodes {
				fmt.Printf("Getting stream for %s - Episode %d...\n", mediaDetails.Title, episode.Number)

				// Get the stream URL for this episode
				stream, err := provider.GetStreamURL(ctx, episode.ID, parsedQuality)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to get stream for episode %d: %v\n", episode.Number, err)
					continue
				}

				// Create download task
				task := downloader.DownloadTask{
					MediaID:    mediaID,
					MediaTitle: mediaDetails.Title,
					MediaType:  mediaDetails.Type,
					Episode:    episode.Number,
					Season:     0, // We can extract this from episode data if needed
					Quality:    parsedQuality,
					Provider:   provider.Name(),
					StreamURL:  stream.URL,
					StreamType: stream.Type,
					Headers:    stream.Headers,
					Referer:    stream.Referer,
					Subtitles:  stream.Subtitles,
					EmbedSubs:  cfg.Downloads.EmbedSubtitles,
				}

				// Add to download queue
				if err := downloadMgr.AddToQueue(ctx, task); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to add episode %d to queue: %v\n", episode.Number, err)
					continue
				}
			}

			// Monitor progress
			fmt.Printf("Downloading %d episodes of %s...\n", len(targetEpisodes), mediaDetails.Title)
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()

			for {
				<-ticker.C
				queue, err := downloadMgr.GetQueue(ctx)
				if err != nil {
					return fmt.Errorf("failed to get download queue: %w", err)
				}

				if len(queue) == 0 {
					break
				}

				// Check if all downloads are complete
				allComplete := true
				for _, task := range queue {
					if !task.Status.IsComplete() {
						allComplete = false
						break
					}
				}

				if allComplete {
					fmt.Println("\nAll downloads completed!")
					break
				}

				// Show summary
				completed := 0
				for _, task := range queue {
					if task.Status == downloader.StatusCompleted {
						completed++
					}
				}

				fmt.Printf("\rProgress: %d/%d completed", completed, len(queue))
			}

			return nil
		}
	},
}

func init() {
	searchCmd.Flags().StringP("provider", "p", "", "provider to use (default: auto-detect by type)")
	searchCmd.Flags().StringP("type", "t", "anime", "media type: anime, movie, movies, tv, shows (default: anime)")
}

// providersCmd manages providers
var providersCmd = &cobra.Command{
	Use:   "providers",
	Short: "Manage streaming providers",
}

var providersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available providers",
	Run: func(cmd *cobra.Command, args []string) {
		providersList := providers.List()

		if len(providersList) == 0 {
			fmt.Println("No providers registered")
			return
		}

		fmt.Printf("Available providers (%d):\n\n", len(providersList))
		for _, name := range providersList {
			provider, _ := providers.Get(name)
			fmt.Printf("- %s (Type: %s)\n", name, provider.Type())
		}
	},
}

var providersInfoCmd = &cobra.Command{
	Use:   "info <provider-name>",
	Short: "Get information about a provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		provider, err := providers.Get(name)
		if err != nil {
			return fmt.Errorf("provider %s not found: %w", name, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		fmt.Printf("Provider: %s\n", provider.Name())
		fmt.Printf("Type: %s\n", provider.Type())

		// Check availability
		fmt.Print("Status: ")
		if err := provider.HealthCheck(ctx); err == nil {
			fmt.Println("Available ✓")
		} else {
			fmt.Printf("Unavailable ✗ (%v)\n", err)
		}

		return nil
	},
}

func init() {
	providersCmd.AddCommand(providersListCmd)
	providersCmd.AddCommand(providersInfoCmd)
}

// debugCmd provides debugging utilities
var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debugging utilities",
}

var debugLinksCmd = &cobra.Command{
	Use:   "links <query>",
	Short: "Find and display video links for a query",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		providerName, _ := cmd.Flags().GetString("provider")
		mediaTypeStr, _ := cmd.Flags().GetString("type")
		episodeStr, _ := cmd.Flags().GetString("episode")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Parse media type
		var mediaType providers.MediaType
		switch mediaTypeStr {
		case "anime":
			mediaType = providers.MediaTypeAnime
		case "movie", "movies":
			mediaType = providers.MediaTypeMovie
		case "tv", "tvshows":
			mediaType = providers.MediaTypeTV
		case "movie_tv":
			mediaType = providers.MediaTypeMovieTV
		default:
			mediaType = providers.MediaTypeAnime // Default to anime
		}

		// Get provider
		var provider providers.Provider
		var err error

		if providerName != "" {
			provider, err = providers.Get(providerName)
			if err != nil {
				return fmt.Errorf("provider %s not found: %w", providerName, err)
			}
		} else {
			// Use first available provider of the specified type
			providerList := providers.GetByType(mediaType)
			if len(providerList) == 0 {
				// If no provider of specific type, try to find any available provider
				allProviders := providers.GetAll()
				if len(allProviders) == 0 {
					return fmt.Errorf("no providers available")
				}
				provider = allProviders[0]
				logger.Info("using fallback provider", "provider", provider.Name())
			} else {
				provider = providerList[0]
				logger.Info("using provider", "provider", provider.Name())
			}
		}

		logger.Info("searching for media", "query", query, "provider", provider.Name())

		// Search for the media
		results, err := provider.Search(ctx, query)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		if len(results) == 0 {
			return fmt.Errorf("no results found for query: %s", query)
		}

		// Use the first result for now
		media := results[0]
		logger.Info("found media", "title", media.Title, "id", media.ID)

		fmt.Printf("Media: %s (ID: %s)\n", media.Title, media.ID)
		fmt.Printf("Type: %s\n", media.Type)
		if media.TotalEpisodes > 0 {
			fmt.Printf("Total Episodes: %d\n", media.TotalEpisodes)
		}
		fmt.Println()

		// Get seasons from provider
		seasons, err := provider.GetSeasons(ctx, media.ID)
		if err != nil {
			return fmt.Errorf("failed to get seasons: %w", err)
		}

		var episodes []providers.Episode
		if len(seasons) > 0 {
			// For debugging, just use the first season
			firstSeason := seasons[0]
			logger.Info("using first season", "season_id", firstSeason.ID, "season_title", firstSeason.Title)

			episodes, err = provider.GetEpisodes(ctx, firstSeason.ID)
			if err != nil {
				return fmt.Errorf("failed to get episodes: %w", err)
			}
		} else if media.Type == providers.MediaTypeMovie {
			// It's a movie, so we need to get the movie episode ID
			type movieEpisodeIDGetter interface {
				GetMovieEpisodeID(ctx context.Context, mediaID string) (string, error)
			}

			episodeIDGetter, ok := provider.(movieEpisodeIDGetter)
			if !ok {
				return fmt.Errorf("provider does not support direct movie playback")
			}

			episodeID, err := episodeIDGetter.GetMovieEpisodeID(context.Background(), media.ID)
			if err != nil {
				return fmt.Errorf("failed to get movie episode ID: %w", err)
			}
			episodes = []providers.Episode{
				{
					ID:     episodeID,
					Number: 1,
					Title:  media.Title,
				},
			}
		}

		// If no episodes were returned (fallback), create a single episode
		if len(episodes) == 0 {
			episodes = []providers.Episode{
				{
					ID:     media.ID,
					Number: 1,
					Title:  "Movie",
				},
			}
		}

		// Determine which episode to get stream URL for
		var targetEpisode *providers.Episode
		if episodeStr != "" {
			episodeNum, err := strconv.Atoi(episodeStr)
			if err != nil {
				return fmt.Errorf("invalid episode number: %s", episodeStr)
			}

			for _, ep := range episodes {
				if ep.Number == episodeNum {
					targetEpisode = &ep
					break
				}
			}

			if targetEpisode == nil {
				return fmt.Errorf("episode %d not found", episodeNum)
			}
		} else {
			// Use first episode if no specific episode requested
			if len(episodes) > 0 {
				targetEpisode = &episodes[0]
			} else {
				return fmt.Errorf("no episodes available")
			}
		}

		// Get stream URL for the specific episode
		logger.Info("getting stream URL", "episode", targetEpisode.Number, "episode_id", targetEpisode.ID)

		fmt.Printf("Attempting to get stream URL for episode ID: %s\n", targetEpisode.ID)

		stream, err := provider.GetStreamURL(ctx, targetEpisode.ID, providers.QualityAuto)
		if err != nil {
			return fmt.Errorf("failed to get stream URL: %w", err)
		}

		// Display the stream info
		fmt.Printf("Episode: %s (Number: %d)\n", targetEpisode.Title, targetEpisode.Number)
		fmt.Printf("Stream URL: %s\n", stream.URL)
		fmt.Printf("Quality: %s\n", stream.Quality)
		fmt.Printf("Type: %s\n", stream.Type)
		if stream.Referer != "" {
			fmt.Printf("Referer: %s\n", stream.Referer)
		}

		fmt.Println("\nHeaders:")
		for key, value := range stream.Headers {
			fmt.Printf("  %s: %s\n", key, value)
		}

		fmt.Println("\nSubtitles:")
		if len(stream.Subtitles) == 0 {
			fmt.Println("  (none available)")
		} else {
			for i, subtitle := range stream.Subtitles {
				fmt.Printf("  %d. Language: %s, Format: %s\n", i+1, subtitle.Language, subtitle.Format)
				fmt.Printf("     URL: %s\n", subtitle.URL)
			}
		}

		return nil
	},
}

func init() {
	debugCmd.AddCommand(debugLinksCmd)
	debugLinksCmd.Flags().StringP("provider", "p", "", "provider to use (default: first available of specified type)")
	debugLinksCmd.Flags().StringP("type", "t", "anime", "media type (anime, movie, tv, movie_tv)")
	debugLinksCmd.Flags().StringP("episode", "e", "", "specific episode number to get links for")
	rootCmd.AddCommand(debugCmd)
}

// authCmd handles authentication for tracking services
var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with tracking services",
}

var authAniListCmd = &cobra.Command{
	Use:   "anilist",
	Short: "Authenticate with AniList",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !cfg.Tracker.AniList.Enabled {
			return fmt.Errorf("AniList tracking is disabled in config")
		}

		tokenStorage := anilist.NewTokenStorage(database.DB)
		// Use Browser credentials as default for normal operation
		client := anilist.NewClient(anilist.Config{
			ClientID:    anilist.AuthBrowserClientID,
			RedirectURI: anilist.AuthBrowserRedirectURI,
			SaveToken:   tokenStorage.SaveToken,
			LoadToken:   tokenStorage.LoadToken,
		})

		if client.IsAuthenticated() {
			fmt.Println("Already authenticated with AniList")
			logout, _ := cmd.Flags().GetBool("logout")
			if logout {
				if err := client.Logout(); err != nil {
					return fmt.Errorf("failed to logout: %w", err)
				}
				fmt.Println("Successfully logged out from AniList")
			}
			return nil
		}

		// Prompt for authentication method
		fmt.Println("Select Authentication Method:")
		fmt.Println("  1. Browser Authentication (Automatic Callback) [Recommended]")
		fmt.Println("  2. Manual / Greg Link (Copy-Paste Token)")
		fmt.Print("\nEnter choice (1-2): ")

		reader := bufio.NewReader(os.Stdin)
		methodChoice, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read choice: %w", err)
		}
		methodChoice = strings.TrimSpace(methodChoice)

		var accessToken string

		switch methodChoice {
		case "1":
			// Option 1: Browser Authentication (Automatic Callback)
			fmt.Println("Starting browser-based authentication...")
			authConfig := anilist.BrowserAuthConfig{
				ClientID:     anilist.AuthBrowserClientID,
				ClientSecret: anilist.AuthBrowserClientSecret,
				RedirectURI:  anilist.AuthBrowserRedirectURI,
				ServerPort:   cfg.Tracker.AniList.ServerPort,
			}
			token, err := anilist.AuthenticateWithBrowser(context.Background(), authConfig, tokenStorage.SaveToken)
			if err != nil {
				return fmt.Errorf("browser authentication failed: %w", err)
			}
			accessToken = token.AccessToken

		case "2":
			// Option 2: Manual / Greg Link
			// Generate Auth URL manually for this client
			authURL := fmt.Sprintf("%s?client_id=%s&response_type=token", "https://anilist.co/api/v2/oauth/authorize", anilist.AuthManualClientID)

			fmt.Printf("Please visit this URL to authenticate:\n\n%s\n\n", authURL)
			fmt.Println("After approving, copy the access token displayed on the page.")
			fmt.Println()

			// Provide input options
			fmt.Println("Choose how to provide your token:")
			fmt.Println("  1. Read from clipboard (recommended for macOS)")
			fmt.Println("  2. Read from file")
			fmt.Println("  3. Type/paste manually")
			fmt.Print("\nEnter choice (1-3): ")

			choice, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read choice: %w", err)
			}
			choice = strings.TrimSpace(choice)

			switch choice {
			case "1":
				// Read from clipboard
				clipboardSvc := clipboard.NewService(logger)
				accessToken, err = tui.ReadFromClipboard(clipboardSvc, cfg)
				if err != nil {
					return fmt.Errorf("failed to read from clipboard: %w\nTip: Copy the token first, then select option 1", err)
				}
				accessToken = anilist.ExtractTokenFromInput(accessToken)
				fmt.Printf("✓ Token read from clipboard (%d characters)\n", len(accessToken))

			case "2":
				// Read from file
				fmt.Print("Enter file path: ")
				filePath, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read file path: %w", err)
				}
				filePath = strings.TrimSpace(filePath)

				tokenBytes, err := os.ReadFile(filePath)
				if err != nil {
					return fmt.Errorf("failed to read file: %w", err)
				}
				accessToken = anilist.ExtractTokenFromInput(strings.TrimSpace(string(tokenBytes)))
				fmt.Printf("✓ Token read from file (%d characters)\n", len(accessToken))

			case "3":
				// Manual input
				fmt.Print("Enter access token: ")
				accessToken, err = reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read access token: %w", err)
				}
				accessToken = anilist.ExtractTokenFromInput(strings.TrimSpace(accessToken))
				fmt.Printf("✓ Token received (%d characters)\n", len(accessToken))

			default:
				return fmt.Errorf("invalid choice: %s", choice)
			}

		default:
			return fmt.Errorf("invalid authentication method: %s", methodChoice)
		}

		if len(accessToken) == 0 {
			return fmt.Errorf("empty token received")
		}

		// Additional validation and cleanup
		accessToken = strings.TrimSpace(accessToken)
		accessToken = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r == '\t' {
				return -1
			}
			return r
		}, accessToken)

		if len(accessToken) < 20 {
			return fmt.Errorf("token seems too short (%d chars). Please ensure you copied the complete token", len(accessToken))
		}

		// Save the token
		// Note: We use the 'client' created earlier to exchange/save, which was initialized with Browser ClientID.
		// Since we have the token directly (implicit flow or manual), we just need to save it.
		// The client.ExchangeCode method just creates a token struct and saves it, keying off the token itself, not client ID.
		// Use a fresh context for saving
		ctxSaving, cancelSaving := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelSaving()

		if err := client.ExchangeCode(ctxSaving, accessToken); err != nil {
			return fmt.Errorf("failed to save token: %w", err)
		}

		// Verify the token works by getting current user
		fmt.Print("Verifying authentication... ")
		// Use the client we already have or creating a new one with the token?
		// Ensure the client has the token set (ExchangeCode sets it on the struct)
		userID, username, err := client.GetCurrentUser(ctxSaving)
		if err != nil {
			return fmt.Errorf("authentication failed: %w\nPlease ensure you copied the complete access token from AniList", err)
		}

		fmt.Printf("✓\nSuccessfully authenticated with AniList!\nLogged in as: %s (ID: %d)\n", username, userID)
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Tracker.AniList.Enabled {
			tokenStorage := anilist.NewTokenStorage(database.DB)
			client := anilist.NewClient(anilist.Config{
				ClientID:    anilist.AuthBrowserClientID,
				RedirectURI: anilist.AuthBrowserRedirectURI,
				LoadToken:   tokenStorage.LoadToken,
			})

			fmt.Print("AniList: ")
			if client.IsAuthenticated() {
				fmt.Println("Authenticated ✓")
			} else {
				fmt.Println("Not authenticated ✗")
			}
		} else {
			fmt.Println("AniList: Disabled")
		}

		return nil
	},
}

func init() {
	// Add download command flags
	downloadCmd.Flags().StringP("provider", "p", "", "provider to use (default: auto-detect by type)")
	downloadCmd.Flags().StringP("type", "t", "anime", "media type: anime, movie, movies, tv, shows (default: anime)")
	downloadCmd.Flags().StringP("episode", "e", "", "episode range (e.g., 1-5, 7, 9-12) - TV/anime only")
	downloadCmd.Flags().StringP("quality", "q", "1080p", "video quality (360p, 480p, 720p, 1080p, etc.)")
	downloadCmd.Flags().StringP("output", "o", "", "output directory (default: config setting)")

	authCmd.AddCommand(authAniListCmd)
	authCmd.AddCommand(authStatusCmd)
	authAniListCmd.Flags().Bool("logout", false, "logout from AniList")
}

// watchpartyCmd creates a WatchParty room for streaming media
var watchpartyCmd = &cobra.Command{
	Use:   "watchparty <query>",
	Short: "Create a WatchParty room for streaming media",
	Long: `Create a WatchParty room for watching media with friends.
This command searches for the specified media, gets the stream URL,
applies proxy headers if needed, and generates a WatchParty URL.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		providerName, _ := cmd.Flags().GetString("provider")
		mediaType, _ := cmd.Flags().GetString("type")
		episodeNum, _ := cmd.Flags().GetInt("episode")
		qualityStr, _ := cmd.Flags().GetString("quality")
		proxyURL, _ := cmd.Flags().GetString("proxy")
		origin, _ := cmd.Flags().GetString("origin")
		openBrowser, _ := cmd.Flags().GetBool("open")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Determine quality
		quality := providers.Quality1080p
		if qualityStr != "" {
			parsedQuality, err := providers.ParseQuality(qualityStr)
			if err != nil {
				return fmt.Errorf("invalid quality: %w", err)
			}
			quality = parsedQuality
		}

		// Get provider
		var provider providers.Provider
		var err error

		if providerName != "" {
			provider, err = providers.Get(providerName)
			if err != nil {
				return fmt.Errorf("provider %s not found: %w", providerName, err)
			}
		} else {
			// Determine which provider to use based on type
			switch mediaType {
			case "anime":
				provider, err = providers.Get(cfg.Providers.Default.Anime)
				if err != nil {
					allProviders := providers.GetByType(providers.MediaTypeAnime)
					if len(allProviders) == 0 {
						return fmt.Errorf("no anime providers available")
					}
					provider = allProviders[0]
				}
			case "movie", "movies", "tv", "shows":
				provider, err = providers.Get(cfg.Providers.Default.MoviesAndTV)
				if err != nil {
					allProviders := providers.GetByType(providers.MediaTypeMovieTV)
					if len(allProviders) == 0 {
						return fmt.Errorf("no movie/TV providers available")
					}
					provider = allProviders[0]
				}
			default:
				// Default to anime if not specified
				provider, err = providers.Get(cfg.Providers.Default.Anime)
				if err != nil {
					allProviders := providers.GetByType(providers.MediaTypeAnime)
					if len(allProviders) == 0 {
						return fmt.Errorf("no providers available")
					}
					provider = allProviders[0]
				}
			}
		}

		logger.Info("searching for media", "query", query, "provider", provider.Name())

		// Search for media
		results, err := provider.Search(ctx, query)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		if len(results) == 0 {
			return fmt.Errorf("no results found for query: %s", query)
		}

		// Use the first result for now
		media := results[0]
		logger.Info("found media", "title", media.Title, "id", media.ID)

		// Get seasons from provider to determine if it's TV/anime or movie
		seasons, err := provider.GetSeasons(ctx, media.ID)
		if err != nil {
			return fmt.Errorf("failed to get seasons: %w", err)
		}

		var episodeID string
		var episodeNumber int

		if len(seasons) > 0 {
			// This is TV/anime with episodes
			var episodes []providers.Episode
			if len(seasons) > 0 {
				// For simplicity, use the first season
				firstSeason := seasons[0]
				logger.Info("using first season", "season_id", firstSeason.ID, "season_title", firstSeason.Title)

				episodes, err = provider.GetEpisodes(ctx, firstSeason.ID)
				if err != nil {
					return fmt.Errorf("failed to get episodes: %w", err)
				}
			}

			// Determine which episode to use
			if episodeNum > 0 {
				// Specific episode requested
				for _, ep := range episodes {
					if ep.Number == episodeNum {
						episodeID = ep.ID
						episodeNumber = ep.Number
						break
					}
				}
				if episodeID == "" {
					return fmt.Errorf("episode %d not found", episodeNum)
				}
			} else {
				// Use first episode if no specific episode requested
				if len(episodes) > 0 {
					episodeID = episodes[0].ID
					episodeNumber = episodes[0].Number
				} else {
					return fmt.Errorf("no episodes available")
				}
			}
		} else if media.Type == providers.MediaTypeMovie {
			// It's a movie, so we need to get the movie episode ID
			type movieEpisodeIDGetter interface {
				GetMovieEpisodeID(ctx context.Context, mediaID string) (string, error)
			}

			episodeIDGetter, ok := provider.(movieEpisodeIDGetter)
			if !ok {
				return fmt.Errorf("provider does not support direct movie playback")
			}

			episodeID, err = episodeIDGetter.GetMovieEpisodeID(context.Background(), media.ID)
			if err != nil {
				return fmt.Errorf("failed to get movie episode ID: %w", err)
			}
			episodeNumber = 1
		} else {
			// Fallback: treat as single episode
			episodeID = media.ID
			episodeNumber = 1
		}

		// Create WatchParty manager
		wpConfig := watchparty.Config{
			Enabled:         cfg.WatchParty.Enabled,
			DefaultProxy:    cfg.WatchParty.DefaultProxy,
			AutoOpenBrowser: cfg.WatchParty.AutoOpenBrowser,
			DefaultOrigin:   cfg.WatchParty.DefaultOrigin,
		}
		wpManager := watchparty.NewManager(wpConfig)

		// Determine proxy configuration
		finalProxyURL := proxyURL
		if finalProxyURL == "" {
			finalProxyURL = cfg.WatchParty.DefaultProxy
		}

		finalOrigin := origin
		if finalOrigin == "" {
			finalOrigin = cfg.WatchParty.DefaultOrigin
		}

		// Generate proxied URL if needed
		proxyConfig := watchparty.ProxyConfig{
			ProxyURL: finalProxyURL,
			Origin:   finalOrigin,
			Referer:  "", // Will be set from stream later in CreateWatchParty
		}

		logger.Info("creating watchparty", "media_id", media.ID, "episode_id", episodeID, "quality", quality, "proxy", finalProxyURL)

		// Create WatchParty URL
		watchPartyURL, err := wpManager.CreateWatchParty(ctx, provider, media.ID, episodeID, quality, proxyConfig)
		if err != nil {
			return fmt.Errorf("failed to create WatchParty: %w", err)
		}

		fmt.Printf("WatchParty room created!\n")
		fmt.Printf("Media: %s\n", media.Title)
		if episodeNumber > 0 {
			fmt.Printf("Episode: %d\n", episodeNumber)
		}
		fmt.Printf("WatchParty URL: %s\n", watchPartyURL)

		// Optionally open browser
		autoOpen := cfg.WatchParty.AutoOpenBrowser
		if openBrowser {
			autoOpen = true
		}
		if autoOpen {
			if err := watchparty.OpenURL(watchPartyURL); err != nil {
				logger.Error("failed to open browser", "error", err)
				fmt.Printf("Failed to open browser: %v\n", err)
				fmt.Printf("Please open the URL manually in your browser.\n")
			} else {
				fmt.Printf("Opening WatchParty room in your browser...\n")
			}
		} else {
			fmt.Printf("Use the above URL to join the WatchParty room.\n")
		}

		return nil
	},
}

func init() {
	// Add watchparty command flags
	watchpartyCmd.Flags().StringP("provider", "p", "", "provider to use (default: auto-detect by type)")
	watchpartyCmd.Flags().StringP("type", "t", "anime", "media type: anime, movie, movies, tv, shows (default: anime)")
	watchpartyCmd.Flags().IntP("episode", "e", 0, "specific episode number (TV/anime only)")
	watchpartyCmd.Flags().StringP("quality", "q", "1080p", "video quality (360p, 480p, 720p, 1080p, etc.)")
	watchpartyCmd.Flags().StringP("proxy", "", "", "m3u8 proxy URL (overrides default)")
	watchpartyCmd.Flags().StringP("origin", "", "", "Origin header for proxy (overrides default)")
	watchpartyCmd.Flags().BoolP("open", "o", false, "Open browser to WatchParty room")
}
