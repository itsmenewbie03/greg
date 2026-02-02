package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gorm.io/gorm"

	"github.com/justchokingaround/greg/internal/clipboard"
	"github.com/justchokingaround/greg/internal/config"
	"github.com/justchokingaround/greg/internal/database"
	"github.com/justchokingaround/greg/internal/downloader"
	historyservice "github.com/justchokingaround/greg/internal/history"
	"github.com/justchokingaround/greg/internal/player"
	"github.com/justchokingaround/greg/internal/player/mpv"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tracker"
	"github.com/justchokingaround/greg/internal/tracker/mapping"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/components/anilist"
	"github.com/justchokingaround/greg/internal/tui/components/audioselect"
	"github.com/justchokingaround/greg/internal/tui/components/downloads"
	"github.com/justchokingaround/greg/internal/tui/components/episodes"
	"github.com/justchokingaround/greg/internal/tui/components/help"

	"github.com/justchokingaround/greg/internal/tui/components/history"
	"github.com/justchokingaround/greg/internal/tui/components/home"
	"github.com/justchokingaround/greg/internal/tui/components/manga"
	"github.com/justchokingaround/greg/internal/tui/components/mangadownload"
	"github.com/justchokingaround/greg/internal/tui/components/mangainfo"
	"github.com/justchokingaround/greg/internal/tui/components/providerstatus"
	"github.com/justchokingaround/greg/internal/tui/components/results"
	"github.com/justchokingaround/greg/internal/tui/components/search"
	"github.com/justchokingaround/greg/internal/tui/components/seasons"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

type sessionState int

// clearStatusMsg is an internal message to clear the status message
type clearStatusMsg struct{}

// dismissDownloadNotificationMsg dismisses the download notification popup
type dismissDownloadNotificationMsg struct{}

const (
	homeView sessionState = iota
	searchView
	resultsView
	loadingView
	errorView
	seasonView
	episodeView
	launchingPlayerView
	playingView
	playbackCompletedView
	audioSelectView
	anilistView
	providerSelectionView
	downloadsView
	historyView
	mangaReaderView
	mangaInfoView
	providerStatusView
	mangaDownloadProgressView
)

type loadingOperation int

const (
	loadingSearch loadingOperation = iota
	loadingSeasons
	loadingEpisodes
	loadingStream
	loadingMangaPages
	loadingAniListLibrary
	loadingProviderSearch
	loadingMangaInfo
)

type App struct {
	state                   sessionState
	loadingOp               loadingOperation
	width                   int
	height                  int
	providers               map[providers.MediaType]providers.Provider
	currentMediaType        providers.MediaType
	home                    *home.Model
	search                  search.Model
	results                 results.Model
	seasons                 seasons.Model
	episodeListModel        results.Model
	episodesComponent       episodes.Model
	audioSelectorModel      *audioselect.Model
	anilistComponent        anilist.Model
	downloadsComponent      downloads.Model
	historyComponent        history.Model
	mangaComponent          manga.Model
	mangaInfoComponent      *mangainfo.Model
	mangaDownloadComponent  mangadownload.Model
	providerStatusComponent providerstatus.Model
	historyService          *historyservice.Service
	helpComponent           help.Model
	spinner                 spinner.Model
	err                     error

	// For sending messages to UI from goroutines
	msgChan chan tea.Msg

	// For episode selection
	selectedMedia providers.Media
	seasonsList   []providers.Season
	episodes      []providers.Episode

	// For playback
	player player.Player

	// Playback state tracking
	currentEpisodeID        string
	currentEpisodeNumber    int
	currentSeasonNumber     int // Track which season is being played (for TV shows)
	currentEpisodeTitle     string
	currentPlaybackProvider string                   // Provider used for current playback session
	previousState           sessionState             // To return to after playback
	lastProgress            *player.PlaybackProgress // Store last known progress
	playbackCompletionMsg   string                   // Message to show after playback ends
	episodeCompleted        bool                     // Whether the last episode was completed (>= 85%)
	launchStartTime         time.Time                // When player launch started (for timeout)
	lastPlayedEpisodeNumber int                      // Episode number to position cursor on after playback

	// Completion confirmation modal
	showCompletionDialog bool   // Show completion confirmation modal
	completionDialogMsg  string // Formatted modal content
	pendingCompletion    bool   // Should mark episode complete on 'yes'

	// Search queries per media type
	searchQueries map[providers.MediaType]string

	// For debug links mode
	inDebugLinksMode bool
	debugInfo        *DebugInfo
	forceQuit        bool
	quitRequested    bool // Track if user wants to quit (for warning with active downloads)

	// Status message (shown briefly at bottom)
	statusMsg     string
	statusMsgTime time.Time

	// Download notification popup
	showDownloadNotification bool
	downloadNotificationMsg  string

	// Tracker integration
	trackerMgr interface{} // *tracker.Manager

	// AniList dialog state
	dialogMode  anilist.DialogMode
	dialogState anilist.DialogState

	// Database and mapping
	db         *gorm.DB
	mappingMgr interface{} // *mapping.Manager

	// Download manager
	downloadMgr  *downloader.Manager
	cfg          interface{} // *config.Config
	logger       *slog.Logger
	clipboardSvc clipboard.Service

	// AniList playback context
	watchingFromAniList     bool
	currentAniListID        int
	currentAniListMedia     *tracker.TrackedMedia
	providerSearchResults   []providers.Media
	providerName            string        // Name of provider being used
	providerSelectionResult results.Model // Reuse results component for selection
	isLastEpisode           bool
	anilistSearchRetried    bool // Track if we've already retried search to avoid infinite loops
	remapShouldSave         bool // Track if we should save the mapping after selection

	// Temporary proxy overrides for WatchParty
	tempProxyURL string
	tempOrigin   string

	// WatchParty popup state
	showWatchPartyPopup bool
	watchPartyInfo      *common.WatchPartyInfo

	// Debug sources popup state
	showDebugPopup   bool
	debugSourcesInfo *common.DebugSourcesInfo
	cameFromHistory  bool

	// Semaphore for limiting concurrent detail fetches
	detailsSem chan struct{}

	// Audio preference from CLI flag or config
	audioPreference    string               // "dub", "sub", or "" (use DB/config)
	selectedAudioTrack *int                 // User-selected audio track index from selector (nil if not set)
	pendingStream      *providers.StreamURL // Stream waiting for audio selection
}

