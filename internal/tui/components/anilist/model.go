package anilist

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tracker"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
	"github.com/justchokingaround/greg/internal/tui/utils"
)

// ViewMode represents different AniList views
type ViewMode int

const (
	ViewLibrary ViewMode = iota
	ViewWatching
	ViewDetails
	ViewStatusUpdate
	ViewScoreUpdate
	ViewSearch
	ViewSearchResults
)

// Model represents the AniList TUI component
type Model struct {
	// Data
	library      []tracker.TrackedMedia
	currentIndex int
	viewMode     ViewMode

	// State
	width  int
	height int
	ready  bool

	// Filter
	statusFilter string // empty = all, or specific status

	// For search within AniList
	searchInput        textinput.Model
	searchQuery        string
	searchResults      []tracker.TrackedMedia
	currentSearchIndex int

	// For adding anime to list
	animeToAdd         *tracker.TrackedMedia
	statusToAddIndex   int // Index for status selection (0: Watching, 1: Plan to Watch, etc.)
	statusToAddOptions []string

	// Dialog state
	dialog DialogState

	// Info dialog
	showInfoDialog bool
	dialogScroll   int

	// Keybindings
	keys KeyMap

	// Fuzzy search
	fuzzySearch *common.FuzzySearch
}

// KeyMap defines keybindings for AniList view
type KeyMap struct {
	Up             key.Binding
	Down           key.Binding
	Select         key.Binding
	Back           key.Binding
	UpdateStatus   key.Binding
	UpdateScore    key.Binding
	UpdateProgress key.Binding
	FilterWatching key.Binding
	FilterAll      key.Binding
	Play           key.Binding
	Refresh        key.Binding
	Remap          key.Binding
	SearchNew      key.Binding
	Delete         key.Binding
}

// DefaultKeyMap returns default keybindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select/play"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc", "q"),
			key.WithHelp("esc/q", "back"),
		),
		UpdateStatus: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "status"),
		),
		UpdateScore: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "rate/score"),
		),
		UpdateProgress: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "progress"),
		),
		FilterWatching: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "watching only"),
		),
		FilterAll: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "all anime"),
		),
		Play: key.NewBinding(
			key.WithKeys("enter", "space"),
			key.WithHelp("enter/space", "play"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "refresh"),
		),
		Remap: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "remap provider"),
		),
		SearchNew: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "search new anime"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete from list"),
		),
	}
}

// New creates a new AniList model
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "Search Anilist..."
	ti.Prompt = ""
	ti.CharLimit = 200
	ti.Width = 60 // Default width, will be updated by SetSize

	// Styling for consistency with AniList theme
	ti.Prompt = "" // We'll add the prompt in the View function to match search component
	ti.PromptStyle = styles.AniListTitleStyle
	ti.TextStyle = styles.AniListMetadataStyle
	ti.PlaceholderStyle = styles.AniListMetadataStyle

	return Model{
		library:            []tracker.TrackedMedia{},
		currentIndex:       0,
		viewMode:           ViewLibrary,
		statusFilter:       "CURRENT", // Default to watching
		dialog:             InitDialogState(),
		keys:               DefaultKeyMap(),
		searchInput:        ti,
		searchResults:      []tracker.TrackedMedia{},
		currentSearchIndex: 0,
		animeToAdd:         nil,
		statusToAddIndex:   0,
		statusToAddOptions: []string{"CURRENT", "PLANNING", "COMPLETED", "PAUSED", "DROPPED"},
		fuzzySearch:        common.NewFuzzySearch(),
	}
}

// SetSize sets the model dimensions
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.ready = true
	m.searchInput.Width = width - 10 // Adjust text input width to leave some margin
	m.fuzzySearch.SetWidth(width)
}

// SetLibrary sets the anime library data
func (m *Model) SetLibrary(library []tracker.TrackedMedia) {
	m.library = library
	m.currentIndex = 0
}

