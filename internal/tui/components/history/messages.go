package history

import (
	"github.com/justchokingaround/greg/internal/history"
)

// HistoryLoadedMsg is sent when history is loaded
type HistoryLoadedMsg struct {
	Items []history.HistoryItem
}

// HistoryLoadErrorMsg is sent when history loading fails
type HistoryLoadErrorMsg struct {
	Error error
}

// ResumePlaybackMsg is sent when user wants to resume playback
type ResumePlaybackMsg struct {
	Item history.HistoryItem
}

// DeleteItemMsg is sent when user wants to delete an item
type DeleteItemMsg struct {
	ItemID uint
}

// MarkAsWatchedMsg is sent when user wants to mark an item as watched
type MarkAsWatchedMsg struct {
	ItemID uint
}

// GoToHistoryMsg is sent when user wants to navigate to history view
type GoToHistoryMsg struct{}
