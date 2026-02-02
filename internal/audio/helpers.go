package audio

import (
	"strings"

	"github.com/justchokingaround/greg/internal/providers"
)

// DetectAudioType classifies audio track from label when Type field not set by provider
// Returns "dub", "sub", "original", or "unknown"
func DetectAudioType(label string) string {
	lowerLabel := strings.ToLower(label)

	// Dub indicators (English dubs most common)
	if strings.Contains(lowerLabel, "dub") ||
		strings.Contains(lowerLabel, "english") ||
		strings.Contains(lowerLabel, "dubbed") ||
		strings.Contains(lowerLabel, "eng") {
		return "dub"
	}

	// Sub indicators (Japanese original with subs)
	if strings.Contains(lowerLabel, "sub") ||
		strings.Contains(lowerLabel, "japanese") ||
		strings.Contains(lowerLabel, "original") ||
		strings.Contains(lowerLabel, "jpn") ||
		strings.Contains(lowerLabel, "jp") {
		return "sub"
	}

	// Unknown - provider label ambiguous
	return "unknown"
}

// SelectAudioTrack finds best matching audio track for user preference
// Returns nil if no match found (triggers user prompt in TUI)
func SelectAudioTrack(tracks []providers.AudioTrack, preference string) *providers.AudioTrack {
	if len(tracks) == 0 {
		return nil
	}

	// Single track available - return it (no choice)
	if len(tracks) == 1 {
		return &tracks[0]
	}

	// Exact match on Type field (provider explicitly set dub/sub)
	for i := range tracks {
		if tracks[i].Type == preference {
			return &tracks[i]
		}
	}

	// Fuzzy match via label detection (fallback when Type unknown)
	for i := range tracks {
		if tracks[i].Type == "" || tracks[i].Type == "unknown" {
			detected := DetectAudioType(tracks[i].Label)
			if detected == preference {
				// Update Type for future reference
				tracks[i].Type = detected
				return &tracks[i]
			}
		}
	}

	// No match - return nil to trigger user prompt
	// CONTEXT.md: "When preferred track unavailable: prompt user with fuzzy search selector"
	return nil
}

// NormalizeAudioLabel formats audio track for display in TUI
// Returns: "[DUB] English (Original: English Audio)"
func NormalizeAudioLabel(track providers.AudioTrack) string {
	trackType := track.Type
	if trackType == "" || trackType == "unknown" {
		trackType = DetectAudioType(track.Label)
	}

	typeLabel := strings.ToUpper(trackType)
	return "[" + typeLabel + "] " + track.Language + " (Original: " + track.Label + ")"
}