// GetSelectedMedia returns the currently selected media
func (m *Model) GetSelectedMedia() *tracker.TrackedMedia {
	filtered := m.GetFilteredLibrary()
	if len(filtered) == 0 || m.currentIndex < 0 || m.currentIndex >= len(filtered) {
		return nil
	}
	return &filtered[m.currentIndex]
}

// FilterByStatus filters the library by watch status
func (m *Model) FilterByStatus(status string) {
	m.statusFilter = status
	m.currentIndex = 0
}

// GetFilteredLibrary returns the library filtered by current status
func (m *Model) GetFilteredLibrary() []tracker.TrackedMedia {
	if m.statusFilter == "" {
		// Copy library to avoid modifying original
		sorted := make([]tracker.TrackedMedia, len(m.library))
		copy(sorted, m.library)

		// Sort by UpdatedAt descending (most recent first)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].UpdatedAt.After(sorted[j].UpdatedAt)
		})

		return sorted
	}

	filtered := []tracker.TrackedMedia{}
	for _, media := range m.library {
		if string(media.Status) == m.statusFilter ||
			mapStatusToAniList(string(media.Status)) == m.statusFilter {
			filtered = append(filtered, media)
		}
	}

	// Sort filtered results by UpdatedAt descending
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
	})

	return filtered
}

// RenderLibraryView renders the library list (mangal-style)
func (m Model) RenderLibraryView() string {
	if !m.ready {
		return "Loading..."
	}

	filtered := m.GetFilteredLibrary()
	if len(filtered) == 0 {
		return styles.AniListMetadataStyle.Render("No anime found. Press 'a' to view all, or 'ctrl+r' to refresh.")
	}

	var output string

	// Header with count
	filterName := m.statusFilter
	switch filterName {
	case "":
		filterName = "All Items"
	case "CURRENT":
		if len(m.library) > 0 && m.library[0].Type == providers.MediaTypeManga {
			filterName = "Reading"
		} else {
			filterName = "Watching"
		}
	}

	header := styles.AniListHeaderStyle.Render(fmt.Sprintf("  %s  ", filterName))
	output += header + "\n"

	// Get fuzzy filtered indices
	filteredIndices := m.getFilteredLibraryIndices()
	displayCount := len(filteredIndices)

	// Determine term based on content
	term := "anime"
	if len(m.library) > 0 && m.library[0].Type == providers.MediaTypeManga {
		term = "manga"
	}

	countInfo := fmt.Sprintf("%d %s", displayCount, term)
	if m.fuzzySearch.IsActive() && m.fuzzySearch.Query() != "" {
		countInfo += fmt.Sprintf(" (filtered from %d)", len(filtered))
	}
	if displayCount > 0 {
		countInfo += fmt.Sprintf(" • Viewing %d of %d", m.currentIndex+1, displayCount)
	}
	output += styles.AniListMetadataStyle.Render(countInfo) + "\n"

	// Show fuzzy search input if active
	if m.fuzzySearch.IsActive() {
		output += "\n" + m.fuzzySearch.View() + "\n\n"
	} else {
		output += "\n"
	}

	// Render visible items
	visibleStart := 0
	visibleEnd := displayCount

	// Adjust visible range if list is long
	// Each item now takes ~3-4 lines (title, metadata, borders, margins)
	// Account for header (4 lines) and help text (2 lines)
	// Limit to reasonable number for less clutter
	itemsPerPage := (m.height - 6) / 4
	if itemsPerPage < 1 {
		itemsPerPage = 1
	}
	// Cap at 8 items for less clutter
	if itemsPerPage > 8 {
		itemsPerPage = 8
	}

	if displayCount > itemsPerPage {
		// Center the current selection
		visibleStart = m.currentIndex - itemsPerPage/2
		if visibleStart < 0 {
			visibleStart = 0
		}
		visibleEnd = visibleStart + itemsPerPage
		if visibleEnd > displayCount {
			visibleEnd = displayCount
			visibleStart = visibleEnd - itemsPerPage
			if visibleStart < 0 {
				visibleStart = 0
			}
		}
	}

	for i := visibleStart; i < visibleEnd; i++ {
		if i >= len(filteredIndices) {
			break
		}
		actualIndex := filteredIndices[i]
		if actualIndex >= len(filtered) {
			break
		}
		media := filtered[actualIndex]
		output += m.renderMediaItem(media, i == m.currentIndex) + "\n\n"
	}

	// Help text
	if m.fuzzySearch.IsActive() {
		if m.fuzzySearch.IsLocked() {
			output += "\n" + styles.AniListHelpStyle.Render("↑/↓ nav • enter play • i info • s status • r rate • d del • / edit • esc clear")
		} else {
			output += "\n" + styles.AniListHelpStyle.Render("Type to filter • ↑/↓ nav • esc lock")
		}
	} else {
		output += "\n" + m.renderHelp()
	}

	return output
}

