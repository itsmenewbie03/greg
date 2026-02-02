package history

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gorm.io/gorm"

	"github.com/justchokingaround/greg/internal/database"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

// Model represents the history TUI component
type Model struct {
	// Data
	history      []database.History
	currentIndex int

	// State
	width  int
	height int
	ready  bool

	// Database
	db *gorm.DB

	// Filter
	mediaTypeFilter string // "all", "anime", "movie", "tv"
	sortOrder       string // "recent", " progress", "title"

	// Search
	searchInput textinput.Model
	query       string

	// Fuzzy search
	fuzzySearch *common.FuzzySearch

	// Keybindings
	keys KeyMap
}

// KeyMap defines keybindings for history view
type KeyMap struct {
	Up          key.Binding
	Down        key.Binding
	Select      key.Binding
	Back        key.Binding
	Search      key.Binding
	ClearSearch key.Binding
	FilterAll   key.Binding
	FilterAnime key.Binding
	FilterMovie key.Binding
	FilterManga key.Binding
	SortRecent  key.Binding
	SortTitle   key.Binding
	SortPercent key.Binding
	Delete      key.Binding
	DeleteAll   key.Binding
	Help        key.Binding
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
			key.WithHelp("enter", "play"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc", "q"),
			key.WithHelp("esc/q", "back"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		ClearSearch: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "clear search"),
		),
		FilterAll: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "all media"),
		),
		FilterAnime: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "anime only"),
		),
		FilterMovie: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "movies & tv"),
		),
		FilterManga: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "manga only"),
		),
		SortRecent: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "sort by recent"),
		),
		SortTitle: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "sort by title"),
		),
		SortPercent: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "sort by progress"),
		),
		Delete: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "delete item"),
		),
		DeleteAll: key.NewBinding(
			key.WithKeys("X"),
			key.WithHelp("X", "delete all"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
	}
}

// New creates a new history model
func New(db *gorm.DB) Model {

	ti := textinput.New()
	ti.Placeholder = "Search history..."
	ti.Prompt = ""
	ti.CharLimit = 200
	ti.Width = 60

	// Styling
	ti.PromptStyle = styles.AniListTitleStyle
	ti.TextStyle = styles.AniListMetadataStyle
	ti.PlaceholderStyle = styles.AniListMetadataStyle

	return Model{
		history:         []database.History{}, // Start with empty history
		currentIndex:    0,
		mediaTypeFilter: "all",
		sortOrder:       "recent",
		db:              db, // Store the database
		searchInput:     ti,
		query:           "",
		fuzzySearch:     common.NewFuzzySearch(),
		keys:            DefaultKeyMap(),
		ready:           false, // Not ready until we load data
	}
}

// SetSize sets the model dimensions
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.ready = true
	m.searchInput.Width = width - 10
	m.fuzzySearch.SetWidth(width)
}

// SetHistory sets the history data
func (m *Model) SetHistory(history []database.History) {
	m.history = history
	m.currentIndex = 0
}

// GetSelectedHistory returns the currently selected history item
func (m *Model) GetSelectedHistory() *database.History {
	filtered := m.GetFilteredHistory()
	if len(filtered) == 0 || m.currentIndex < 0 || m.currentIndex >= len(filtered) {
		return nil
	}
	return &filtered[m.currentIndex]
}

// FilterByMediaType filters the history by media type
func (m *Model) FilterByMediaType(mediaType string) {
	m.mediaTypeFilter = mediaType
	m.currentIndex = 0
}

// SetMediaTypeFilter sets the media type filter from a providers.MediaType.
func (m *Model) SetMediaTypeFilter(mediaType providers.MediaType) {
	switch mediaType {
	case providers.MediaTypeAnime:
		m.mediaTypeFilter = "anime"
	case providers.MediaTypeMovie, providers.MediaTypeTV, providers.MediaTypeMovieTV:
		m.mediaTypeFilter = "movie"
	case providers.MediaTypeManga:
		m.mediaTypeFilter = "manga"
	default:
		m.mediaTypeFilter = "all"
	}
	m.currentIndex = 0
}