func NewApp(providerMap map[providers.MediaType]providers.Provider, db *gorm.DB, cfg interface{}, logger *slog.Logger, audioPreference string) *App {
	// Use default logger if none provided
	if logger == nil {
		logger = slog.Default()
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.SelectedItemStyle

	// Check if debug mode is enabled
	debugMode := false
	if cfg != nil {
		if configPtr, ok := cfg.(*config.Config); ok {
			debugMode = configPtr.Advanced.Debug
		}
	}

	// Initialize MPV player with configuration
	var mpvPlayer player.Player
	if cfg != nil {
		if appCfg, ok := cfg.(*config.Config); ok {
			mpvPlayerWithConfig, err := mpv.NewMPVPlayerWithConfig(appCfg, debugMode)
			if err != nil {
				// Log the error but continue initializing the app
				logger.Warn("failed to initialize MPV player with config", "error", err)
			} else {
				mpvPlayer = mpvPlayerWithConfig
			}
		}
	}

	// Fallback to old initialization if config is not available or failed
	if mpvPlayer == nil {
		mpvPlayerWithDebug, err := mpv.NewMPVPlayerWithDebug(debugMode)
		if err != nil {
			// Log the error but continue initializing the app
			logger.Warn("failed to initialize MPV player", "error", err)
		} else {
			mpvPlayer = mpvPlayerWithDebug
		}
	}

	// Validate database connection
	if debugMode {
		logger.Debug("database connection check", "db_pointer", fmt.Sprintf("%v", db))
	}
	if db == nil {
		logger.Error("database is nil, AniList features will not work")
	} else {
		// Test database connection with a simple query
		var count int64
		if err := db.Raw("SELECT 1").Count(&count).Error; err != nil {
			logger.Error("database connection test failed", "error", err)
		} else if debugMode {
			logger.Debug("database connection test OK")
		}
	}

	// Initialize mapping manager if database is available
	var mappingMgr *mapping.Manager
	if db != nil {
		// Determine preferred provider (use first anime provider)
		preferredProvider := ""
		if animeProvider, exists := providerMap[providers.MediaTypeAnime]; exists {
			preferredProvider = animeProvider.Name()
		}
		mappingMgr = mapping.NewManagerWithDebug(db, preferredProvider, debugMode, logger)
		if debugMode {
			logger.Debug("mapping manager initialized", "preferred_provider", preferredProvider)
		}
	}

	if mappingMgr == nil {
		logger.Error("mapping manager is nil, provider mappings will not persist")
	}

	// Initialize download manager if config and database are available
	var downloadMgr *downloader.Manager
	var downloadsComp downloads.Model
	if cfg != nil && db != nil {
		if appCfg, ok := cfg.(*config.Config); ok {
			// Create download manager
			dlMgr, err := downloader.NewManager(db, &appCfg.Downloads, logger)
			if err != nil {
				logger.Warn("failed to initialize download manager", "error", err)
			} else {
				downloadMgr = dlMgr
				downloadsComp = downloads.New(dlMgr)

				// Start the download manager
				if err := dlMgr.Start(context.Background()); err != nil {
					logger.Warn("failed to start download manager", "error", err)
				} else if debugMode {
					logger.Debug("download manager initialized successfully")
				}
			}
		}
	}

	homeModel := home.New(db)
	historyModel := history.New(db)
	var historyService *historyservice.Service
	if db != nil {
		historyService = historyservice.NewService(db)
	}

	var appConfig *config.Config
	if cfg != nil {
		if c, ok := cfg.(*config.Config); ok {
			appConfig = c
		}
	}

	// Initialize clipboard service
	clipboardSvc := clipboard.NewService(logger)

	// Determine default media type from config
	defaultMediaType := providers.MediaTypeMovieTV // Default to Movies/TV
	if appConfig.UI.DefaultMediaType != "" {
		switch appConfig.UI.DefaultMediaType {
		case "anime":
			defaultMediaType = providers.MediaTypeAnime
		case "manga":
			defaultMediaType = providers.MediaTypeManga
		case "movie_tv", "movies", "movies_tv":
			defaultMediaType = providers.MediaTypeMovieTV
		}
	}

	app := &App{
		state:                   homeView,
		previousState:           -1, // Initialize to invalid state
		providers:               providerMap,
		currentMediaType:        defaultMediaType,
		home:                    &homeModel,
		search:                  search.New(),
		results:                 results.New(),
		seasons:                 seasons.New(),
		episodeListModel:        results.New(),
		episodesComponent:       episodes.New(),
		anilistComponent:        anilist.New(),
		downloadsComponent:      downloadsComp,
		historyComponent:        historyModel,
		mangaComponent:          manga.New(appConfig, db),
		mangaInfoComponent:      mangainfo.New(nil),
		mangaDownloadComponent:  mangadownload.New(),
		providerStatusComponent: providerstatus.New(),
		historyService:          historyService,
		helpComponent:           help.New(),
		spinner:                 s,
		player:                  mpvPlayer,
		detailsSem:              make(chan struct{}, 5), // Limit to 5 concurrent fetches
		searchQueries:           make(map[providers.MediaType]string),
		inDebugLinksMode:        false,
		dialogMode:              anilist.DialogNone,
		dialogState:             anilist.InitDialogState(),
		db:                      db,
		mappingMgr:              mappingMgr,
		downloadMgr:             downloadMgr,
		cfg:                     cfg,
		logger:                  logger,
		clipboardSvc:            clipboardSvc,
		msgChan:                 make(chan tea.Msg, 100),
		audioPreference:         audioPreference,
	}

	// Set parent for manga info component
	app.mangaInfoComponent.SetParent(app)

	// Set initial provider name and media type for home component filtering
	app.home.CurrentMediaType = app.currentMediaType
	if provider, ok := providerMap[app.currentMediaType]; ok {
		app.home.SetProvider(provider.Name())
		app.helpComponent.SetProviderName(provider.Name())
	}

	// Set up download manager callbacks if available
	if app.downloadMgr != nil {
		// Callback for progress updates
		app.downloadMgr.OnProgressUpdate(func(task downloader.DownloadTask) {
			// Progress updates are frequent, just log at debug level
			// The downloads view will poll for updates
			app.debugLog("Download progress: %s - %.1f%%", task.ID, task.Progress)
		})

		// Callback for download completion
		app.downloadMgr.OnDownloadComplete(func(task downloader.DownloadTask) {
			app.logger.Info("download completed", "media_title", task.MediaTitle)
		})

		// Callback for download errors
		app.downloadMgr.OnDownloadError(func(task downloader.DownloadTask, err error) {
			app.logger.Error("download failed", "media_title", task.MediaTitle, "error", err)
		})
	}

	return app
}

// updateProvider updates the provider map and UI components with the new provider
func (a *App) updateProvider(p providers.Provider) {
	a.debugLog("updateProvider: Switching to %s (Type: %s)", p.Name(), p.Type())

	// Update the primary type
	a.providers[p.Type()] = p

	// Handle composite types - ensure all related keys are updated
	if p.Type() == providers.MediaTypeMovieTV {
		a.providers[providers.MediaTypeMovie] = p
		a.providers[providers.MediaTypeTV] = p
		a.providers[providers.MediaTypeMovieTV] = p
	} else if p.Type() == providers.MediaTypeAll || p.Type() == providers.MediaTypeAnimeMovieTV {
		a.providers[providers.MediaTypeAnime] = p
		a.providers[providers.MediaTypeMovie] = p
		a.providers[providers.MediaTypeTV] = p
		a.providers[providers.MediaTypeMovieTV] = p
	}

	// Also update based on current app state if needed
	// If we are in a specific mode (e.g. Movie), and we selected a provider that supports it,
	// ensure the current mode's key is updated even if the provider's primary type is different.
	// e.g. Provider is MovieTV, current mode is Movie.
	if a.currentMediaType == providers.MediaTypeMovie && (p.Type() == providers.MediaTypeMovieTV || p.Type() == providers.MediaTypeAll || p.Type() == providers.MediaTypeAnimeMovieTV) {
		a.providers[providers.MediaTypeMovie] = p
	} else if a.currentMediaType == providers.MediaTypeTV && (p.Type() == providers.MediaTypeMovieTV || p.Type() == providers.MediaTypeAll || p.Type() == providers.MediaTypeAnimeMovieTV) {
		a.providers[providers.MediaTypeTV] = p
	} else if a.currentMediaType == providers.MediaTypeMovieTV && (p.Type() == providers.MediaTypeMovie || p.Type() == providers.MediaTypeTV) {
		// This case is rare (provider is narrower than current mode), but we should update the composite key
		a.providers[providers.MediaTypeMovieTV] = p
	}

	// Update UI components
	a.providerName = p.Name()
	a.home.SetProvider(p.Name())
	a.results.SetProviderName(p.Name())
	a.episodeListModel.SetProviderName(p.Name())
	a.helpComponent.SetProviderName(p.Name())

	a.debugLog("updateProvider: UI components updated with %s", p.Name())
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(
		a.home.Init(),
		a.listenForMessages(),
	)
}

// listenForMessages listens for messages from background goroutines
func (a *App) listenForMessages() tea.Cmd {
	return func() tea.Msg {
		return <-a.msgChan
	}
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Pass window size messages to all components
		// IMPORTANT: Must assign updated models back to preserve width/height
		var homeCmd, searchCmd, resultsCmd, seasonsCmd, episodesCmd, anilistCmd, downloadsCmd, mangaCmd tea.Cmd
		var homeModel, searchModel, resultsModel, seasonsModel, episodesModel, mangaModel tea.Model

		homeModel, homeCmd = a.home.Update(msg)
		a.home = homeModel.(*home.Model)

		searchModel, searchCmd = a.search.Update(msg)
		a.search = searchModel.(search.Model)

		resultsModel, resultsCmd = a.results.Update(msg)
		a.results = resultsModel.(results.Model)

		seasonsModel, seasonsCmd = a.seasons.Update(msg)
		a.seasons = seasonsModel.(seasons.Model)

		episodesModel, episodesCmd = a.episodesComponent.Update(msg)
		a.episodesComponent = episodesModel.(episodes.Model)

		mangaModel, mangaCmd = a.mangaComponent.Update(msg)
		a.mangaComponent = mangaModel.(manga.Model)

		var anilistModel tea.Model
		anilistModel, anilistCmd = a.anilistComponent.Update(msg)
		a.anilistComponent = anilistModel.(anilist.Model)

		var downloadsModel tea.Model
		downloadsModel, downloadsCmd = a.downloadsComponent.Update(msg)
		a.downloadsComponent = downloadsModel.(downloads.Model)

		var historyModel tea.Model
		var historyCmd tea.Cmd
		historyModel, historyCmd = a.historyComponent.Update(msg)
		a.historyComponent = historyModel.(history.Model)

		var providerStatusModel tea.Model
		var providerStatusCmd tea.Cmd
		providerStatusModel, providerStatusCmd = a.providerStatusComponent.Update(msg)
		a.providerStatusComponent = providerStatusModel.(providerstatus.Model)

		// Update help component with window size
		var helpCmd tea.Cmd
		a.helpComponent, helpCmd = a.helpComponent.Update(msg)

		// Update provider name in help
		if p, ok := a.providers[a.currentMediaType]; ok {
			a.helpComponent.SetProviderName(p.Name())
		}

		// Also update episodeListModel which is used for episode selection view
		episodeListModel, _ := a.episodeListModel.Update(msg)
		a.episodeListModel = episodeListModel.(results.Model)
		a.episodeListModel.SetProviderName(a.providerName)

		// Also update providerSelectionResult if active
		if a.state == providerSelectionView {
			var providerSelectionModel tea.Model
			var providerSelectionCmd tea.Cmd
			providerSelectionModel, providerSelectionCmd = a.providerSelectionResult.Update(msg)
			a.providerSelectionResult = providerSelectionModel.(results.Model)
			providerStatusCmd = tea.Batch(providerStatusCmd, providerSelectionCmd)
		}

		var mangaInfoModel tea.Model
		var mangaInfoCmd tea.Cmd
		mangaInfoModel, mangaInfoCmd = a.mangaInfoComponent.Update(msg)
		a.mangaInfoComponent = mangaInfoModel.(*mangainfo.Model)

		return a, tea.Batch(homeCmd, searchCmd, resultsCmd, seasonsCmd, episodesCmd, anilistCmd, downloadsCmd, helpCmd, mangaCmd, historyCmd, mangaInfoCmd, providerStatusCmd)
	case tea.KeyMsg:
		return a.handleKeyMsg(msg)
	// Custom messages
	case common.MangaInfoMsg:
		return a.handleMangaInfoMsg(msg)

	case common.MangaInfoResultMsg:
		return a.handleMangaInfoResultMsg(msg)

	case common.GoToSearchMsg:
		return a.handleGoToSearchMsg()
	case common.GoToHomeMsg:
		return a.handleGoToHomeMsg()
	case common.BackMsg:
		return a.handleBackMsg()
	case common.ToggleProviderMsg:
		return a.handleToggleProviderMsg()
	case common.GoToAniListMsg:
		return a.handleGoToAniListMsg()
	case common.GoToDownloadsMsg:
		return a.handleGoToDownloadsMsg()
	case common.GoToProviderStatusMsg:
		return a.handleGoToProviderStatusMsg()
	case common.DownloadsTickMsg:
		return a.handleDownloadsTickMsg(msg)
	case common.GoToHistoryMsg:
		return a.handleGoToHistoryMsg(msg)
	case anilist.LibraryLoadedMsg:
		return a.handleLibraryLoadedMsg(msg)
	case anilist.BackMsg:
		return a.handleAnilistBackMsg()
	case common.RefreshHistoryMsg:
		return a.handleRefreshHistoryMsg(msg)
	case anilist.SearchNewAnimeMsg:
		return a.handleSearchNewAnimeMsg(msg)

	case anilist.AniListSearchRequestedMsg:
		return a.handleAniListSearchRequestedMsg(msg)

	case anilist.AniListSearchResultMsg:
		return a.handleAniListSearchResultMsg(msg)

	case anilist.AniListAddToListDialogOpenMsg:
		return a.handleAniListAddToListDialogOpenMsg(msg)

	case anilist.AniListAddToListMsg:
		return a.handleAniListAddToListMsg(msg)

	case anilist.AniListDeleteConfirmationMsg:
		return a.handleAniListDeleteConfirmationMsg(msg)

	case anilist.AniListSearchSelectMsg:
		return a.handleAniListSearchSelectMsg(msg)

	case anilist.AniListDeleteRequestedMsg:
		return a.handleAniListDeleteRequestedMsg(msg)

	case anilist.AniListDeleteResultMsg:
		return a.handleAniListDeleteResultMsg(msg)

	case anilist.SelectMediaMsg:
		return a.handleSelectMediaMsg(msg)

	case anilist.RefreshLibraryMsg:
		return a.handleRefreshLibraryMsg(msg)

	case anilist.OpenStatusUpdateMsg:
		return a.handleOpenStatusUpdateMsg(msg)

	case anilist.OpenScoreUpdateMsg:
		return a.handleOpenScoreUpdateMsg(msg)

	case anilist.OpenProgressUpdateMsg:
		return a.handleOpenProgressUpdateMsg(msg)

	case anilist.StatusUpdatedMsg:
		return a.handleStatusUpdatedMsg(msg)

	case anilist.ScoreUpdatedMsg:
		return a.handleScoreUpdatedMsg(msg)

	case anilist.ProgressUpdatedMsg:
		return a.handleProgressUpdatedMsg(msg)

	case anilist.ProviderSearchResultMsg:
		return a.handleProviderSearchResultMsg(msg)

	case anilist.RemapRequestedMsg:
		return a.handleRemapRequestedMsg(msg)

	case common.PerformSearchMsg:
		return a.handlePerformSearchMsg(msg)

	case common.SearchProviderMsg:
		return a.handleSearchProviderMsg(msg)

	case common.SearchResultsMsg:
		return a.handleSearchResultsMsg(msg)

	case common.GenerateDebugInfoMsg:
		return a.handleGenerateDebugInfoMsg(msg)

	case common.GenerateMediaDebugInfoMsg:
		return a.handleGenerateMediaDebugInfoMsg(msg)

	case common.DebugSourcesLoadedMsg:
		return a.handleDebugSourcesLoadedMsg(msg)

	case common.RequestDetailsMsg:
		return a.handleRequestDetailsMsg(msg)

	case common.DetailsLoadedMsg:
		return a.handleDetailsLoadedMsg(msg)

	case common.MediaSelectedMsg:
		return a.handleMediaSelectedMsg(msg)

	case common.SeasonsLoadedMsg:
		return a.handleSeasonsLoadedMsg(msg)

	case common.SeasonSelectedMsg:
		return a.handleSeasonSelectedMsg(msg)

	case common.EpisodesLoadedMsg:
		return a.handleEpisodesLoadedMsg(msg)

	case common.MediaDownloadMsg:
		return a.handleMediaDownloadMsg(msg)

	case common.EpisodeDownloadMsg:
		return a.handleEpisodeDownloadMsg(msg)

	case common.BatchDownloadMsg:
		return a.handleBatchDownloadMsg(msg)

	case common.DownloadAddedMsg:
		return a.handleDownloadAddedMsg(msg)

	case clearStatusMsg:
		return a.handleClearStatusMsg(msg)

	case dismissDownloadNotificationMsg:
		return a.handleDismissDownloadNotificationMsg(msg)

	case common.WatchPartyMsg:
		return a.handleWatchPartyMsg(msg)

	case common.SetWatchPartyProxyMsg:
		return a.handleSetWatchPartyProxyMsg(msg)

	case common.OpenWatchPartyMsg:
		return a.handleOpenWatchPartyMsg(msg)

	case common.GenerateWatchPartyMsg:
		return a.handleGenerateWatchPartyMsg(msg)

	case common.ShareViaWatchPartyMsg:
		return a.handleShareViaWatchPartyMsg(msg)

	case common.ShareHistoryViaWatchPartyMsg:
		return a.handleShareHistoryViaWatchPartyMsg(msg)

	case common.ShareRecentViaWatchPartyMsg:
		return a.handleShareRecentViaWatchPartyMsg(msg)

	case common.ShareMediaViaWatchPartyMsg:
		return a.handleShareMediaViaWatchPartyMsg(msg)

	case common.NextChapterMsg:
		return a.handleNextChapterMsg(msg)

	case common.EpisodeSelectedMsg:
		return a.handleEpisodeSelectedMsg(msg)

	case common.MangaPagesLoadedMsg:
		return a.handleMangaPagesLoadedMsg(msg)

	case common.ChapterCompletedMsg:
		return a.handleChapterCompletedMsg(msg)

	case common.MangaQuitMsg:
		return a.handleMangaQuitMsg(msg)

	case common.ResumePlaybackMsg:
		return a.handleResumePlaybackMsg(msg)

	case common.PlaybackStartingMsg:
		return a.handlePlaybackStartingMsg(msg)

	case common.PlayerLaunchingMsg:
		return a.handlePlayerLaunchingMsg(msg)

	case common.PlayerLaunchTimeoutCheckMsg:
		return a.handlePlayerLaunchTimeoutCheckMsg(msg)

	case common.PlaybackStartedMsg:
		return a.handlePlaybackStartedMsg(msg)

	case common.PlaybackEndedMsg:
		return a.handlePlaybackEndedMsg(msg)

	case common.PlaybackTickMsg:
		return a, a.handlePlaybackTickMsg()

	case common.PlaybackProgressMsg:
		return a, a.handlePlaybackProgressMsg(msg)

	case common.PlaybackErrorMsg:
		return a.handlePlaybackErrorMsg(msg)

	case common.PlaybackAutoReturnMsg:
		return a.handlePlaybackAutoReturnMsg(msg)

	case common.ShowAudioSelectorMsg:
		// Show audio selector when no matching track found
		selector := audioselect.New(msg.Tracks, msg.AniListID)
		a.audioSelectorModel = &selector
		a.pendingStream = msg.Stream
		// Store episode context for playback resumption
		a.currentEpisodeID = msg.EpisodeID
		a.currentEpisodeNumber = msg.EpisodeNum
		a.currentEpisodeTitle = msg.EpisodeTitle
		a.previousState = a.state
		a.state = audioSelectView
		return a, nil

	case audioselect.SelectionMsg:
		// User selected an audio track
		// Save preference to database
		if msg.AniListID > 0 {
			trackIndexPtr := &msg.Track.Index
			err := database.SaveAudioPreference(a.db, msg.AniListID, msg.Track.Type, trackIndexPtr)
			if err != nil {
				a.logger.Error("failed to save audio preference", "error", err, "anilist_id", msg.AniListID, "type", msg.Track.Type)
				// Non-blocking - playback continues even if save fails
			} else {
				a.logger.Debug("saved audio preference", "anilist_id", msg.AniListID, "type", msg.Track.Type)
			}
		}
		// Resume playback with selected track
		a.state = launchingPlayerView
		return a, a.continuePlaybackWithAudioTrack(msg.Track.Index)

	case audioselect.CancelMsg:
		// User canceled audio selection - return to previous view
		a.state = a.previousState
		a.selectedAudioTrack = nil
		a.pendingStream = nil
		return a, nil
	}

	// Component-specific updates
	switch a.state {
	case providerStatusView:
		var newModel tea.Model
		newModel, cmd = a.providerStatusComponent.Update(msg)
		a.providerStatusComponent = newModel.(providerstatus.Model)
		return a, cmd
	case homeView:
		var newModel tea.Model
		newModel, cmd = a.home.Update(msg)
		a.home = newModel.(*home.Model)
		cmds = append(cmds, cmd)
	case searchView:
		var newModel tea.Model
		newModel, cmd = a.search.Update(msg)
		a.search = newModel.(search.Model)
		cmds = append(cmds, cmd)
	case resultsView:
		var newModel tea.Model
		newModel, cmd = a.results.Update(msg)
		a.results = newModel.(results.Model)
		cmds = append(cmds, cmd)
	case providerSelectionView:
		var newModel tea.Model
		newModel, cmd = a.providerSelectionResult.Update(msg)
		a.providerSelectionResult = newModel.(results.Model)
		cmds = append(cmds, cmd)
	case seasonView:
		var newModel tea.Model
		newModel, cmd = a.seasons.Update(msg)
		a.seasons = newModel.(seasons.Model)
		cmds = append(cmds, cmd)
	case episodeView:
		// Handle episode selection using new episodes component
		var newModel tea.Model
		newModel, cmd = a.episodesComponent.Update(msg)
		a.episodesComponent = newModel.(episodes.Model)
		cmds = append(cmds, cmd)
	case anilistView:
		var anilistModel tea.Model
		anilistModel, cmd = a.anilistComponent.Update(msg)
		a.anilistComponent = anilistModel.(anilist.Model)
		cmds = append(cmds, cmd)
	case downloadsView:
		var downloadsModel tea.Model
		downloadsModel, cmd = a.downloadsComponent.Update(msg)
		a.downloadsComponent = downloadsModel.(downloads.Model)
		cmds = append(cmds, cmd)
	case historyView:
		var historyModel tea.Model
		historyModel, cmd = a.historyComponent.Update(msg)
		a.historyComponent = historyModel.(history.Model)
		cmds = append(cmds, cmd)
	case mangaReaderView:
		var newModel tea.Model
		newModel, cmd = a.mangaComponent.Update(msg)
		a.mangaComponent = newModel.(manga.Model)
		cmds = append(cmds, cmd)
	case mangaInfoView:
		var newModel tea.Model
		newModel, cmd = a.mangaInfoComponent.Update(msg)
		a.mangaInfoComponent = newModel.(*mangainfo.Model)
		cmds = append(cmds, cmd)
	case mangaDownloadProgressView:
		var newModel tea.Model
		newModel, cmd = a.mangaDownloadComponent.Update(msg)
		a.mangaDownloadComponent = newModel.(mangadownload.Model)
		cmds = append(cmds, cmd)

		// Handle completion message
		if _, ok := msg.(mangadownload.DownloadCompleteAckMsg); ok {
			a.state = episodeView
		}

		// Keep listening for messages from background downloads
		if _, ok := msg.(mangadownload.ProgressUpdateMsg); ok {
			cmds = append(cmds, a.listenForMessages())
		}
		if _, ok := msg.(mangadownload.ChapterCompleteMsg); ok {
			cmds = append(cmds, a.listenForMessages())
		}
		if _, ok := msg.(mangadownload.ChapterFailedMsg); ok {
			cmds = append(cmds, a.listenForMessages())
		}
	case audioSelectView:
		if a.audioSelectorModel != nil {
			updatedModel, cmd := a.audioSelectorModel.Update(msg)
			a.audioSelectorModel = &updatedModel
			cmds = append(cmds, cmd)
		}
	case loadingView, launchingPlayerView:
		a.spinner, cmd = a.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Check if force quit is requested
	if a.forceQuit {
		return a, tea.Quit
	}

	return a, tea.Batch(cmds...)
}

func (a *App) View() string {
	var finalView string

	baseView := a.renderView()
	finalView = baseView

	// Render popups if visible (before status message so status appears on top)
	if a.showWatchPartyPopup {
		popupView := a.renderWatchPartyPopup()
		// Place popup centered over the base view
		finalView = lipgloss.Place(
			lipgloss.Width(finalView),
			lipgloss.Height(finalView),
			lipgloss.Center,
			lipgloss.Center,
			popupView,
			lipgloss.WithWhitespaceBackground(styles.OxocarbonBlack),
			lipgloss.WithWhitespaceForeground(styles.OxocarbonBlack),
		)
	}

	// Render Debug popup if visible
	if a.showDebugPopup {
		popupView := a.renderDebugPopup()
		// Place popup centered over the base view
		finalView = lipgloss.Place(
			lipgloss.Width(finalView),
			lipgloss.Height(finalView),
			lipgloss.Center,
			lipgloss.Center,
			popupView,
			lipgloss.WithWhitespaceBackground(styles.OxocarbonBlack),
			lipgloss.WithWhitespaceForeground(styles.OxocarbonBlack),
		)
	}

	// Render download notification popup if visible
	if a.showDownloadNotification {
		notificationView := a.renderDownloadNotification()
		// Overlay at top-right
		finalView = lipgloss.Place(
			a.width,
			a.height,
			lipgloss.Right,
			lipgloss.Top,
			notificationView,
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceForeground(styles.OxocarbonBase00),
		)
	}

	// Add INPUT MODE indicator to status bar if any component has active input
	inputModeActive := false
	switch a.state {
	case resultsView:
		inputModeActive = a.results.IsInputActive()
	case episodeView:
		inputModeActive = a.episodesComponent.IsInputActive()
	case seasonView:
		inputModeActive = a.seasons.IsInputActive()
	case historyView:
		inputModeActive = a.historyComponent.IsInputActive()
	}

	if inputModeActive {
		// Override status message with input mode indicator
		width := a.width
		if width == 0 {
			width = 80
		}

		inputModeStyle := styles.FooterStyle.
			Width(width).
			Background(styles.OxocarbonPurple).
			Foreground(styles.OxocarbonBase00).
			Bold(true).
			Align(lipgloss.Center)

		finalView = strings.TrimRight(finalView, "\n")

		// Truncate content if needed to fit input mode indicator
		if a.height > 0 {
			lines := strings.Split(finalView, "\n")
			if len(lines) >= a.height {
				if a.height > 1 {
					lines = lines[:a.height-1]
					finalView = strings.Join(lines, "\n")
				}
			}
		}

		finalView += "\n" + inputModeStyle.Render("⌨ INPUT MODE")
	} else if a.statusMsg != "" && a.state != mangaReaderView {
		// Add status message overlay if present (render AFTER popups so it appears on top)
		var statusColor lipgloss.Color
		var icon string

		if strings.HasPrefix(a.statusMsg, "📋") {
			statusColor = styles.OxocarbonBlue
			icon = "📋"
		} else if strings.HasPrefix(a.statusMsg, "✓") {
			statusColor = styles.OxocarbonGreen
			icon = "✓"
		} else if strings.HasPrefix(a.statusMsg, "✗") {
			statusColor = styles.OxocarbonPink
			icon = "✗"
		} else if strings.HasPrefix(a.statusMsg, "⚠") {
			statusColor = styles.OxocarbonTeal
			icon = "⚠"
		} else {
			statusColor = styles.OxocarbonCyan
			icon = "ℹ"
		}

		// Clean message (remove icon if present to avoid double icon)
		cleanMsg := strings.TrimPrefix(a.statusMsg, icon+" ")
		cleanMsg = strings.TrimSpace(cleanMsg)

		// Render full width footer
		width := a.width
		if width == 0 {
			width = 80 // Fallback
		}

		// Use a bar style at the bottom
		statusStyle := styles.FooterStyle.
			Width(width). // Full width
			Background(statusColor).
			Foreground(styles.OxocarbonBase00).
			Bold(true).
			Align(lipgloss.Left)

		// Overlay at the bottom of the screen
		// We replace the last line of the base view to avoid scrolling/shifting
		// This ensures the footer remains visible after the status message clears

		// Remove trailing newlines
		finalView = strings.TrimRight(finalView, "\n")

		// If we have height information, ensure we don't exceed it
		if a.height > 0 {
			lines := strings.Split(finalView, "\n")
			if len(lines) >= a.height {
				// For home view, ALWAYS preserve the header (first 2 lines)
				if a.state == homeView {
					// Keep header + as much content as fits + space for status
					if a.height > 3 {
						headerLines := 2
						keepLines := a.height - 1 // -1 for status line
						if keepLines > headerLines {
							// Keep header + truncate content to fit
							contentLines := lines[headerLines:]
							contentToKeep := keepLines - headerLines
							if len(contentLines) > contentToKeep {
								// Keep header + only the content that fits
								lines = append(lines[:headerLines], contentLines[:contentToKeep]...)
							}
						} else {
							// Terminal too small, just show header
							lines = lines[:headerLines]
						}
						finalView = strings.Join(lines, "\n")
					}
				} else {
					// For non-home views, normal truncation
					if a.height > 1 {
						lines = lines[:a.height-1]
						finalView = strings.Join(lines, "\n")
					}
				}
			}
		}

		finalView += "\n" + statusStyle.Render(fmt.Sprintf("%s %s", icon, cleanMsg))
	}

	// Render help overlay on top if visible (render AFTER status so it appears above everything)
	if a.helpComponent.IsVisible() {
		helpView := a.helpComponent.View()
		// Place help overlay centered over the entire terminal
		return lipgloss.Place(
			a.width,
			a.height,
			lipgloss.Center,
			lipgloss.Center,
			helpView,
			lipgloss.WithWhitespaceBackground(styles.OxocarbonBlack),
			lipgloss.WithWhitespaceForeground(styles.OxocarbonBlack),
		)
	}

	return finalView
}

func (a *App) renderHeader() string {
	header := styles.TitleStyle.Render("  greg  ")

	mode := "ANIME"
	modeColor := styles.OxocarbonPurple
	switch a.currentMediaType {
	case providers.MediaTypeMovieTV:
		mode = "MOVIES/TV"
		modeColor = styles.OxocarbonBlue
	case providers.MediaTypeManga:
		mode = "MANGA"
		modeColor = styles.OxocarbonPink
	}

	modeBadge := lipgloss.NewStyle().
		Foreground(styles.OxocarbonBase00).
		Background(modeColor).
		Padding(0, 1).
		Bold(true).
		Render(mode)

	providerName := a.providerName
	// Don't render header if provider name isn't loaded yet
	if providerName == "" {
		return ""
	}
	providerBadge := lipgloss.NewStyle().
		Foreground(styles.OxocarbonBase05).
		Background(styles.OxocarbonBase02).
		Padding(0, 1).
		Render(providerName)

	return lipgloss.JoinHorizontal(lipgloss.Center, header, "  ", modeBadge, " ", providerBadge) + "\n\n"
}

func (a *App) renderView() string {
	switch a.state {
	case errorView:
		errorMsg := "An error occurred:\n\n"
		errorMsg += a.err.Error()
		errorMsg += "\n\n"
		errorMsg += styles.HelpStyle.Render("Press 'esc' to return.")
		return styles.AppStyle.Render(errorMsg)
	case loadingView:
		var loadingMsg string
		switch a.loadingOp {
		case loadingSearch:
			loadingMsg = "Searching..."
		case loadingSeasons:
			if a.selectedMedia.Type == providers.MediaTypeManga {
				loadingMsg = "Loading volumes..."
			} else {
				loadingMsg = "Loading seasons..."
			}
		case loadingEpisodes:
			if a.selectedMedia.Type == providers.MediaTypeManga || a.currentMediaType == providers.MediaTypeManga {
				loadingMsg = "Loading chapters..."
			} else {
				loadingMsg = "Loading episodes..."
			}
		case loadingStream:
			loadingMsg = "Loading media..."
		case loadingAniListLibrary:
			loadingMsg = "Fetching your Anilist library..."
		case loadingProviderSearch:
			loadingMsg = "Searching providers for anime..."
		case loadingMangaPages:
			loadingMsg = "Loading manga pages..."
		default:
			loadingMsg = "Loading..."
		}
		return fmt.Sprintf("\n\n   %s %s\n\n", a.spinner.View(), loadingMsg)
	case launchingPlayerView:
		// Show launching state with spinner and timeout info
		elapsed := time.Since(a.launchStartTime)
		remaining := 15*time.Second - elapsed

		var launchMsg string
		if a.currentEpisodeNumber == 0 { // For movies
			launchMsg += fmt.Sprintf("Launching player: %s\n\n", a.selectedMedia.Title)
		} else { // For episodes
			launchMsg += fmt.Sprintf("Launching player: %s - Episode %d\n\n",
				a.selectedMedia.Title, a.currentEpisodeNumber)
		}

		launchMsg += fmt.Sprintf("%s Starting mpv player...\n\n", a.spinner.View())
		launchMsg += fmt.Sprintf("Timeout in: %.1fs\n\n", remaining.Seconds())
		launchMsg += styles.HelpStyle.Render("Press 'esc' to cancel, 'q' or Ctrl+C to quit application")

		return styles.AppStyle.Render(launchMsg)
	case playingView:
		// Show background indicator that playback is active
		var playingMsg string
		if a.currentEpisodeNumber == 0 { // For movies and single-episode content
			playingMsg += fmt.Sprintf("▶ Playing: %s", a.selectedMedia.Title)
		} else { // For multi-episode series
			playingMsg += fmt.Sprintf("▶ Playing: %s - Episode %d", a.selectedMedia.Title, a.currentEpisodeNumber)
			// Only add episode title if it's different from media title and not generic
			if a.currentEpisodeTitle != "" &&
				a.currentEpisodeTitle != "Movie" &&
				a.currentEpisodeTitle != a.selectedMedia.Title {
				playingMsg += fmt.Sprintf(": %s", a.currentEpisodeTitle)
			}
		}
		playingMsg += "\n\n"
		playingMsg += "Playback is running in mpv player.\n"
		playingMsg += "UI will return automatically when playback ends.\n\n"
		playingMsg += "Press 'q' or Ctrl+C to quit application."
		return styles.AppStyle.Render(playingMsg)
	case playbackCompletedView:
		// Show playback completion message
		if a.showCompletionDialog {
			// Render the completion confirmation modal
			modalContent := a.completionDialogMsg
			dialogView := styles.PopupStyle.Render(modalContent)

			// Center the dialog overlay using terminal dimensions
			return lipgloss.Place(
				a.width,
				a.height,
				lipgloss.Center,
				lipgloss.Center,
				dialogView,
				lipgloss.WithWhitespaceChars(" "),
				lipgloss.WithWhitespaceForeground(lipgloss.Color("0")),
			)
		}
		return styles.AppStyle.Render(a.playbackCompletionMsg)
	case searchView:
		return a.search.View()
	case resultsView:
		return a.results.View()
	case providerSelectionView:
		// Show provider selection with a header
		var title string
		isGlobalSwitch := false

		if a.currentAniListMedia != nil {
			title = a.currentAniListMedia.Title
		} else if a.selectedMedia.Title != "" {
			title = a.selectedMedia.Title
		} else if a.search.GetValue() != "" {
			title = a.search.GetValue()
		} else {
			// If no specific title, we are switching global default
			isGlobalSwitch = true
			title = "Global Default"
		}

		headerText := "\nSelect provider for: " + title + "\n"
		if isGlobalSwitch {
			headerText = "\nSelect Default Provider\n"
		}

		// Add current provider info
		currentProvider := "Unknown"
		if p, ok := a.providers[a.currentMediaType]; ok {
			currentProvider = p.Name()
		}
		headerText += fmt.Sprintf("Current: %s\n\n", currentProvider)

		header := lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.OxocarbonPurple).
			Render(headerText)

		var baseView string
		// If info dialog is open, don't show header to avoid layout issues
		if a.providerSelectionResult.IsInfoOpen() {
			baseView = a.providerSelectionResult.View()
		} else {
			baseView = header + a.providerSelectionResult.View()
			// Add hint for provider selection actions
			baseView += "\n" + styles.AniListHelpStyle.Render("  Enter: Just Once • Ctrl+S: Set Default")
		}

		return baseView
	case seasonView:
		return a.seasons.View()
	case episodeView:
		return a.episodesComponent.View()
	case anilistView:
		baseView := a.anilistComponent.View()

		// Overlay dialog if one is open
		if a.dialogMode != anilist.DialogNone {
			var dialogView string
			switch a.dialogMode {
			case anilist.DialogStatus:
				selectedMedia := a.anilistComponent.GetSelectedMedia()
				if selectedMedia != nil {
					dialogView = anilist.RenderStatusDialog(string(selectedMedia.Status), a.dialogState.StatusIndex, string(selectedMedia.Type))
				}
			case anilist.DialogScore:
				selectedMedia := a.anilistComponent.GetSelectedMedia()
				if selectedMedia != nil {
					dialogView = a.anilistComponent.RenderScoreDialog(selectedMedia.Score, a.dialogState.ScoreInput)
				}
			case anilist.DialogProgress:
				selectedMedia := a.anilistComponent.GetSelectedMedia()
				if selectedMedia != nil {
					dialogView = a.anilistComponent.RenderProgressDialog(selectedMedia.Progress, selectedMedia.TotalEpisodes, a.dialogState.ProgressInput)
				}
			case anilist.DialogAddToList:
				if a.currentAniListMedia != nil {
					// Using the same order as in dialogs.go statusOptions to keep index consistent
					statusOptions := []string{"CURRENT", "COMPLETED", "PAUSED", "DROPPED", "PLANNING", "REPEATING"}
					dialogView = anilist.RenderAddToListDialog(a.currentAniListMedia.Title, statusOptions, a.dialogState.StatusIndex, string(a.currentAniListMedia.Type))
				}
			case anilist.DialogDelete:
				if a.currentAniListMedia != nil {
					dialogView = anilist.RenderDeleteConfirmationDialog(a.currentAniListMedia.Title)
				}
			}

			if dialogView != "" {
				// Center the dialog overlay using terminal dimensions
				return lipgloss.Place(
					a.width,
					a.height,
					lipgloss.Center,
					lipgloss.Center,
					dialogView,
					lipgloss.WithWhitespaceChars(" "),
					lipgloss.WithWhitespaceForeground(lipgloss.Color("0")),
				)
			}
		}

		return baseView
	case downloadsView:
		return a.downloadsComponent.View()
	case historyView:
		return a.historyComponent.View()
	case providerStatusView:
		return a.providerStatusComponent.View()
	case mangaReaderView:
		return a.mangaComponent.View()
	case mangaInfoView:
		return a.mangaInfoComponent.View()
	case mangaDownloadProgressView:
		return a.mangaDownloadComponent.View()
	case audioSelectView:
		if a.audioSelectorModel != nil {
			return a.audioSelectorModel.View()
		}
		return "Audio selector not initialized"
	default:
		return a.home.View()
	}
}