// renderMediaItem renders a single media item (mangal-style with purple border)
func (m Model) renderMediaItem(media tracker.TrackedMedia, selected bool) string {
	style := styles.AniListItemStyle
	titleStyle := styles.AniListTitleStyle
	metaStyle := styles.AniListMetadataStyle

	if selected {
		style = styles.AniListItemSelectedStyle
		titleStyle = titleStyle.Foreground(styles.OxocarbonPurple)
		metaStyle = metaStyle.Foreground(styles.OxocarbonMauve)
	}

	var lines []string

	// Line 1: Title
	lines = append(lines, titleStyle.Render(media.Title))

	// Line 2: Type • Progress • Score • Status
	var metaParts []string

	// Type
	if media.Type != "" {
		metaParts = append(metaParts, string(media.Type))
	}

	// Progress
	unit := "episodes"
	if media.Type == providers.MediaTypeManga {
		unit = "chapters"
	}
	progress := fmt.Sprintf("%d/%d %s", media.Progress, media.TotalEpisodes, unit)
	if media.TotalEpisodes == 0 {
		progress = fmt.Sprintf("%d %s", media.Progress, unit)
	}
	metaParts = append(metaParts, progress)

	// Score
	if media.Score > 0 {
		metaParts = append(metaParts, fmt.Sprintf("★ %.1f", media.Score))
	}

	// Combine metadata
	metaLine := metaStyle.Render(strings.Join(metaParts, " • "))

	// Status badge
	statusBadge := styles.FormatStatusBadge(formatStatus(media.Status, media.Type))
	lines = append(lines, metaLine+" • "+statusBadge)

	content := strings.Join(lines, "\n")
	return style.Render(content)
}

// formatStatus returns the display text for a status based on media type
func formatStatus(status tracker.WatchStatus, mediaType providers.MediaType) string {
	isManga := mediaType == providers.MediaTypeManga
	s := string(status)

	switch s {
	case "watching":
		if isManga {
			return "Reading"
		}
		return "Watching"
	case "plan_to_watch":
		if isManga {
			return "Plan to Read"
		}
		return "Plan to Watch"
	case "completed":
		return "Completed"
	case "on_hold":
		if isManga {
			return "Paused"
		}
		return "On Hold"
	case "dropped":
		return "Dropped"
	case "rewatching":
		if isManga {
			return "Rereading"
		}
		return "Rewatching"
	default:
		return s
	}
}

// renderHelp renders the help text
func (m Model) renderHelp() string {
	helps := []string{
		"↑/↓ navigate",
		"enter play",
		"i info",
		"s status",
		"r rate",
		"p progress",
		"d delete",
		"/ filter",
		"w watching",
		"a all",
		"n new",
		"P provider",
		"m manga info",
		"? help",
		"q back",
	}

	helpStr := ""
	for i, h := range helps {
		if i > 0 {
			helpStr += " • "
		}
		helpStr += h
	}

	return styles.AniListHelpStyle.Render(helpStr)
}