// SortBy sets the sort order
func (m *Model) SortBy(order string) {
	m.sortOrder = order
	m.currentIndex = 0
}

// Search sets the search query
func (m *Model) Search(query string) {
	m.query = query
	m.currentIndex = 0
}

// GetFilteredHistory returns the history filtered by current filters
func (m *Model) GetFilteredHistory() []database.History {
	// Start with all history
	fullHistory := make([]database.History, len(m.history))
	copy(fullHistory, m.history)

	deduplicatedHistory := []database.History{}
	seenEntries := make(map[string]bool)
	for _, item := range fullHistory {
		key := fmt.Sprintf("%s-%d-%d", item.MediaID, item.Episode, item.Season)
		if _, seen := seenEntries[key]; !seen {
			deduplicatedHistory = append(deduplicatedHistory, item)
			seenEntries[key] = true
		}
	}

	filtered := deduplicatedHistory

	// Apply media type filter
	if m.mediaTypeFilter != "all" {
		temp := []database.History{}
		for _, item := range filtered {
			if m.mediaTypeFilter == "movie" {
				if item.MediaType == "movie" || item.MediaType == "tv" {
					temp = append(temp, item)
				}
			} else if item.MediaType == m.mediaTypeFilter {
				temp = append(temp, item)
			}
		}
		filtered = temp
	}

	// Apply search query if active
	if m.query != "" {
		temp := []database.History{}
		for _, item := range filtered {
			// Search in title
			if strings.Contains(strings.ToLower(item.MediaTitle), strings.ToLower(m.query)) {
				temp = append(temp, item)
				continue
			}
			// Search in provider
			if strings.Contains(strings.ToLower(item.ProviderName), strings.ToLower(m.query)) {
				temp = append(temp, item)
			}
		}
		filtered = temp
	}

	// Apply fuzzy search if active
	if m.fuzzySearch.IsActive() && m.fuzzySearch.Query() != "" {
		searchStrings := make([]string, len(filtered))
		for i, item := range filtered {
			searchStrings[i] = item.MediaTitle
		}
		indices := m.fuzzySearch.Filter(searchStrings)
		temp := make([]database.History, len(indices))
		for i, idx := range indices {
			temp[i] = filtered[idx]
		}
		filtered = temp
	}

	// Apply sort order
	switch m.sortOrder {
	case "recent":
		// Already sorted by recent (default from database)
	case "title":
		sort.Slice(filtered, func(i, j int) bool {
			return strings.ToLower(filtered[i].MediaTitle) < strings.ToLower(filtered[j].MediaTitle)
		})
	case "progress":
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].ProgressPercent > filtered[j].ProgressPercent
		})
	}

	return filtered
}

// LoadHistory loads history from the database
func (m *Model) LoadHistory(db *gorm.DB) error {
	var history []database.History
	err := db.Order("watched_at DESC").Find(&history).Error
	if err != nil {
		return err
	}
	m.SetHistory(history)
	return nil
}

// Refresh loads history from the stored database
func (m *Model) Refresh() tea.Cmd {
	if m.db != nil {
		return loadHistoryCmd(m.db)
	}
	return nil
}

// DeleteHistoryItem deletes a specific history item from the database
func (m *Model) DeleteHistoryItem(db *gorm.DB, id uint) error {
	dbToUse := db
	if dbToUse == nil {
		dbToUse = m.db
	}

	if dbToUse == nil {
		return fmt.Errorf("database is nil")
	}

	err := dbToUse.Delete(&database.History{}, id).Error
	if err != nil {
		return err
	}
	// Reload history
	return m.LoadHistory(dbToUse)
}

// DeleteAllHistory deletes all history from the database
func (m *Model) DeleteAllHistory(db *gorm.DB) error {
	dbToUse := db
	if dbToUse == nil {
		dbToUse = m.db
	}

	if dbToUse == nil {
		return fmt.Errorf("database is nil")
	}

	err := dbToUse.Where("1 = 1").Delete(&database.History{}).Error
	if err != nil {
		return err
	}
	m.SetHistory([]database.History{})
	return nil
}

