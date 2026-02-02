package anilist

import "github.com/justchokingaround/greg/internal/tracker"

// Messages for communication with parent TUI

// SelectMediaMsg is sent when user selects a media to play
type SelectMediaMsg struct {
	Media *tracker.TrackedMedia
}

// OpenStatusUpdateMsg requests opening status update dialog
type OpenStatusUpdateMsg struct {
	Media *tracker.TrackedMedia
}

// OpenScoreUpdateMsg requests opening score update dialog
type OpenScoreUpdateMsg struct {
	Media *tracker.TrackedMedia
}

// OpenProgressUpdateMsg requests opening progress update dialog
type OpenProgressUpdateMsg struct {
	Media *tracker.TrackedMedia
}

// RefreshLibraryMsg requests refreshing the library from AniList
type RefreshLibraryMsg struct{}

// BackMsg requests going back to previous view
type BackMsg struct{}

// LibraryLoadedMsg is sent when library data is loaded
type LibraryLoadedMsg struct {
	Library []tracker.TrackedMedia
	Error   error
}

// StatusUpdatedMsg is sent when status is updated
type StatusUpdatedMsg struct {
	MediaID string
	Status  string
	Error   error
}

// ScoreUpdatedMsg is sent when score is updated
type ScoreUpdatedMsg struct {
	MediaID string
	Score   float64
	Error   error
}

// ProgressUpdatedMsg is sent when progress is updated
type ProgressUpdatedMsg struct {
	MediaID  string
	Episode  int
	Progress float64
	Error    error
}

// ProviderSearchingMsg is sent when searching providers for an AniList anime
type ProviderSearchingMsg struct {
	AniListID int
	Title     string
}

// ProviderSearchResultMsg is sent when provider search completes
type ProviderSearchResultMsg struct {
	AniListID     int
	ProviderName  string        // Name of the provider searched
	Mapping       interface{}   // *mapping.ProviderMapping (if existing mapping found)
	SearchResults []interface{} // []providers.Media (if no mapping, show results to user)
	Error         error
}

// ProviderMappingSelectedMsg is sent when user selects a provider result
type ProviderMappingSelectedMsg struct {
	AniListID     int
	ProviderName  string
	SelectedIndex int
}

// ManualSearchRequestedMsg is sent when user wants to manually search for anime
type ManualSearchRequestedMsg struct {
	AniListID int
	Title     string
}

// RemapRequestedMsg is sent when user wants to change the provider mapping
type RemapRequestedMsg struct {
	Media *tracker.TrackedMedia
}

// SearchNewAnimeMsg is sent when user wants to search for a new anime to add to AniList
type SearchNewAnimeMsg struct{}

// AniListSearchRequestedMsg is sent when user wants to search AniList for a new anime
type AniListSearchRequestedMsg struct {
	Query string
}

// AniListSearchResultMsg is sent when AniList search completes
type AniListSearchResultMsg struct {
	Query   string
	Results []tracker.TrackedMedia
	Error   error
}

// AniListSearchSelectMsg is sent when user selects a search result to add to AniList
type AniListSearchSelectMsg struct {
	Media *tracker.TrackedMedia
}

// AniListAddToListDialogOpenMsg is sent when opening the add-to-list dialog
type AniListAddToListDialogOpenMsg struct {
	Media *tracker.TrackedMedia
}

// AniListAddToListMsg is sent when user selects what status to add a new anime with
type AniListAddToListMsg struct {
	Media  *tracker.TrackedMedia
	Status tracker.WatchStatus
}

// AniListDeleteConfirmationMsg is sent when user requests to delete a media from AniList
type AniListDeleteConfirmationMsg struct {
	Media *tracker.TrackedMedia
}

// AniListDeleteRequestedMsg is sent when user confirms deletion
type AniListDeleteRequestedMsg struct {
	MediaListID int
}

// AniListDeleteResultMsg is sent when deletion completes
type AniListDeleteResultMsg struct {
	Error error
}