// mapStatusToAniList converts tracker status to AniList status
func mapStatusToAniList(status string) string {
	switch status {
	case "watching":
		return "CURRENT"
	case "completed":
		return "COMPLETED"
	case "on_hold":
		return "PAUSED"
	case "dropped":
		return "DROPPED"
	case "plan_to_watch":
		return "PLANNING"
	case "rewatching":
		return "REPEATING"
	default:
		return status
	}
}

// getFilteredLibraryIndices returns the indices of library items that match the fuzzy search
func (m Model) getFilteredLibraryIndices() []int {
	filtered := m.GetFilteredLibrary()
	searchStrings := make([]string, len(filtered))
	for i, media := range filtered {
		searchStrings[i] = media.Title
	}
	return m.fuzzySearch.Filter(searchStrings)
}

// getFilteredSearchIndices returns the indices of search results that match the fuzzy search
func (m Model) getFilteredSearchIndices() []int {
	searchStrings := make([]string, len(m.searchResults))
	for i, media := range m.searchResults {
		searchStrings[i] = media.Title
	}
	return m.fuzzySearch.Filter(searchStrings)
}

// View renders the model
func (m Model) View() string {
	var baseView string
	switch m.viewMode {
	case ViewLibrary, ViewWatching:
		baseView = m.RenderLibraryView()
	case ViewSearch:
		baseView = m.RenderSearchView()
	case ViewSearchResults:
		baseView = m.RenderSearchResultsView()
	default:
		baseView = m.RenderLibraryView()
	}

	// Overlay info dialog if shown
	if m.showInfoDialog {
		filtered := m.GetFilteredLibrary()
		filteredIndices := m.getFilteredLibraryIndices()

		// Use filtered indices to get the actual media
		if m.currentIndex < len(filteredIndices) {
			actualIndex := filteredIndices[m.currentIndex]
			if actualIndex < len(filtered) {
				media := filtered[actualIndex]
				dialog := m.renderInfoDialog(media)
				return lipgloss.Place(m.width, m.height,
					lipgloss.Center, lipgloss.Center,
					dialog,
					lipgloss.WithWhitespaceChars(" "),
					lipgloss.WithWhitespaceForeground(lipgloss.Color("#161616")))
			}
		}
	}

	return baseView
}

// RenderSearchView renders the search input view
func (m Model) RenderSearchView() string {
	if !m.ready {
		return "Loading search..."
	}

	var output string

	// Header
	output += styles.AniListHeaderStyle.Render("  Search Anilist  ") + "\n"
	output += styles.AniListMetadataStyle.Render("Enter anime title to search on Anilist\n")

	// Create a prompt similar to the search component
	prompt := styles.AniListTitleStyle.Render("┃")

	// Combine prompt with text input (similar to search component)
	output += "\n" + prompt + " " + m.searchInput.View() + "\n\n"

	// Help text
	output += styles.AniListHelpStyle.Render("enter search • esc cancel")

	return output
}

