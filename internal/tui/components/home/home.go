package home

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gorm.io/gorm"

	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

type Model struct {
	CurrentMediaType providers.MediaType
	width            int
	height           int
	db               *gorm.DB
	providerName     string // Current provider name for filtering recent history
	recentItems      []RecentItem
	selectedIndex    int  // Index for recent items navigation
	focusOnRecent    bool // Whether focus is on recent items section
	recentLoaded     bool // Whether recent items have been loaded
	displayCount     int  // Number of items currently displayed
}

// RecentHistoryLoadedMsg is sent when recent history is loaded
type RecentHistoryLoadedMsg struct {
	Items []RecentItem
	Error error
}

func New(db *gorm.DB) Model {
	return Model{
		CurrentMediaType: providers.MediaTypeMovieTV, // Default (will be overridden by config)
		db:               db,
		providerName:     "", // Will be set by parent
		selectedIndex:    0,
		focusOnRecent:    false,
		recentLoaded:     false,
	}
}

// SetProvider updates the current provider name for filtering history
func (m *Model) SetProvider(providerName string) {
	m.providerName = providerName
}

func (m *Model) Init() tea.Cmd {
	// Load recent history on init
	return m.loadRecent()
}

// loadRecent loads recent history from the database
func (m *Model) loadRecent() tea.Cmd {
	if m.db == nil {
		return nil
	}

	return func() tea.Msg {
		// Fetch recent history for current media type
		var mediaType string
		switch m.CurrentMediaType {
		case providers.MediaTypeAnime:
			mediaType = "anime"
		case providers.MediaTypeManga:
			mediaType = "manga"
		default:
			mediaType = "movie" // Will include both movie and tv in the query
		}

		// Only fetch items for the current provider to avoid ID conflicts
		// If providerName is empty, it means we haven't initialized properly yet
		if m.providerName == "" {
			return RecentHistoryLoadedMsg{
				Items: []RecentItem{},
				Error: nil,
			}
		}

		// Debug log (simulated since we don't have logger here)
		// fmt.Printf("DEBUG: Fetching recent history for provider: %s\n", m.providerName)

		items, err := FetchRecentHistory(m.db, mediaType, m.providerName, 5)
		return RecentHistoryLoadedMsg{
			Items: items,
			Error: err,
		}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case common.RefreshHistoryMsg:
		// Reload recent history when explicitly requested
		return m, m.loadRecent()

	case RecentHistoryLoadedMsg:
		m.recentLoaded = true
		if msg.Error == nil {
			m.recentItems = msg.Items
			// Recalculate display count
			m.displayCount = m.calculateDisplayCount()
			// If we have recent items, focus on them by default
			if len(m.recentItems) > 0 {
				m.focusOnRecent = true
				m.selectedIndex = 0
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Recalculate display count based on new dimensions
		m.displayCount = m.calculateDisplayCount()
		// Ensure selectedIndex is within bounds
		if m.displayCount > 0 && m.selectedIndex >= m.displayCount {
			m.selectedIndex = m.displayCount - 1
		}
		return m, nil

	case tea.KeyMsg:
		// Navigation keys for recent items
		if m.focusOnRecent && len(m.recentItems) > 0 && m.displayCount > 0 {
			switch msg.String() {
			case "up", "k":
				// Cycle within visible items
				if m.selectedIndex > 0 {
					m.selectedIndex--
				} else {
					// Wrap around to last visible item
					m.selectedIndex = m.displayCount - 1
				}
				return m, nil
			case "down", "j":
				// Cycle within visible items
				if m.selectedIndex < m.displayCount-1 {
					m.selectedIndex++
				} else {
					// Wrap around to first visible item
					m.selectedIndex = 0
				}
				return m, nil
			case "enter":
				// Resume playback of selected recent item
				if m.selectedIndex < len(m.recentItems) {
					item := m.recentItems[m.selectedIndex]
					return m, func() tea.Msg {
						return common.ResumePlaybackMsg{
							MediaID:         item.MediaID,
							MediaTitle:      item.MediaTitle,
							Episode:         item.Episode,
							Season:          item.Season,
							ProgressSeconds: item.ProgressSeconds,
							ProviderName:    item.ProviderName,
						}
					}
				}
				return m, nil
			case "x":
				// Remove from recent (mark as completed in database)
				// TODO: Remove from recent - not implemented in beta
				return m, nil
			case "w":
				// Share selected recent item via WatchParty
				if m.selectedIndex < len(m.recentItems) {
					item := m.recentItems[m.selectedIndex]
					return m, func() tea.Msg {
						return common.ShareRecentViaWatchPartyMsg{
							MediaID:      item.MediaID,
							MediaTitle:   item.MediaTitle,
							Episode:      item.Episode,
							Season:       item.Season,
							ProviderName: item.ProviderName,
						}
					}
				}
				return m, nil
			case "m":
				// Show manga info for selected recent item
				if m.selectedIndex < len(m.recentItems) {
					item := m.recentItems[m.selectedIndex]
					// Only for anime
					if item.MediaType == "anime" {
						return m, func() tea.Msg {
							return common.MangaInfoMsg{
								AnimeTitle: item.MediaTitle,
							}
						}
					}
				}
				return m, nil
			}
		}

		// Global shortcuts (work regardless of focus)
		switch msg.String() {
		case "s":
			return m, func() tea.Msg {
				return common.GoToSearchMsg{}
			}
		case "h":
			// Go to History view
			return m, func() tea.Msg {
				return common.GoToHistoryMsg{MediaType: m.CurrentMediaType}
			}
		case "P":
			// Go to Provider Status view (capital P)
			return m, func() tea.Msg {
				return common.GoToProviderStatusMsg{}
			}
		case "tab":
			// Toggle provider and reload recent history
			return m, func() tea.Msg {
				return common.ToggleProviderMsg{}
			}
		case "l":
			// Go to AniList view (only for Anime/Manga)
			if m.CurrentMediaType != providers.MediaTypeMovieTV {
				return m, func() tea.Msg {
					return common.GoToAniListMsg{}
				}
			}
			return m, nil
		case "d":
			// Go to Downloads view
			return m, func() tea.Msg {
				return common.GoToDownloadsMsg{}
			}
		}
	}
	return m, nil
}

func (m *Model) View() string {
	var output strings.Builder

	// Header with mode and provider - ALWAYS render this
	header := styles.TitleStyle.Render("  greg  ")

	mode := "ANIME"
	modeColor := styles.OxocarbonPurple
	switch m.CurrentMediaType {
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

	providerName := m.providerName
	if providerName == "" {
		providerName = "loading..."
	}
	providerBadge := lipgloss.NewStyle().
		Foreground(styles.OxocarbonBase05).
		Background(styles.OxocarbonBase02).
		Padding(0, 1).
		Render(providerName)

	// Always write header
	headerLine := lipgloss.JoinHorizontal(lipgloss.Center, header, "  ", modeBadge, " ", providerBadge)
	output.WriteString(headerLine)
	output.WriteString("\n\n")

	// Continue Watching section
	if len(m.recentItems) > 0 {
		headerText := "Continue Watching"
		if m.CurrentMediaType == providers.MediaTypeManga {
			headerText = "Continue Reading"
		}
		continueHeader := styles.SubtitleStyle.Render(headerText)
		output.WriteString(continueHeader)
		output.WriteString("\n")

		// Use pre-calculated display count
		displayCount := m.displayCount
		if displayCount == 0 {
			// Fallback if not initialized
			displayCount = m.calculateDisplayCount()
		}

		for i := 0; i < displayCount; i++ {
			item := m.recentItems[i]
			output.WriteString(m.renderRecentItem(item, i == m.selectedIndex && m.focusOnRecent))
			if i < displayCount-1 {
				output.WriteString("\n")
			}
		}

		// Add separator after recent items
		sepWidth := m.calculateSeparatorWidth()
		separator := strings.Repeat("─", sepWidth)
		output.WriteString("\n")
		output.WriteString(styles.HomeSeparatorStyle.Render(separator))
		output.WriteString("\n\n")
	}

	// Quick Actions section
	output.WriteString(styles.SubtitleStyle.Render("Quick Actions"))
	output.WriteString("\n")

	// Core navigation actions with descriptions
	searchDesc := "Find and browse content"
	switch m.CurrentMediaType {
	case providers.MediaTypeAnime:
		searchDesc = "Search for anime series"
	case providers.MediaTypeManga:
		searchDesc = "Search for manga titles"
	}

	output.WriteString(m.renderAction("s", "Search", searchDesc))
	output.WriteString("\n")

	output.WriteString(m.renderAction("h", "History", "View your watch history"))
	output.WriteString("\n")

	// AniList for anime/manga
	if m.CurrentMediaType != providers.MediaTypeMovieTV {
		output.WriteString(m.renderAction("l", "AniList", "Sync and manage your library"))
		output.WriteString("\n")
	}

	output.WriteString(m.renderAction("p", "Switch Providers", "Browse available providers"))
	output.WriteString("\n")

	output.WriteString(m.renderAction("d", "Downloads", "Manage your downloads"))
	output.WriteString("\n")

	// Separator between sections
	sepWidth := m.calculateSeparatorWidth()
	separator := strings.Repeat("─", sepWidth)
	output.WriteString("\n")
	output.WriteString(styles.HomeSeparatorStyle.Render(separator))
	output.WriteString("\n")

	// Mode switching section
	output.WriteString(styles.SubtitleStyle.Render("Modes"))
	output.WriteString("\n")

	output.WriteString(m.renderAction("tab", "Switch Mode", "Cycle: Movies/TV, Anime, Manga"))
	output.WriteString("\n")

	output.WriteString(m.renderAction("1 / 2 / 3", "Quick Switch", "Jump to specific mode"))
	output.WriteString("\n")

	// Footer
	output.WriteString("\n")
	output.WriteString(styles.HomeSeparatorStyle.Render(separator))
	output.WriteString("\n")

	if len(m.recentItems) > 0 {
		// Show different hints based on how many items are displayed vs total
		if m.displayCount > 1 {
			output.WriteString(styles.AniListHelpStyle.Render("↑/↓ navigate  •  enter resume  •  h more history  •  ? help  •  q quit"))
		} else if len(m.recentItems) > 1 {
			output.WriteString(styles.AniListHelpStyle.Render("enter resume  •  h view all history  •  ? help  •  q quit"))
		} else {
			output.WriteString(styles.AniListHelpStyle.Render("enter resume  •  ? help  •  q quit"))
		}
	} else {
		output.WriteString(styles.AniListHelpStyle.Render("? help  •  q quit"))
	}

	return output.String()
}

// calculateSeparatorWidth returns the appropriate separator width
func (m Model) calculateSeparatorWidth() int {
	sepWidth := 60
	if m.width > 4 && m.width-4 < sepWidth {
		sepWidth = m.width - 4
	}
	if sepWidth < 1 {
		sepWidth = 1
	}
	return sepWidth
}

// calculateDisplayCount returns how many recent items should be displayed
func (m Model) calculateDisplayCount() int {
	if len(m.recentItems) == 0 {
		return 0
	}

	displayCount := len(m.recentItems)
	// Limit to 2 items max to ensure header stays visible
	if displayCount > 2 {
		displayCount = 2
	}
	// Show only 1 item on smaller screens
	if m.height > 0 && m.height < 30 {
		displayCount = 1
	}
	return displayCount
}

// renderAction renders a menu action with key, title, and description in a clean format
func (m Model) renderAction(key, title, description string) string {
	// Fixed width for key column (includes brackets)
	keyText := "[" + key + "]"
	keyStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonCyan).
		Bold(true).
		Width(14).
		Align(lipgloss.Left)

	// Fixed width for title column
	titleStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonBase05).
		Bold(true).
		Width(18).
		Align(lipgloss.Left)

	// Description
	descStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonBase03)

	keyPart := keyStyle.Render(keyText)
	titlePart := titleStyle.Render(title)
	descPart := descStyle.Render(description)

	return keyPart + titlePart + descPart
}

