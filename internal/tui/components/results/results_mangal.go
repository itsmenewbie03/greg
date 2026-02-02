package results

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
	"github.com/justchokingaround/greg/internal/tui/utils"
)

// itemType indicates what type of item we're displaying
type itemType int

const (
	mediaType itemType = iota
	episodeType
)

// MangalModel is a mangal-style results view
type MangalModel struct {
	results             []providers.Media
	episodes            []providers.Episode
	currentIndex        int
	itemType            itemType
	width               int
	height              int
	fuzzySearch         *common.FuzzySearch
	showInfoDialog      bool
	dialogScroll        int // Scroll offset for info dialog
	showMangaInfo       bool
	providerName        string
	isProviderSelection bool // True when showing provider selection
}

func NewMangal() MangalModel {
	return MangalModel{
		results:       []providers.Media{},
		episodes:      []providers.Episode{},
		currentIndex:  0,
		itemType:      mediaType,
		fuzzySearch:   common.NewFuzzySearch(),
		showMangaInfo: true, // Default to true, disable for provider selection
		providerName:  "",
	}
}

func (m *MangalModel) SetMediaResults(results []providers.Media) {
	m.results = results
	m.episodes = []providers.Episode{}
	m.itemType = mediaType
	m.currentIndex = 0
}

func (m *MangalModel) SetEpisodeResults(episodes []providers.Episode) {
	m.results = []providers.Media{}
	m.episodes = episodes
	m.itemType = episodeType
	m.currentIndex = 0
}

func (m *MangalModel) UpdateMediaItem(index int, media providers.Media) {
	if index >= 0 && index < len(m.results) {
		// Merge details instead of overwriting to preserve existing data
		existing := m.results[index]

		// Only update fields that are present in the new media object
		if media.Title != "" {
			existing.Title = media.Title
		}
		if media.Synopsis != "" {
			existing.Synopsis = media.Synopsis
		}
		if media.Year > 0 {
			existing.Year = media.Year
		}
		if media.Rating > 0 {
			existing.Rating = media.Rating
		}
		if media.Status != "" {
			existing.Status = media.Status
		}
		if len(media.Genres) > 0 {
			existing.Genres = media.Genres
		}
		if media.TotalEpisodes > 0 {
			existing.TotalEpisodes = media.TotalEpisodes
		}
		if media.PosterURL != "" {
			existing.PosterURL = media.PosterURL
		}

		m.results[index] = existing
	}
}

func (m MangalModel) GetMediaResults() []providers.Media {
	return m.results
}

func (m MangalModel) GetSelectedIndex() int {
	return m.currentIndex
}

func (m MangalModel) GetItems() []providers.Media {
	return m.results
}

func (m MangalModel) GetSelectedMedia() *providers.Media {
	if m.itemType != mediaType {
		return nil
	}

	filteredIndices := m.getFilteredIndices()
	if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
		actualIndex := filteredIndices[m.currentIndex]
		if actualIndex < len(m.results) {
			return &m.results[actualIndex]
		}
	}
	return nil
}

func (m MangalModel) Init() tea.Cmd {
	return nil
}

// checkDetailsNeeded checks if visible items need details fetched
func (m *MangalModel) checkDetailsNeeded() tea.Cmd {
	if m.itemType != mediaType || len(m.results) == 0 {
		return nil
	}

	// Get filtered indices
	filteredIndices := m.getFilteredIndices()
	displayCount := len(filteredIndices)
	if displayCount == 0 {
		return nil
	}

	// Get visible range
	start, end := m.getVisibleRange(displayCount)

	var cmds []tea.Cmd

	// Check all visible items
	for i := start; i < end; i++ {
		if i >= len(filteredIndices) {
			break
		}
		actualIndex := filteredIndices[i]
		if actualIndex >= len(m.results) {
			continue
		}

		media := m.results[actualIndex]
		if media.Synopsis == "" {
			// Capture index for closure
			idx := actualIndex
			mid := media.ID
			cmds = append(cmds, func() tea.Msg {
				return common.RequestDetailsMsg{
					MediaID: mid,
					Index:   idx,
				}
			})
		}
	}

	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}

	return nil
}