// RenderSearchResultsView renders the search results
func (m Model) RenderSearchResultsView() string {
	if !m.ready {
		return "Loading search results..."
	}

	var output string

	// Header
	output += styles.AniListHeaderStyle.Render("  Search Results  ") + "\n"

	if len(m.searchResults) == 0 {
		output += styles.AniListMetadataStyle.Render("No results found for: " + m.searchQuery)
		output += "\n\n" + styles.AniListHelpStyle.Render("↑/↓ nav • enter select • esc back")
		return output
	} else {
		// Get fuzzy filtered indices
		filteredIndices := m.getFilteredSearchIndices()
		displayCount := len(filteredIndices)

		countInfo := fmt.Sprintf("%d results for '%s'", displayCount, m.searchQuery)
		if m.fuzzySearch.IsActive() && m.fuzzySearch.Query() != "" {
			countInfo = fmt.Sprintf("%d results (filtered from %d) for '%s'", displayCount, len(m.searchResults), m.searchQuery)
		}
		if displayCount > 0 {
			countInfo += fmt.Sprintf(" • Result %d of %d", m.currentSearchIndex+1, displayCount)
		}
		output += styles.AniListMetadataStyle.Render(countInfo) + "\n"

		// Show fuzzy search input if active
		if m.fuzzySearch.IsActive() {
			output += "\n" + m.fuzzySearch.View() + "\n\n"
		} else {
			output += "\n"
		}

		// Show visible results
		visibleStart := 0
		visibleEnd := displayCount

		// Same logic as library view for pagination
		itemsPerPage := (m.height - 6) / 5
		if itemsPerPage < 1 {
			itemsPerPage = 1
		}

		if displayCount > itemsPerPage {
			visibleStart = m.currentSearchIndex - itemsPerPage/2
			if visibleStart < 0 {
				visibleStart = 0
			}
			visibleEnd = visibleStart + itemsPerPage
			if visibleEnd > displayCount {
				visibleEnd = displayCount
				visibleStart = visibleEnd - itemsPerPage
				if visibleStart < 0 {
					visibleStart = 0
				}
			}
		}

		for i := visibleStart; i < visibleEnd; i++ {
			if i >= len(filteredIndices) {
				break
			}
			actualIndex := filteredIndices[i]
			if actualIndex >= len(m.searchResults) {
				break
			}
			media := m.searchResults[actualIndex]
			output += m.renderSearchResultItem(media, i == m.currentSearchIndex) + "\n\n"
		}
	}

	// Help text
	if m.fuzzySearch.IsActive() {
		if m.fuzzySearch.IsLocked() {
			output += "\n" + styles.AniListHelpStyle.Render("↑/↓ nav • enter select • / edit • esc clear")
		} else {
			output += "\n" + styles.AniListHelpStyle.Render("Type to filter • ↑/↓ nav • esc lock")
		}
	} else {
		output += "\n" + styles.AniListHelpStyle.Render("↑/↓ nav • enter select • / filter • esc back")
	}

	return output
}

// renderSearchResultItem renders a single search result item
func (m Model) renderSearchResultItem(media tracker.TrackedMedia, selected bool) string {
	style := styles.AniListItemStyle
	if selected {
		style = styles.AniListItemSelectedStyle
	}

	// Title
	title := styles.AniListTitleStyle.Render(media.Title)

	// Episodes info
	var epInfo string
	unit := "episodes"
	if media.Type == providers.MediaTypeManga {
		unit = "chapters"
	}
	if media.TotalEpisodes > 0 {
		epInfo = fmt.Sprintf("%d %s", media.TotalEpisodes, unit)
	} else {
		if unit != "" {
			epInfo = fmt.Sprintf("%s unknown", strings.ToUpper(unit[:1])+unit[1:])
		} else {
			epInfo = "unknown"
		}
	}
	epInfoStr := styles.AniListMetadataStyle.Render(epInfo)

	// Combine
	line1 := title
	line2 := epInfoStr

	content := line1 + "\n" + line2

	return style.Render(content)
}

// formatTimeAgo formats a time as a relative time string
func formatTimeAgo(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	case diff < 30*24*time.Hour:
		weeks := int(diff.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / 24 / 30)
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(diff.Hours() / 24 / 365)
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}

