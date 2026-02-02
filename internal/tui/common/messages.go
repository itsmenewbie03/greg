package common

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justchokingaround/greg/internal/player"
	"github.com/justchokingaround/greg/internal/providers"
)

// This file contains custom tea.Msg types for communication between components.

// GoToSearchMsg is a message to switch to the search view.
type GoToSearchMsg struct{}

// GoToHomeMsg is a message to switch to the home view.
type GoToHomeMsg struct{}

// BackMsg is a generic message to go back to the previous view.
type BackMsg struct{}

// ToggleProviderMsg is a message to switch between provider types.
type ToggleProviderMsg struct{}

// PerformSearchMsg is a message that triggers a search.
type PerformSearchMsg struct {
	Query string
}

// SearchResultsMsg is a message that contains the results of a search.
type SearchResultsMsg struct {
	Results []interface{}
	Err     error
}

// MediaSelectedMsg is a message when a media item is selected.
type MediaSelectedMsg struct {
	MediaID string `json:"media_id"`
	Title   string `json:"title"`
	Type    string `json:"type"`
}

// MediaDownloadMsg is a message when a download is requested for a media item (e.g. movie)
type MediaDownloadMsg struct {
	MediaID string
	Title   string
	Type    string
}

// EpisodeSelectedMsg is a message when an episode is selected.
type EpisodeSelectedMsg struct {
	EpisodeID string `json:"episode_id"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
}

// EpisodeDownloadMsg is a message when a download is requested for an episode.
type EpisodeDownloadMsg struct {
	EpisodeID string `json:"episode_id"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
}

// BatchDownloadMsg is a message when batch download is requested for multiple episodes/chapters.
type BatchDownloadMsg struct {
	Episodes []EpisodeInfo
}

// MangaChapterDownloadMsg is a message when a manga chapter download is requested.
type MangaChapterDownloadMsg struct {
	MediaID      string
	MediaTitle   string
	ChapterID    string
	ChapterTitle string
	ChapterNum   int
	Provider     string
}

// BatchMangaDownloadMsg is a message when batch manga download is requested.
type BatchMangaDownloadMsg struct {
	MediaID    string
	MediaTitle string
	Chapters   []MangaChapterInfo
	Provider   string
}

// MangaChapterInfo holds basic chapter information for batch downloads
type MangaChapterInfo struct {
	ChapterID    string
	ChapterTitle string
	ChapterNum   int
}

// SeasonInfo holds basic season information for messaging
type SeasonInfo struct {
	ID     string
	Number int
	Title  string
}

// SeasonsLoadedMsg is a message when seasons are loaded successfully.
type SeasonsLoadedMsg struct {
	Seasons []SeasonInfo
	Error   error
}

// SeasonSelectedMsg is a message when a season is selected.
type SeasonSelectedMsg struct {
	SeasonID string
}

// EpisodeInfo holds basic episode information for messaging
type EpisodeInfo struct {
	EpisodeID string
	Number    int
	Title     string
}

// EpisodesLoadedMsg is a message when episodes are loaded successfully.
type EpisodesLoadedMsg struct {
	Episodes []EpisodeInfo
	Error    error
}

// PlaybackStartingMsg is a message when playback is being initiated
type PlaybackStartingMsg struct {
	EpisodeID     string
	EpisodeNumber int
	EpisodeTitle  string
}

// PlayerLaunchingMsg is a message when player is being launched
type PlayerLaunchingMsg struct{}

// PlayerLaunchTimeoutCheckMsg is a tick message to check player launch status
type PlayerLaunchTimeoutCheckMsg struct{}

// PlaybackStartedMsg is a message when playback has successfully started
type PlaybackStartedMsg struct{}

// PlaybackEndedMsg is a message when playback has ended
type PlaybackEndedMsg struct {
	WatchedPercentage float64
	WatchedDuration   string
	TotalDuration     string
}

// PlaybackErrorMsg is a message when playback encounters an error
type PlaybackErrorMsg struct {
	Error error
}

// GoToAniListMsg is a message to switch to AniList view
type GoToAniListMsg struct{}

// GoToDownloadsMsg is a message to switch to downloads view
type GoToDownloadsMsg struct{}

// GoToHistoryMsg is a message to switch to history view
type GoToHistoryMsg struct {
	MediaType providers.MediaType
}

// DownloadRequestMsg is a message to request downloading an episode
type DownloadRequestMsg struct {
	MediaID       string
	MediaTitle    string
	MediaType     string
	EpisodeID     string
	EpisodeNumber int
	EpisodeTitle  string
	Season        int
	Quality       string
	Provider      string
	StreamURL     string
}

