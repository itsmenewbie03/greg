package common

import "github.com/justchokingaround/greg/internal/providers"

// WatchPartyInfo contains information for the WatchParty popup
type WatchPartyInfo struct {
	URL               string
	ProxiedURL        string
	WatchPartyURL     string
	Subtitles         []providers.Subtitle
	Referer           string
	Headers           map[string]string
	Title             string
	Type              string
	EpisodeTitle      string
	EpisodeNumber     int
	NextEpisodeID     string
	NextEpisodeTitle  string
	NextEpisodeNumber int
	ProviderName      string
}

// WatchPartyMsg is sent when a WatchParty URL is generated
type WatchPartyMsg struct {
	URL            string
	Err            error
	WatchPartyInfo *WatchPartyInfo // Additional info for the popup
}

// OpenWatchPartyMsg is sent when requesting to open a WatchParty URL in browser
type OpenWatchPartyMsg struct {
	URL string
	Err error
}

// GenerateWatchPartyMsg is sent when requesting to generate a WatchParty URL
type GenerateWatchPartyMsg struct {
	EpisodeID string
	Title     string
	Number    int
}

// ShareViaWatchPartyMsg is sent when user wants to share current media via WatchParty
type ShareViaWatchPartyMsg struct{}

// ShareHistoryViaWatchPartyMsg is sent when user wants to share history item via WatchParty
type ShareHistoryViaWatchPartyMsg struct {
	MediaID      string
	MediaTitle   string
	Episode      int
	Season       int
	ProviderName string
}

// ShareRecentViaWatchPartyMsg is sent when user wants to share recent item via WatchParty
type ShareRecentViaWatchPartyMsg struct {
	MediaID      string
	MediaTitle   string
	Episode      int
	Season       int
	ProviderName string
}

// SetWatchPartyProxyMsg is sent when updating the WatchParty proxy configuration temporarily
type SetWatchPartyProxyMsg struct {
	ProxyURL string
	Origin   string
}

// ShareMediaViaWatchPartyMsg is sent when user wants to share a media from search results via WatchParty
type ShareMediaViaWatchPartyMsg struct {
	MediaID string
	Title   string
	Type    string
}