// renderInfoDialog renders a detailed information dialog for an anime
func (m Model) renderInfoDialog(media tracker.TrackedMedia) string {
	// Calculate dialog dimensions first
	dialogWidth := 80
	if m.width > 0 && m.width < 90 {
		dialogWidth = m.width - 10
		if dialogWidth < 50 {
			dialogWidth = 50 // Minimum width
		}
	}

	// Calculate content width (dialog width minus border and padding)
	contentWidth := dialogWidth - 6

	// Calculate max synopsis lines based on terminal height
	maxSynopsisLines := 10 // Default
	if m.height > 0 {
		overhead := 21 // Title, metadata, genres, help, padding, scroll indicators
		availableLines := m.height - overhead
		if availableLines > 5 {
			maxSynopsisLines = availableLines
		} else {
			maxSynopsisLines = 5 // Minimum
		}
		if maxSynopsisLines > 20 {
			maxSynopsisLines = 20
		}
	}

	var output string

	// Title
	title := styles.AniListHeaderStyle.Render("Anime Information")
	output += title + "\n\n"

	// Anime title
	output += styles.AniListTitleStyle.Render(media.Title) + "\n\n"

	// Metadata
	var metaParts []string
	if media.Type != "" {
		metaParts = append(metaParts, fmt.Sprintf("Type: %s", string(media.Type)))
	}
	unit := "Episodes"
	action := "watched"
	if media.Type == providers.MediaTypeManga {
		unit = "Chapters"
		action = "read"
	}
	if media.TotalEpisodes > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%s: %d/%d %s", unit, media.Progress, media.TotalEpisodes, action))
	} else {
		metaParts = append(metaParts, fmt.Sprintf("%s: %d %s", unit, media.Progress, action))
	}
	if media.Score > 0 {
		metaParts = append(metaParts, fmt.Sprintf("Your Score: ★ %.1f", media.Score))
	}
	if media.Status != "" {
		metaParts = append(metaParts, fmt.Sprintf("Status: %s", formatStatus(media.Status, media.Type)))
	}

	for _, part := range metaParts {
		output += styles.AniListMetadataStyle.Render(part) + "\n"
	}

	// Date information
	if media.StartDate != nil {
		output += styles.AniListMetadataStyle.Render(fmt.Sprintf("Started: %s", media.StartDate.Format("Jan 2, 2006"))) + "\n"
	}
	if media.EndDate != nil {
		output += styles.AniListMetadataStyle.Render(fmt.Sprintf("Completed: %s", media.EndDate.Format("Jan 2, 2006"))) + "\n"
	}
	if !media.UpdatedAt.IsZero() {
		timeAgo := formatTimeAgo(media.UpdatedAt)
		output += styles.AniListMetadataStyle.Render(fmt.Sprintf("Last Updated: %s", timeAgo)) + "\n"
	}
	output += "\n"

	// Synopsis with scrolling (fixed viewport)
	if media.Synopsis != "" {
		output += styles.AniListTitleStyle.Render("Synopsis:") + "\n"
		wrappedSynopsis := utils.WrapText(media.Synopsis, contentWidth)

		// Calculate scroll bounds
		maxScroll := len(wrappedSynopsis) - maxSynopsisLines
		if maxScroll < 0 {
			maxScroll = 0
		}

		// Clamp scroll offset
		scrollOffset := m.dialogScroll
		if scrollOffset > maxScroll {
			scrollOffset = maxScroll
		}
		if scrollOffset < 0 {
			scrollOffset = 0
		}

		// Determine scroll indicators
		canScrollUp := scrollOffset > 0
		canScrollDown := scrollOffset < maxScroll

		// Always reserve space for top indicator
		if canScrollUp {
			output += styles.AniListMetadataStyle.Render("  ▲ scroll up") + "\n"
		} else {
			output += "\n"
		}

		// Get visible lines based on scroll
		start := scrollOffset
		end := start + maxSynopsisLines
		if end > len(wrappedSynopsis) {
			end = len(wrappedSynopsis)
		}

		visibleLines := wrappedSynopsis[start:end]

		// Render visible synopsis lines (always fill to maxSynopsisLines)
		for i := 0; i < maxSynopsisLines; i++ {
			if i < len(visibleLines) {
				output += styles.SynopsisStyle.Render(visibleLines[i]) + "\n"
			} else {
				output += "\n"
			}
		}

		// Always reserve space for bottom indicator
		if canScrollDown {
			output += styles.AniListMetadataStyle.Render("  ▼ scroll down") + "\n"
		} else {
			output += "\n"
		}
		output += "\n"
	}

	output += styles.AniListHelpStyle.Render("↑/↓ scroll • enter/esc close")

	// Box it with calculated width
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OxocarbonPurple).
		Padding(1, 2).
		Width(dialogWidth)

	return boxStyle.Render(output)
}