// DownloadProgressUpdateMsg is a message containing download progress update
type DownloadProgressUpdateMsg struct {
	TaskID   string
	Progress float64
	Speed    int64
	ETA      string
	Status   string
}

// DownloadCompleteMsg is a message when a download completes
type DownloadCompleteMsg struct {
	TaskID   string
	FilePath string
}

// DownloadErrorMsg is a message when a download fails
type DownloadErrorMsg struct {
	TaskID string
	Error  error
}

// DownloadAddedMsg is a message when a download is added to queue
type DownloadAddedMsg struct {
	Title    string
	Episode  int
	Quality  string
	Location string // Output file path
}

// ResumePlaybackMsg is a message to resume playback from a specific position
type ResumePlaybackMsg struct {
	MediaID         string
	MediaTitle      string
	MediaType       string // anime, movie, tv, manga
	Episode         int
	Season          int
	ProgressSeconds int
	ProviderName    string
}

// PlaybackAutoReturnMsg is sent after a delay to automatically return from playback completion
type PlaybackAutoReturnMsg struct{}

// MangaPagesLoadedMsg is a message when manga pages are loaded successfully.
type MangaPagesLoadedMsg struct {
	Pages []string
	Err   error
}

type MangaQuitMsg struct{}

// NextChapterMsg is a message to request the next chapter in manga reader
type NextChapterMsg struct{}

// ChapterCompletedMsg is a message when a manga chapter is completed
type ChapterCompletedMsg struct {
	MediaID string
	Chapter int
}

// GenerateDebugInfoMsg is a message to request debug info (source links) for an episode
type GenerateDebugInfoMsg struct {
	EpisodeID string
	Number    int
	Title     string
}

// GenerateMediaDebugInfoMsg is a message to request debug info for a media item (e.g. movie)
type GenerateMediaDebugInfoMsg struct {
	MediaID string
	Title   string
	Type    string
}

// DebugSource holds info for a single source
type DebugSource struct {
	Quality string
	URL     string
	Type    string
	Referer string
}

// DebugSubtitle holds info for a single subtitle
type DebugSubtitle struct {
	Language string
	URL      string
}

// DebugSourcesInfo holds all sources for an episode
type DebugSourcesInfo struct {
	EpisodeTitle  string
	EpisodeNumber int
	Sources       []DebugSource
	Subtitles     []DebugSubtitle
	ProviderName  string
	SelectedIndex int
}

// DebugSourcesLoadedMsg is a message when debug sources are loaded
type DebugSourcesLoadedMsg struct {
	Info  *DebugSourcesInfo
	Error error
}

// DownloadsTickMsg is sent periodically to trigger auto-refresh in downloads view
type DownloadsTickMsg struct{}

// MangaInfoMsg is a message to trigger scraping for manga info.
type MangaInfoMsg struct {
	AnimeTitle string
}

// MangaInfoResultMsg is a message that contains the result of scraping for manga info.
type MangaInfoResultMsg struct {
	Info string
	Err  error
}

// RequestDetailsMsg is a message to request details for a media item
type RequestDetailsMsg struct {
	MediaID string
	Index   int
}

// DetailsLoadedMsg is a message when details for a media item are loaded
type DetailsLoadedMsg struct {
	Media providers.Media
	Index int
	Err   error
}

// SearchProviderMsg is a message to search a specific provider
type SearchProviderMsg struct {
	ProviderName string
	Query        string
	SaveMapping  bool
}

// TickMsg is a message sent on a timer tick.
type TickMsg time.Time

// ProviderStatusesMsg is a message that contains the health statuses of providers.
type ProviderStatusesMsg []*providers.ProviderStatus

// GoToProviderStatusMsg is a message to switch to the provider status view.
type GoToProviderStatusMsg struct{}

// ErrMsg is a message that contains an error.
type ErrMsg struct{ Err error }

// Error satisfies the error interface.
func (e ErrMsg) Error() string { return e.Err.Error() }

// RefreshHistoryMsg is a message to refresh recent history in home view
type RefreshHistoryMsg struct{}

// ShowAudioSelectorMsg is a message to show the audio selector with given tracks
type ShowAudioSelectorMsg struct {
	Tracks       []providers.AudioTrack
	Stream       *providers.StreamURL
	AniListID    int
	EpisodeID    string
	EpisodeNum   int
	EpisodeTitle string
}

// PlaybackTickMsg is sent periodically to trigger playback status polling
type PlaybackTickMsg struct{}

// PlaybackProgressMsg carries the result of an async GetProgress call
type PlaybackProgressMsg struct {
	Progress *player.PlaybackProgress
	Err      error
}

// Component is an interface for a generic Bubble Tea component.
type Component interface {
	Update(tea.Msg) (tea.Model, tea.Cmd)
}