// View renders the history view
func (m Model) View() string {
	if !m.ready {
		return "Loading history..."
	}

	filtered := m.GetFilteredHistory()

	if len(filtered) == 0 {
		if m.query != "" {
			return styles.SubtitleStyle.Render("\nNo history items match your search.\n\nPress 'esc' to go back.")
		}
		return styles.SubtitleStyle.Render("\nNo watch history yet. Start watching to see history here.\n\nPress 'esc' to go back.")
	}

	var content strings.Builder

	content.WriteString("\n\n")

	header := styles.TitleStyle.Render("  Watch History  ")
	content.WriteString(header + "\n")

	filterText := "All Media"
	switch m.mediaTypeFilter {
	case "anime":
		filterText = "Anime Only"
	case "movie":
		filterText = "Movies & TV"
	case "manga":
		filterText = "Manga Only"
	}

	sortText := "Recent"
	switch m.sortOrder {
	case "title":
		sortText = "Title"
	case "progress":
		sortText = "Progress"
	}

	count := styles.SubtitleStyle.Render(fmt.Sprintf("  %d items", len(filtered)))
	if m.fuzzySearch.IsActive() && m.fuzzySearch.Query() != "" {
		count += styles.AniListMetadataStyle.Render(fmt.Sprintf(" (filtered) • %s • %s", filterText, sortText))
	} else {
		count += styles.AniListMetadataStyle.Render(fmt.Sprintf(" • %s • %s", filterText, sortText))
	}
	content.WriteString(count + "\n")

	if m.fuzzySearch.IsActive() {
		fuzzyView := m.fuzzySearch.View()
		// Add border highlight when editing (active and not locked)
		if !m.fuzzySearch.IsLocked() {
			borderStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(styles.OxocarbonPurple).
				Padding(0, 1)
			fuzzyView = borderStyle.Render(fuzzyView)
		}
		content.WriteString("\n" + fuzzyView + "\n\n")
	} else {
		content.WriteString("\n")
	}

	visibleStart, visibleEnd := m.getVisibleRange(len(filtered))

	for i := visibleStart; i < visibleEnd; i++ {
		item := filtered[i]
		content.WriteString(m.renderHistoryItem(item, i == m.currentIndex) + "\n\n")
	}

	helpText := "  ↑/↓ nav • enter play • / search • 1-4 filter • r/t/p sort • x del • q back"
	if m.fuzzySearch.IsActive() {
		if m.fuzzySearch.IsLocked() {
			helpText = "  ↑/↓ nav • enter play • / edit • q back"
		} else {
			helpText = "  Type to filter • esc lock • q back"
		}
	}
	styledHelpText := styles.AniListHelpStyle.Render(helpText)

	if m.height > 0 {
		contentLines := strings.Split(content.String(), "\n")
		contentHeight := len(contentLines)

		availableContentHeight := m.height - 2

		if contentHeight < availableContentHeight {
			padding := strings.Repeat("\n", availableContentHeight-contentHeight)
			return content.String() + padding + "\n" + styledHelpText
		}
	}

	return content.String() + "\n" + styledHelpText
}