// updateHelpContext updates the help component's context based on the current state
func (a *App) updateHelpContext() {
	switch a.state {
	case homeView:
		a.helpComponent.SetContext(help.HomeContext)
	case searchView:
		a.helpComponent.SetContext(help.SearchContext)
	case resultsView:
		a.helpComponent.SetContext(help.ResultsContext)
	case seasonView:
		a.helpComponent.SetContext(help.SeasonsContext)
	case episodeView:
		a.helpComponent.SetContext(help.EpisodesContext)
	case anilistView:
		a.helpComponent.SetContext(help.AniListContext)
	case downloadsView:
		a.helpComponent.SetContext(help.DownloadsContext)
	case historyView:
		a.helpComponent.SetContext(help.HistoryContext)
	case providerStatusView:
		a.helpComponent.SetContext(help.ProviderStatusContext)
	default:
		a.helpComponent.SetContext(help.GlobalContext)
	}
}

// getMangaPages fetches pages for a manga chapter
func (a *App) getMangaPages(chapterID string) tea.Cmd {
	return func() tea.Msg {
		provider, ok := a.providers[providers.MediaTypeManga]
		if !ok {
			return common.MangaPagesLoadedMsg{
				Err: fmt.Errorf("no manga provider available"),
			}
		}

		mangaProvider, ok := provider.(providers.MangaProvider)
		if !ok {
			return common.MangaPagesLoadedMsg{
				Err: fmt.Errorf("provider does not support manga pages"),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pages, err := mangaProvider.GetMangaPages(ctx, chapterID)
		if err != nil {
			return common.MangaPagesLoadedMsg{
				Err: fmt.Errorf("failed to get manga pages: %w", err),
			}
		}

		return common.MangaPagesLoadedMsg{
			Pages: pages,
		}
	}
}

// switchProvider handles switching the provider and optionally saving it as default
func (a *App) switchProvider(saveAsDefault bool) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Update UI components with new provider
	if p, err := providers.Get(a.providerName); err == nil {
		a.updateProvider(p)
	}

	// Save to config if requested
	if saveAsDefault {
		if cfg, ok := a.cfg.(*config.Config); ok {
			switch a.currentMediaType {
			case providers.MediaTypeAnime:
				cfg.Providers.Default.Anime = a.providerName
			case providers.MediaTypeMovie, providers.MediaTypeTV, providers.MediaTypeMovieTV:
				cfg.Providers.Default.MoviesAndTV = a.providerName
			}

			if err := cfg.Save(); err != nil {
				a.statusMsg = fmt.Sprintf("⚠ Failed to save config: %v", err)
			} else {
				a.statusMsg = fmt.Sprintf("✓ Default provider set to %s", a.providerName)
			}
			a.statusMsgTime = time.Now()
		}
	} else {
		a.statusMsg = fmt.Sprintf("✓ Switched to %s (temporary)", a.providerName)
		a.statusMsgTime = time.Now()
	}

	// Auto re-search if query exists
	if a.search.GetValue() != "" {
		a.state = loadingView
		a.loadingOp = loadingSearch
		cmds = append(cmds, a.spinner.Tick, a.performSearch(a.search.GetValue()), tea.ClearScreen)
		return a, tea.Batch(cmds...)
	}

	// Return to previous state
	if a.previousState != -1 {
		a.state = a.previousState
		a.previousState = -1
	} else {
		a.state = homeView
	}

	if a.state == homeView {
		cmds = append(cmds, a.home.Init())
	}

	cmds = append(cmds, tea.ClearScreen)
	return a, tea.Batch(cmds...)
}