// renderRecentItem renders a single recent item
func (m Model) renderRecentItem(item RecentItem, selected bool) string {
	// Title with episode info
	title := FormatEpisodeTitle(item)

	// Progress info
	var progressInfo string
	if item.MediaType == "manga" {
		if item.TotalPages > 0 {
			progressInfo = fmt.Sprintf("Progress: %.0f%% • Page %d/%d • %s",
				item.ProgressPercent,
				item.Page,
				item.TotalPages,
				FormatTimeAgo(item.WatchedAt))
		} else {
			progressInfo = fmt.Sprintf("Progress: %.0f%% • Page %d • %s",
				item.ProgressPercent,
				item.Page,
				FormatTimeAgo(item.WatchedAt))
		}
	} else {
		progressInfo = fmt.Sprintf("Progress: %.0f%% • %s / %s • %s",
			item.ProgressPercent,
			FormatDuration(item.ProgressSeconds),
			FormatDuration(item.TotalSeconds),
			FormatTimeAgo(item.WatchedAt))
	}

	// Style based on selection
	var itemStyle lipgloss.Style
	if selected {
		itemStyle = styles.AniListItemSelectedStyle
	} else {
		itemStyle = styles.AniListItemStyle
	}

	// Title style
	titleStyle := styles.AniListTitleStyle
	if selected {
		titleStyle = titleStyle.Foreground(styles.OxocarbonPurple)
	}

	// Render
	content := titleStyle.Render(title) + "\n" +
		styles.AniListMetadataStyle.Render(progressInfo)

	return itemStyle.Render(content)
}