// getVisibleRange calculates the visible range of items for pagination.
func (m Model) getVisibleRange(total int) (int, int) {
	// Default to showing 3 items if height not set yet
	maxVisible := 3

	if m.height > 0 {
		// Calculate max visible items to ensure header stays visible
		overhead := 7
		if m.fuzzySearch.IsActive() {
			overhead = 10 // Fuzzy search takes more space
		}

		itemsSpace := m.height - overhead
		if itemsSpace > 0 {
			// Each item takes ~8 lines (6 for content, 2 for newlines)
			linesPerItem := 4
			maxVisible = itemsSpace / linesPerItem
		}

		if maxVisible < 1 {
			maxVisible = 1
		}

		// if maxVisible > 20 {
		// 	maxVisible = 20
		// }
	}

	if total <= maxVisible {
		return 0, total
	}

	// Keep selection centered
	start := 0
	if m.currentIndex > maxVisible/2 {
		start = m.currentIndex - maxVisible/2
	}

	if start < 0 {
		start = 0
	}

	end := start + maxVisible
	if end > total {
		end = total
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	return start, end
}

// renderHistoryItem renders a single history item
func (m Model) renderHistoryItem(item database.History, selected bool) string {
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
	title := item.MediaTitle
	if item.Episode > 0 {
		if item.Season > 0 {
			title = fmt.Sprintf("%s - S%d E%d", item.MediaTitle, item.Season, item.Episode)
		} else {
			prefix := "Episode"
			if item.MediaType == "manga" {
				prefix = "Chapter"
			}
			title = fmt.Sprintf("%s - %s %d", item.MediaTitle, prefix, item.Episode)
		}
	}
	lines = append(lines, titleStyle.Render(title))

	// Line 2: Progress, Provider, Date
	var metaParts []string

	// Progress
	if item.Completed {
		metaParts = append(metaParts, "Completed")
	} else {
		if item.MediaType == "manga" {
			if item.TotalPages > 0 {
				metaParts = append(metaParts, fmt.Sprintf("Page %d/%d (%.0f%%)", item.Page, item.TotalPages, item.ProgressPercent))
			} else {
				metaParts = append(metaParts, fmt.Sprintf("Page %d", item.Page))
			}
		} else {
			metaParts = append(metaParts, fmt.Sprintf("%.0f%%", item.ProgressPercent))
		}
	}

	// Provider
	if item.ProviderName != "" {
		metaParts = append(metaParts, item.ProviderName)
	}

	// Date
	timeAgo := formatTimeAgo(item.WatchedAt)
	metaParts = append(metaParts, timeAgo)

	// Media type
	mediaType := strings.ToUpper(item.MediaType[:1]) + item.MediaType[1:]
	metaParts = append(metaParts, mediaType)

	metaLine := metaStyle.Render(strings.Join(metaParts, " • "))
	lines = append(lines, metaLine)

	content := strings.Join(lines, "\n")
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

// LoadHistoryMsg is sent when history is loaded
type LoadHistoryMsg struct {
	History []database.History
	Error   error
}

// loadHistoryCmd creates a command to load history from the database
func loadHistoryCmd(db *gorm.DB) tea.Cmd {
	return func() tea.Msg {
		if db == nil {
			return LoadHistoryMsg{History: []database.History{}, Error: fmt.Errorf("database is nil")}
		}

		var history []database.History
		err := db.Order("watched_at DESC").Find(&history).Error
		return LoadHistoryMsg{History: history, Error: err}
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	if m.db != nil {
		return loadHistoryCmd(m.db)
	}
	return nil
}

// Reset resets the history component to its default state
func (m *Model) Reset() {
	m.mediaTypeFilter = "all" // Always show all media types by default in history view
	m.sortOrder = "recent"    // Default to most recent first
	m.query = ""              // Clear any search query
	m.currentIndex = 0        // Start at first item
}

// IsInputActive returns true if the fuzzy search input is active and not locked
func (m Model) IsInputActive() bool {
	return m.fuzzySearch.IsActive() && !m.fuzzySearch.IsLocked()
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.fuzzySearch.IsActive() {
		if m.fuzzySearch.IsLocked() {
			// Filter is locked - allow action keys for navigating filtered results
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				switch keyMsg.String() {
				case "esc":
					// Clear the filter entirely
					m.fuzzySearch.Deactivate()
					m.query = ""
					m.currentIndex = 0
					return m, nil
				case "/":
					// Unlock to edit filter again
					cmd := m.fuzzySearch.Unlock()
					return m, cmd
				case "up", "k":
					if len(m.GetFilteredHistory()) > 0 && m.currentIndex > 0 {
						m.currentIndex--
					}
					return m, nil
				case "down", "j":
					filtered := m.GetFilteredHistory()
					if len(filtered) > 0 && m.currentIndex < len(filtered)-1 {
						m.currentIndex++
					}
					return m, nil
				case "enter":
					selected := m.GetSelectedHistory()
					if selected != nil {
						return m, func() tea.Msg {
							return common.ResumePlaybackMsg{
								MediaID:         selected.MediaID,
								MediaTitle:      selected.MediaTitle,
								MediaType:       selected.MediaType,
								Episode:         selected.Episode,
								Season:          selected.Season,
								ProgressSeconds: selected.ProgressSeconds,
								ProviderName:    selected.ProviderName,
							}
						}
					}
					return m, nil
				}
			}
		} else {
			// Filter is being edited - typing mode
			// ALL keys except esc go to fuzzy search input (including j/k/up/down for typing)
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				switch keyMsg.String() {
				case "esc":
					// Lock the filter (stop editing)
					m.fuzzySearch.Lock()
					return m, nil
				default:
					// Pass ALL other keys (including j/k/up/down) to fuzzy search input for typing
					cmd = m.fuzzySearch.Update(msg)
					if m.fuzzySearch.Query() != m.query {
						m.query = m.fuzzySearch.Query()
						m.currentIndex = 0
					}
					return m, cmd
				}
			}
		}
	}

	switch msg := msg.(type) {
	case LoadHistoryMsg:
		if msg.Error != nil {
			m.history = []database.History{}
		} else {
			m.history = msg.History
		}
		m.ready = true
		return m, nil

	case DeleteHistoryItemMsg:
		if m.db != nil {
			m.db.Delete(&database.History{}, msg.ID)
		}
		return m, m.Refresh()

	case DeleteAllHistoryMsg:
		if m.db != nil {
			m.db.Where("1 = 1").Delete(&database.History{})
		}
		m.history = []database.History{}
		return m, nil

	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if len(m.GetFilteredHistory()) > 0 && m.currentIndex > 0 {
				m.currentIndex--
			}
		case "down", "j":
			filtered := m.GetFilteredHistory()
			if len(filtered) > 0 && m.currentIndex < len(filtered)-1 {
				m.currentIndex++
			}
		case "enter":
			selected := m.GetSelectedHistory()
			if selected != nil {
				return m, func() tea.Msg {
					return common.ResumePlaybackMsg{
						MediaID:         selected.MediaID,
						MediaTitle:      selected.MediaTitle,
						MediaType:       selected.MediaType,
						Episode:         selected.Episode,
						Season:          selected.Season,
						ProgressSeconds: selected.ProgressSeconds,
						ProviderName:    selected.ProviderName,
					}
				}
			}
		case "1":
			m.FilterByMediaType("all")
		case "2":
			m.FilterByMediaType("anime")
		case "3":
			m.FilterByMediaType("movie")
		case "4":
			m.FilterByMediaType("manga")
		case "r":
			m.SortBy("recent")
		case "t":
			m.SortBy("title")
		case "p":
			m.SortBy("progress")
		case "/":
			return m, m.fuzzySearch.Activate()
		case "q", "esc":
			return m, func() tea.Msg {
				return common.GoToHomeMsg{}
			}
		case "x":
			selected := m.GetSelectedHistory()
			if selected != nil {
				return m, func() tea.Msg {
					return DeleteHistoryItemMsg{ID: selected.ID}
				}
			}
		case "X":
			return m, func() tea.Msg {
				return DeleteAllHistoryMsg{}
			}
		case "w":
			selected := m.GetSelectedHistory()
			if selected != nil {
				return m, func() tea.Msg {
					return common.ShareHistoryViaWatchPartyMsg{
						MediaID:      selected.MediaID,
						MediaTitle:   selected.MediaTitle,
						Episode:      selected.Episode,
						Season:       selected.Season,
						ProviderName: selected.ProviderName,
					}
				}
			}
		}
	}

	return m, cmd
}

// Messages for history operations
type DeleteHistoryItemMsg struct {
	ID uint
}

type DeleteAllHistoryMsg struct{}