func (m MangalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.fuzzySearch.SetWidth(msg.Width)
		// Check if visible items need details when resized
		cmds = append(cmds, m.checkDetailsNeeded())
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		// If info dialog is open, handle it first
		if m.showInfoDialog {
			switch msg.String() {
			case "esc", "i", "enter":
				m.showInfoDialog = false
				m.dialogScroll = 0 // Reset scroll
				return m, nil
			case "up", "k":
				if m.dialogScroll > 0 {
					m.dialogScroll--
				}
				return m, nil
			case "down", "j":
				// Scrolling down is handled in renderInfoDialog with max scroll
				m.dialogScroll++
				return m, nil
			}
			return m, nil
		}

		// If fuzzy search is active, handle it first
		if m.fuzzySearch.IsActive() {
			if m.fuzzySearch.IsLocked() {
				// Filter is locked - allow action keys
				switch msg.String() {
				case "esc":
					// Clear the filter entirely
					m.fuzzySearch.Deactivate()
					m.currentIndex = 0
					return m, nil
				case "/":
					// Unlock to edit filter again
					cmd := m.fuzzySearch.Unlock()
					return m, cmd
				case "i":
					// Show info dialog
					if m.itemType == mediaType && len(m.results) > 0 {
						m.showInfoDialog = true
						m.dialogScroll = 0
					}
					return m, nil
				case "up", "k":
					if m.currentIndex > 0 {
						m.currentIndex--
					}
					if m.currentIndex < 0 {
						m.currentIndex = 0
					}
					cmds = append(cmds, m.checkDetailsNeeded())
					return m, tea.Batch(cmds...)
				case "down", "j":
					filteredIndices := m.getFilteredIndices()
					maxIndex := len(filteredIndices) - 1
					if m.currentIndex < maxIndex {
						m.currentIndex++
					}
					if m.currentIndex > maxIndex && maxIndex >= 0 {
						m.currentIndex = maxIndex
					}
					cmds = append(cmds, m.checkDetailsNeeded())
					return m, tea.Batch(cmds...)
				case "enter":
					// Get filtered indices
					filteredIndices := m.getFilteredIndices()
					if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
						actualIndex := filteredIndices[m.currentIndex]
						if m.itemType == mediaType && actualIndex < len(m.results) {
							selected := m.results[actualIndex]
							return m, func() tea.Msg {
								return common.MediaSelectedMsg{
									MediaID: selected.ID,
									Title:   selected.Title,
									Type:    string(selected.Type),
								}
							}
						} else if m.itemType == episodeType && actualIndex < len(m.episodes) {
							selected := m.episodes[actualIndex]
							return m, func() tea.Msg {
								return common.EpisodeSelectedMsg{
									EpisodeID: selected.ID,
									Number:    selected.Number,
									Title:     selected.Title,
								}
							}
						}
					}
					return m, nil
				case "d":
					// Download selected media (if movie or single episode anime)
					filteredIndices := m.getFilteredIndices()
					if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
						actualIndex := filteredIndices[m.currentIndex]
						if m.itemType == mediaType && actualIndex < len(m.results) {
							selected := m.results[actualIndex]
							// Allow direct download for movies or single-episode anime
							if selected.Type == providers.MediaTypeMovie || (selected.Type == providers.MediaTypeAnime && selected.TotalEpisodes == 1) {
								return m, func() tea.Msg {
									return common.MediaDownloadMsg{
										MediaID: selected.ID,
										Title:   selected.Title,
										Type:    string(selected.Type),
									}
								}
							}
						}
					}
					return m, nil
				case "m":
					// Show manga info for selected media
					filteredIndices := m.getFilteredIndices()
					if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
						actualIndex := filteredIndices[m.currentIndex]
						if m.itemType == mediaType && actualIndex < len(m.results) {
							selected := m.results[actualIndex]
							if selected.Type == providers.MediaTypeAnime {
								return m, func() tea.Msg {
									return common.MangaInfoMsg{
										AnimeTitle: selected.Title,
									}
								}
							}
						}
					}
					return m, nil
				}
				return m, nil
			}

			// Filter is being edited - typing mode
			// ALL keys except esc/enter go to fuzzy search input (including j/k/up/down for typing)
			switch msg.String() {
			case "esc":
				// Lock the filter (stop editing)
				m.fuzzySearch.Lock()
				return m, nil
			case "enter":
				// Get filtered indices
				filteredIndices := m.getFilteredIndices()
				if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
					actualIndex := filteredIndices[m.currentIndex]
					if m.itemType == mediaType && actualIndex < len(m.results) {
						selected := m.results[actualIndex]
						return m, func() tea.Msg {
							return common.MediaSelectedMsg{
								MediaID: selected.ID,
								Title:   selected.Title,
								Type:    string(selected.Type),
							}
						}
					} else if m.itemType == episodeType && actualIndex < len(m.episodes) {
						selected := m.episodes[actualIndex]
						return m, func() tea.Msg {
							return common.EpisodeSelectedMsg{
								EpisodeID: selected.ID,
								Number:    selected.Number,
								Title:     selected.Title,
							}
						}
					}
				}
				return m, nil
			default:
				// Pass ALL other keys (including j/k/up/down) to fuzzy search input for typing
				cmd := m.fuzzySearch.Update(msg)
				// Reset currentIndex when query changes
				m.currentIndex = 0
				return m, cmd
			}
		}

		// Normal mode (fuzzy search not active)
		maxIndex := 0
		if m.itemType == mediaType {
			maxIndex = len(m.results) - 1
		} else {
			maxIndex = len(m.episodes) - 1
		}

		switch msg.String() {
		case "i":
			// Show info dialog for current selection
			if (m.itemType == mediaType && len(m.results) > 0) ||
				(m.itemType == episodeType && len(m.episodes) > 0) {
				m.showInfoDialog = true
				m.dialogScroll = 0 // Reset scroll when opening
			}
			return m, nil
		case "/":
			// Activate fuzzy search
			cmd := m.fuzzySearch.Activate()
			m.currentIndex = 0
			return m, cmd
		case "up", "k":
			if m.currentIndex > 0 {
				m.currentIndex--
			}
			// Ensure currentIndex never goes below 0
			if m.currentIndex < 0 {
				m.currentIndex = 0
			}
			cmds = append(cmds, m.checkDetailsNeeded())
		case "down", "j":
			if m.currentIndex < maxIndex {
				m.currentIndex++
			}
			// Ensure currentIndex never exceeds maxIndex
			if m.currentIndex > maxIndex {
				m.currentIndex = maxIndex
			}
			cmds = append(cmds, m.checkDetailsNeeded())
		case "enter":
			if m.itemType == mediaType && len(m.results) > 0 {
				selected := m.results[m.currentIndex]
				return m, func() tea.Msg {
					return common.MediaSelectedMsg{
						MediaID: selected.ID,
						Title:   selected.Title,
						Type:    string(selected.Type),
					}
				}
			} else if m.itemType == episodeType && len(m.episodes) > 0 {
				selected := m.episodes[m.currentIndex]
				return m, func() tea.Msg {
					return common.EpisodeSelectedMsg{
						EpisodeID: selected.ID,
						Number:    selected.Number,
						Title:     selected.Title,
					}
				}
			}
		case "d":
			// Download selected media (if movie or single episode anime)
			if m.itemType == mediaType && len(m.results) > 0 {
				selected := m.results[m.currentIndex]
				// Allow direct download for movies or single-episode anime
				if selected.Type == providers.MediaTypeMovie || (selected.Type == providers.MediaTypeAnime && selected.TotalEpisodes == 1) {
					return m, func() tea.Msg {
						return common.MediaDownloadMsg{
							MediaID: selected.ID,
							Title:   selected.Title,
							Type:    string(selected.Type),
						}
					}
				}
			} else if m.itemType == episodeType && len(m.episodes) > 0 {
				// Existing episode download logic
				selected := m.episodes[m.currentIndex]
				return m, func() tea.Msg {
					return common.EpisodeDownloadMsg{
						EpisodeID: selected.ID,
						Number:    selected.Number,
						Title:     selected.Title,
					}
				}
			}
		case "esc":
			return m, func() tea.Msg {
				return common.BackMsg{}
			}
		case "q":
			return m, tea.Quit
		case "w":
			// Share media item via WatchParty
			if m.itemType == mediaType && len(m.results) > 0 {
				selected := m.results[m.currentIndex]
				return m, func() tea.Msg {
					return common.ShareMediaViaWatchPartyMsg{
						MediaID: selected.ID,
						Title:   selected.Title,
						Type:    string(selected.Type),
					}
				}
			}
		case "s":
			// Show debug info (source links)
			if m.itemType == mediaType && len(m.results) > 0 {
				selected := m.results[m.currentIndex]
				return m, func() tea.Msg {
					return common.GenerateMediaDebugInfoMsg{
						MediaID: selected.ID,
						Title:   selected.Title,
						Type:    string(selected.Type),
					}
				}
			}
		case "m":
			// Show manga info for selected media
			if m.itemType == mediaType && len(m.results) > 0 {
				selected := m.results[m.currentIndex]
				if selected.Type == providers.MediaTypeAnime {
					return m, func() tea.Msg {
						return common.MangaInfoMsg{
							AnimeTitle: selected.Title,
						}
					}
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m MangalModel) View() string {
	var baseView string
	if m.itemType == mediaType {
		baseView = m.renderMediaResults()
	} else {
		baseView = m.renderEpisodeResults()
	}

	// Overlay info dialog if shown
	if m.showInfoDialog {
		if m.itemType == mediaType && len(m.results) > 0 {
			// Get the actual index accounting for fuzzy search filter
			filteredIndices := m.getFilteredIndices()
			if m.currentIndex < len(filteredIndices) {
				actualIndex := filteredIndices[m.currentIndex]
				if actualIndex < len(m.results) {
					dialog := m.renderInfoDialog(m.results[actualIndex])
					return lipgloss.Place(m.width, m.height,
						lipgloss.Center, lipgloss.Center,
						dialog,
						lipgloss.WithWhitespaceChars(" "),
						lipgloss.WithWhitespaceForeground(lipgloss.Color("#161616")))
				}
			}
		}
	}

	return baseView
}

func (m MangalModel) renderMediaResults() string {
	if len(m.results) == 0 {
		return styles.SubtitleStyle.Render("\nNo results found.\n\nPress 'esc' to go back.")
	}

	// Build main content (header + items)
	var content strings.Builder

	// Top padding
	content.WriteString("\n\n")

	// Header with count - clean and readable
	header := styles.TitleStyle.Render("  RESULTS  ")

	// Add provider badge if available
	if m.providerName != "" {
		providerBadge := lipgloss.NewStyle().
			Foreground(styles.OxocarbonBase05).
			Background(styles.OxocarbonBase02).
			Padding(0, 1).
			Render(m.providerName)
		header = lipgloss.JoinHorizontal(lipgloss.Center, header, " ", providerBadge)
	}

	content.WriteString(header + "\n")

	// Get filtered indices if fuzzy search is active
	filteredIndices := m.getFilteredIndices()
	displayCount := len(filteredIndices)

	count := styles.SubtitleStyle.Render(fmt.Sprintf("  %d found", displayCount))
	if m.fuzzySearch.IsActive() && m.fuzzySearch.Query() != "" {
		count += styles.AniListMetadataStyle.Render(fmt.Sprintf(" (filtered from %d)", len(m.results)))
	}
	content.WriteString(count + "\n")

	// Show fuzzy search input if active
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

	// Render items with borders
	visibleStart, visibleEnd := m.getVisibleRange(displayCount)

	for i := visibleStart; i < visibleEnd; i++ {
		if i >= len(filteredIndices) {
			break
		}
		actualIndex := filteredIndices[i]
		if actualIndex >= len(m.results) {
			break
		}
		media := m.results[actualIndex]
		content.WriteString(m.renderMediaItem(media, i == m.currentIndex) + "\n\n")
	}

	// Build help text (will be positioned at bottom) - concise version
	var helpText string

	// Provider selection has minimal help text
	if m.isProviderSelection {
		if m.fuzzySearch.IsActive() && !m.fuzzySearch.IsLocked() {
			helpText = "  Type to filter • esc lock"
		} else {
			helpText = "  ↑/↓ nav • enter select • esc back"
		}
	} else {
		// Normal results view
		helpText = "  ↑/↓ nav • enter select • s src • i info • / filter • esc back"

		// Add manga info if enabled and it's anime
		isAnime := false
		if len(m.results) > 0 && m.results[0].Type == providers.MediaTypeAnime {
			isAnime = true
		}

		if m.showMangaInfo && isAnime {
			helpText = "  ↑/↓ nav • enter select • s src • i info • m manga • / filter • esc back"
		}

		if m.fuzzySearch.IsActive() {
			if m.fuzzySearch.IsLocked() {
				if m.showMangaInfo && isAnime {
					helpText = "  ↑/↓ nav • enter select • s src • i info • m manga • / edit • esc clear"
				} else {
					helpText = "  ↑/↓ nav • enter select • s src • i info • / edit • esc clear"
				}
			} else {
				helpText = "  Type to filter • ↑/↓ nav • esc lock"
			}
		}

		// Add download hint if current item is a movie
		isMovie := false
		if len(m.results) > 0 {
			// Get current item
			var currentItem providers.Media
			if m.fuzzySearch.IsActive() {
				filteredIndices := m.getFilteredIndices()
				if m.currentIndex < len(filteredIndices) {
					actualIndex := filteredIndices[m.currentIndex]
					if actualIndex < len(m.results) {
						currentItem = m.results[actualIndex]
					}
				}
			} else if m.currentIndex < len(m.results) {
				currentItem = m.results[m.currentIndex]
			}

			if currentItem.Type == providers.MediaTypeMovie || (currentItem.Type == providers.MediaTypeAnime && currentItem.TotalEpisodes == 1) {
				isMovie = true
			}
		}

		if isMovie {
			if m.fuzzySearch.IsActive() && !m.fuzzySearch.IsLocked() {
				// Don't show 'd' hint when typing
			} else {
				// Insert 'd download' into help text
				helpText = strings.Replace(helpText, "enter select", "enter select • d dl", 1)
			}
		}
	}

	styledHelpText := styles.AniListHelpStyle.Render(helpText)

	// If height is available, use fixed layout with help at bottom
	if m.height > 0 {
		// Calculate content height (everything except help text)
		contentLines := strings.Split(content.String(), "\n")
		contentHeight := len(contentLines)

		// Reserve 2 lines at bottom for help (1 for text, 1 for padding)
		availableContentHeight := m.height - 2

		// If content fits, pad with empty lines to push help to bottom
		if contentHeight < availableContentHeight {
			padding := strings.Repeat("\n", availableContentHeight-contentHeight)
			return content.String() + padding + "\n" + styledHelpText
		}
	}

	// Fallback: just append help text
	return content.String() + "\n" + styledHelpText
}

func (m MangalModel) renderEpisodeResults() string {
	if len(m.episodes) == 0 {
		return styles.SubtitleStyle.Render("\nNo episodes found.\n\nPress 'esc' to go back.")
	}

	// Build main content (header + items)
	var content strings.Builder

	// Top padding
	content.WriteString("\n")

	// Header with better styling
	header := styles.TitleStyle.Render("  EPISODES  ")

	// Add provider badge if available
	if m.providerName != "" {
		providerBadge := lipgloss.NewStyle().
			Foreground(styles.OxocarbonBase05).
			Background(styles.OxocarbonBase02).
			Padding(0, 1).
			Render(m.providerName)
		header = lipgloss.JoinHorizontal(lipgloss.Center, header, " ", providerBadge)
	}

	content.WriteString(header + "\n")

	// Get filtered indices if fuzzy search is active
	filteredIndices := m.getFilteredIndices()
	displayCount := len(filteredIndices)

	count := styles.SubtitleStyle.Render(fmt.Sprintf("  %d available", displayCount))
	if m.fuzzySearch.IsActive() && m.fuzzySearch.Query() != "" {
		count += styles.AniListMetadataStyle.Render(fmt.Sprintf(" (filtered from %d)", len(m.episodes)))
	}
	content.WriteString(count + "\n")

	// Show fuzzy search input if active
	if m.fuzzySearch.IsActive() {
		content.WriteString("\n" + m.fuzzySearch.View() + "\n\n")
	} else {
		content.WriteString("\n")
	}

	// Render items
	visibleStart, visibleEnd := m.getVisibleRange(displayCount)

	for i := visibleStart; i < visibleEnd; i++ {
		if i >= len(filteredIndices) {
			break
		}
		actualIndex := filteredIndices[i]
		if actualIndex >= len(m.episodes) {
			break
		}
		episode := m.episodes[actualIndex]
		content.WriteString(m.renderEpisodeItem(episode, i == m.currentIndex) + "\n\n")
	}

	// Build help text (will be positioned at bottom)
	helpText := "  ↑/↓ navigate • enter play • w share • i info • / filter • P provider • ? help • esc back"

	// Add manga info if enabled (assuming anime if we are here, but let's check context if possible)
	// For episodes, we don't have easy access to media type in MangalModel unless we store it.
	// But results component is mostly used for media selection.
	// If used for episodes, it's likely via SetEpisodeResults.
	// Let's assume if showMangaInfo is true, we want it.
	if m.showMangaInfo {
		helpText = "  ↑/↓ navigate • enter play • w share • i info • m manga info • / filter • P provider • ? help • esc back"
	}

	if m.fuzzySearch.IsActive() {
		if m.fuzzySearch.IsLocked() {
			if m.showMangaInfo {
				helpText = "  ↑/↓ navigate • enter play • w share • i info • m manga info • / edit • ? help • esc clear"
			} else {
				helpText = "  ↑/↓ navigate • enter play • w share • i info • / edit • ? help • esc clear"
			}
		} else {
			helpText = "  Type to filter • ↑/↓ navigate • w share • ? help • esc lock filter"
		}
	}
	styledHelpText := styles.AniListHelpStyle.Render(helpText)

	// If height is available, use fixed layout with help at bottom
	if m.height > 0 {
		// Calculate content height (everything except help text)
		contentLines := strings.Split(content.String(), "\n")
		contentHeight := len(contentLines)

		// Reserve 2 lines at bottom for help (1 for text, 1 for padding)
		availableContentHeight := m.height - 2

		// If content fits, pad with empty lines to push help to bottom
		if contentHeight < availableContentHeight {
			padding := strings.Repeat("\n", availableContentHeight-contentHeight)
			return content.String() + padding + "\n" + styledHelpText
		}
	}

	// Fallback: just append help text
	return content.String() + "\n" + styledHelpText
}

func (m MangalModel) renderMediaItem(media providers.Media, selected bool) string {
	boxStyle := styles.AniListItemStyle
	titleStyle := styles.AniListTitleStyle
	metaStyle := styles.AniListMetadataStyle
	synopsisStyle := styles.SynopsisStyle

	if selected {
		boxStyle = styles.AniListItemSelectedStyle
		titleStyle = titleStyle.Foreground(styles.OxocarbonPurple)
		metaStyle = metaStyle.Foreground(styles.OxocarbonMauve)
	}

	var lines []string

	// Line 1: Title (always present)
	lines = append(lines, titleStyle.Render(media.Title))

	// Line 2: Metadata (Year • Type • Rating • Status • Episodes)
	var metaParts []string
	if media.Year > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d", media.Year))
	}
	if media.Type != "" {
		metaParts = append(metaParts, string(media.Type))
	}
	if media.Rating > 0 {
		metaParts = append(metaParts, fmt.Sprintf("★ %.1f", media.Rating))
	}
	if media.Status != "" && media.Status != "Unknown" {
		metaParts = append(metaParts, media.Status)
	}
	if media.TotalEpisodes > 0 {
		label := "episodes"
		if media.Type == providers.MediaTypeManga {
			label = "chapters"
		}
		metaParts = append(metaParts, fmt.Sprintf("%d %s", media.TotalEpisodes, label))
	}

	if len(metaParts) > 0 {
		meta := strings.Join(metaParts, " • ")
		lines = append(lines, metaStyle.Render(meta))
	}

	// Lines 3-4: Synopsis (max 2 lines) - always present for consistency
	availableWidth := m.width - 10 // Account for borders
	if availableWidth < 40 {
		availableWidth = 40
	}
	if availableWidth > 100 {
		availableWidth = 100
	}

	synopsis := media.Synopsis
	if synopsis == "" {
		synopsis = "Press 'i' to see more info"
	}

	truncatedSynopsis := utils.TruncateToLines(synopsis, 2, availableWidth)
	// Ensure we always have 2 lines for synopsis to keep height consistent
	synopsisLines := strings.Split(truncatedSynopsis, "\n")
	if len(synopsisLines) < 2 {
		for i := len(synopsisLines); i < 2; i++ {
			synopsisLines = append(synopsisLines, " ")
		}
		truncatedSynopsis = strings.Join(synopsisLines, "\n")
	}
	lines = append(lines, synopsisStyle.Render(truncatedSynopsis))

	// Line 5: Genres - always present for consistency
	maxGenres := 5
	if m.width < 80 {
		maxGenres = 3 // Fewer for narrow terminals
	}

	if len(media.Genres) > 0 {
		genres := RenderGenres(media.Genres, selected, maxGenres)
		lines = append(lines, genres)
	} else {
		// Empty line for genres to maintain height
		lines = append(lines, " ")
	}

	// Join all lines with single newline, no trailing newline
	content := strings.Join(lines, "\n")
	return boxStyle.Render(content)
}

func (m MangalModel) renderEpisodeItem(episode providers.Episode, selected bool) string {
	boxStyle := styles.AniListItemStyle
	titleStyle := styles.AniListTitleStyle
	metaStyle := styles.AniListMetadataStyle

	if selected {
		boxStyle = styles.AniListItemSelectedStyle
		titleStyle = titleStyle.Foreground(styles.OxocarbonPurple)
		metaStyle = metaStyle.Foreground(styles.OxocarbonMauve)
	}

	// Always use 2 lines for consistency
	episodeNum := metaStyle.Render(fmt.Sprintf("Episode %d", episode.Number))

	// Show title if it's different from "Episode N", otherwise show original title or empty line
	titleText := episode.Title
	if titleText == "" || titleText == fmt.Sprintf("Episode %d", episode.Number) {
		// If title is just "Episode N", try to use a better title or leave empty
		if episode.Number == 1 {
			// For movies/single episodes, the title might be the media title
			// But here we just want to ensure we have a second line
			titleText = " "
		} else {
			titleText = " "
		}
	}

	title := titleStyle.Render(titleText)
	content := episodeNum + "\n" + title

	return boxStyle.Render(content)
}

// getFilteredIndices returns the indices of items that match the fuzzy search
func (m MangalModel) getFilteredIndices() []int {
	var searchStrings []string

	if m.itemType == mediaType {
		searchStrings = make([]string, len(m.results))
		for i, media := range m.results {
			searchStrings[i] = media.Title
		}
	} else {
		searchStrings = make([]string, len(m.episodes))
		for i, episode := range m.episodes {
			searchStrings[i] = fmt.Sprintf("Episode %d %s", episode.Number, episode.Title)
		}
	}

	return m.fuzzySearch.Filter(searchStrings)
}

func (m MangalModel) getVisibleRange(total int) (int, int) {
	// Default to showing 3 items if height not set yet
	maxVisible := 3

	if m.height > 0 {
		// Calculate max visible items to ensure header stays visible
		// Fixed overhead:
		// - Top padding: 2 lines
		// - Header: 1 line
		// - Count: 1 line
		// - Blank line: 1 line
		// - Bottom blank + help: 2 lines
		// Total overhead: ~7 lines
		overhead := 7
		if m.fuzzySearch.IsActive() {
			overhead = 10 // Fuzzy search takes more space
		}

		itemsSpace := m.height - overhead
		if itemsSpace > 0 {
			// Dynamically calculate based on content complexity
			// Each item typically takes: title (1) + metadata (1) + synopsis (2) + genres (1) + borders/spacing (1) = ~6 lines
			// Use a conservative estimate to maximize space usage
			linesPerItem := 6
			// Allow more items if space permits (user requested more than 3)
			maxVisible = itemsSpace / linesPerItem
		}

		// Ensure at least 1 item visible
		if maxVisible < 1 {
			maxVisible = 1
		}

		// Remove the hard cap to use all available vertical space
		// Only enforce a reasonable maximum for very tall terminals
		if maxVisible > 20 {
			maxVisible = 20
		}
	}

	if total <= maxVisible {
		return 0, total
	}

	// Keep selection centered
	start := 0
	if m.currentIndex > maxVisible/2 {
		start = m.currentIndex - maxVisible/2
	}

	// Ensure start is valid
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

// renderInfoDialog renders a detailed information dialog for a media item
func (m MangalModel) renderInfoDialog(media providers.Media) string {
	// Calculate dialog dimensions first
	dialogWidth := 80
	if m.width > 0 {
		if m.width < 90 {
			dialogWidth = m.width - 4 // Smaller margin for small screens
		}
		// Ensure minimum width but don't exceed screen width
		if dialogWidth < 40 {
			dialogWidth = 40
		}
		if dialogWidth > m.width {
			dialogWidth = m.width - 2 // Fallback to almost full width
		}
	}

	// Calculate content width (dialog width minus border and padding)
	// Border: 2 chars (left + right), Padding(1,2): 4 chars horizontal
	contentWidth := dialogWidth - 6

	// Calculate max synopsis lines based on terminal height
	// Fixed overhead: Title (3 lines) + Metadata (~6 lines) + Genres (3 lines) + Help (3 lines) + Padding (4 lines) + Scroll indicators (2 lines)
	// Total overhead: ~21 lines
	maxSynopsisLines := 10 // Default
	if m.height > 0 {
		overhead := 21
		availableLines := m.height - overhead
		if availableLines > 5 {
			maxSynopsisLines = availableLines
		} else {
			maxSynopsisLines = 5 // Minimum
		}
		// Cap at reasonable max
		if maxSynopsisLines > 20 {
			maxSynopsisLines = 20
		}
	}

	var output string

	// Title
	title := styles.AniListHeaderStyle.Render("Media Information")
	output += title + "\n\n"

	// Media title
	output += styles.AniListTitleStyle.Render(media.Title) + "\n\n"

	// Metadata
	var metaParts []string
	if media.Year > 0 {
		metaParts = append(metaParts, fmt.Sprintf("Year: %d", media.Year))
	}
	if media.Type != "" {
		metaParts = append(metaParts, fmt.Sprintf("Type: %s", string(media.Type)))
	}
	if media.Rating > 0 {
		metaParts = append(metaParts, fmt.Sprintf("Rating: ★ %.1f", media.Rating))
	}
	if media.Status != "" && media.Status != "Unknown" {
		metaParts = append(metaParts, fmt.Sprintf("Status: %s", media.Status))
	}
	if media.TotalEpisodes > 0 {
		label := "Episodes"
		if media.Type == providers.MediaTypeManga {
			label = "Chapters"
		}
		metaParts = append(metaParts, fmt.Sprintf("%s: %d", label, media.TotalEpisodes))
	}

	for _, part := range metaParts {
		output += styles.AniListMetadataStyle.Render(part) + "\n"
	}
	output += "\n"

	// Synopsis with scrolling (fixed viewport)
	if media.Synopsis != "" {
		output += styles.AniListTitleStyle.Render("Synopsis:") + "\n"
		// Wrap synopsis to fit within the content width
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

		// Always reserve space for top indicator (show indicator or empty line)
		if canScrollUp {
			output += styles.AniListMetadataStyle.Render("  ▲ scroll up") + "\n"
		} else {
			output += "\n" // Empty line to maintain layout
		}

		// Get visible lines based on scroll
		start := scrollOffset
		end := start + maxSynopsisLines
		if end > len(wrappedSynopsis) {
			end = len(wrappedSynopsis)
		}

		visibleLines := wrappedSynopsis[start:end]

		// Render visible synopsis lines (always fill to maxSynopsisLines to keep height fixed)
		for i := 0; i < maxSynopsisLines; i++ {
			if i < len(visibleLines) {
				output += styles.SynopsisStyle.Render(visibleLines[i]) + "\n"
			} else {
				output += "\n" // Empty line to maintain fixed height
			}
		}

		// Always reserve space for bottom indicator (show indicator or empty line)
		if canScrollDown {
			output += styles.AniListMetadataStyle.Render("  ▼ scroll down") + "\n"
		} else {
			output += "\n" // Empty line to maintain layout
		}
		output += "\n"
	}

	// Genres
	if len(media.Genres) > 0 {
		output += styles.AniListTitleStyle.Render("Genres:") + "\n"
		genreLine := RenderGenres(media.Genres, false, 10) // Show up to 10 genres
		output += genreLine + "\n\n"
	}

	output += styles.AniListHelpStyle.Render("↑/↓ scroll • i/enter/esc close • ? help")

	// Box it with calculated width
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OxocarbonPurple).
		Padding(1, 2).
		Width(dialogWidth)

	return boxStyle.Render(output)
}
