package episodes

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

// MangalModel is a mangal-style episodes view
type MangalModel struct {
	episodes      []providers.Episode
	currentIndex  int
	width         int
	height        int
	mediaType     providers.MediaType
	fuzzySearch   *common.FuzzySearch
	selectedItems map[int]bool // For batch selection
	selectionMode bool         // Whether in selection mode
}

func NewMangal() MangalModel {
	return MangalModel{
		episodes:      []providers.Episode{},
		currentIndex:  0,
		mediaType:     providers.MediaTypeMovieTV,
		fuzzySearch:   common.NewFuzzySearch(),
		selectedItems: make(map[int]bool),
		selectionMode: false,
	}
}

func (m *MangalModel) SetEpisodes(episodes []providers.Episode) {
	// Sort episodes by number
	sort.Slice(episodes, func(i, j int) bool {
		return episodes[i].Number < episodes[j].Number
	})
	m.episodes = episodes
	m.currentIndex = 0
	m.selectedItems = make(map[int]bool) // Clear selections when episodes change
}

func (m *MangalModel) SetMediaType(mediaType providers.MediaType) {
	m.mediaType = mediaType
}

// SetCursorToEpisode sets the cursor to the episode with the given episode number
func (m *MangalModel) SetCursorToEpisode(episodeNumber int) {
	for i, ep := range m.episodes {
		if ep.Number == episodeNumber {
			m.currentIndex = i
			return
		}
	}
	// If episode not found, keep current index
}

func (m MangalModel) Init() tea.Cmd {
	return nil
}

func (m MangalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.fuzzySearch.SetWidth(msg.Width)
		return m, nil

	case tea.KeyMsg:
		// If fuzzy search is active, handle it first
		if m.fuzzySearch.IsActive() {
			if m.fuzzySearch.IsLocked() {
				// Filter is locked - allow action keys for navigating filtered results
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
				case "up", "k":
					if m.currentIndex > 0 {
						m.currentIndex--
					}
					if m.currentIndex < 0 {
						m.currentIndex = 0
					}
					return m, nil
				case "down", "j":
					filteredIndices := m.getFilteredIndices()
					maxIndex := len(filteredIndices) - 1
					if m.currentIndex < maxIndex {
						m.currentIndex++
					}
					if m.currentIndex > maxIndex && maxIndex >= 0 {
						m.currentIndex = maxIndex
					}
					return m, nil
				case "enter":
					// Get filtered indices
					filteredIndices := m.getFilteredIndices()
					if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
						actualIndex := filteredIndices[m.currentIndex]
						if actualIndex < len(m.episodes) {
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
					// Download selected episode
					filteredIndices := m.getFilteredIndices()
					if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
						actualIndex := filteredIndices[m.currentIndex]
						if actualIndex < len(m.episodes) {
							selected := m.episodes[actualIndex]
							return m, func() tea.Msg {
								return common.EpisodeDownloadMsg{
									EpisodeID: selected.ID,
									Number:    selected.Number,
									Title:     selected.Title,
								}
							}
						}
					}
					return m, nil
				}
			} else {
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
						if actualIndex < len(m.episodes) {
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
					m.currentIndex = 0
					return m, cmd
				}
			}
		}

		// Normal mode (fuzzy search not active)
		maxIndex := len(m.episodes) - 1

		switch msg.String() {
		case "/":
			// Activate fuzzy search
			cmd := m.fuzzySearch.Activate()
			m.currentIndex = 0
			return m, cmd
		case " ":
			// Toggle selection for current item (for batch download)
			if len(m.episodes) > 0 && m.currentIndex >= 0 && m.currentIndex <= maxIndex {
				if m.selectedItems[m.currentIndex] {
					delete(m.selectedItems, m.currentIndex)
				} else {
					m.selectedItems[m.currentIndex] = true
				}
				m.selectionMode = len(m.selectedItems) > 0

				// Auto-advance to next item after selection
				if m.currentIndex < maxIndex {
					m.currentIndex++
				}
			}
		case "a", "A":
			// Select all items
			if len(m.episodes) > 0 {
				for i := range m.episodes {
					m.selectedItems[i] = true
				}
				m.selectionMode = true
			}
		case "c", "C":
			// Clear all selections
			m.selectedItems = make(map[int]bool)
			m.selectionMode = false
		case "ctrl+a", "tab":
			// Alternate select all
			if len(m.episodes) > 0 {
				for i := range m.episodes {
					m.selectedItems[i] = true
				}
				m.selectionMode = true
			}
		case "backspace":
			// Clear selections
			m.selectedItems = make(map[int]bool)
			m.selectionMode = false
		case "up", "k":
			if m.currentIndex > 0 {
				m.currentIndex--
			}
			// Ensure currentIndex never goes below 0
			if m.currentIndex < 0 {
				m.currentIndex = 0
			}
		case "down", "j":
			if m.currentIndex < maxIndex {
				m.currentIndex++
			}
			// Ensure currentIndex never exceeds maxIndex
			if m.currentIndex > maxIndex {
				m.currentIndex = maxIndex
			}
		case "enter":
			if len(m.episodes) > 0 {
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
			// Download selected episode(s)
			if len(m.selectedItems) > 0 {
				// Batch download - collect all selected episodes in order
				selectedEpisodes := make([]common.EpisodeInfo, 0, len(m.selectedItems))
				// Iterate through episodes in order, not map keys
				for idx, ep := range m.episodes {
					if m.selectedItems[idx] {
						selectedEpisodes = append(selectedEpisodes, common.EpisodeInfo{
							EpisodeID: ep.ID,
							Number:    ep.Number,
							Title:     ep.Title,
						})
					}
				}
				return m, func() tea.Msg {
					return common.BatchDownloadMsg{Episodes: selectedEpisodes}
				}
			} else if len(m.episodes) > 0 {
				// Single download
				selected := m.episodes[m.currentIndex]
				return m, func() tea.Msg {
					return common.EpisodeDownloadMsg{
						EpisodeID: selected.ID,
						Number:    selected.Number,
						Title:     selected.Title,
					}
				}
			}
		case "w":
			// Share selected episode via WatchParty
			if len(m.episodes) > 0 {
				selected := m.episodes[m.currentIndex]
				return m, func() tea.Msg {
					return common.GenerateWatchPartyMsg{
						EpisodeID: selected.ID,
						Number:    selected.Number,
						Title:     selected.Title,
					}
				}
			}
		case "s":
			// Show debug info (source links)
			if len(m.episodes) > 0 {
				selected := m.episodes[m.currentIndex]
				return m, func() tea.Msg {
					return common.GenerateDebugInfoMsg{
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
		}
	}

	return m, nil
}

func (m MangalModel) View() string {
	if len(m.episodes) == 0 {
		msg := "\nNo episodes found.\n\nPress 'esc' to go back."
		if m.mediaType == providers.MediaTypeManga {
			msg = "\nNo chapters found.\n\nPress 'esc' to go back."
		}
		return styles.SubtitleStyle.Render(msg)
	}

	var output string

	// Static header - always at top
	headerText := "CHAPTERS"
	if m.mediaType != providers.MediaTypeManga {
		headerText = "EPISODES"
	}
	output += styles.TitleStyle.Render(headerText) + "\n"

	// Get filtered indices if fuzzy search is active
	filteredIndices := m.getFilteredIndices()
	displayCount := len(filteredIndices)

	count := styles.SubtitleStyle.Render(fmt.Sprintf("%d available", displayCount))
	if m.fuzzySearch.IsActive() && m.fuzzySearch.Query() != "" {
		count += styles.AniListMetadataStyle.Render(fmt.Sprintf(" (filtered from %d)", len(m.episodes)))
	}
	if len(m.selectedItems) > 0 {
		count += styles.AniListMetadataStyle.Render(fmt.Sprintf(" • %d selected", len(m.selectedItems)))
	}
	output += count + "\n"

	// Extra spacing after header
	output += "\n"

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
		output += "\n" + fuzzyView + "\n\n"
	}

	// Scrollable content - render items
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
		isSelected := i == m.currentIndex
		isMarked := m.selectedItems[actualIndex]
		output += m.renderEpisodeItem(episode, isSelected, isMarked) + "\n\n"
	}

	// Help text at bottom
	action := "play"
	if m.mediaType == providers.MediaTypeManga {
		action = "read"
	}
	helpText := fmt.Sprintf("  ↑/↓ nav • enter %s • d dl • space sel • a all • c clear • / filter • esc back", action)
	if m.fuzzySearch.IsActive() {
		helpText = fmt.Sprintf("  ↑/↓ nav • enter %s • d dl • esc clear", action)
	}
	// Add 's' to help text if not manga
	if m.mediaType != providers.MediaTypeManga {
		if m.fuzzySearch.IsActive() {
			helpText = fmt.Sprintf("  ↑/↓ nav • enter %s • d dl • s src • esc clear", action)
		} else {
			if m.mediaType == providers.MediaTypeAnime {
				helpText = fmt.Sprintf("  ↑/↓ nav • enter %s • d dl • s src • m manga • / filter • esc back", action)
			} else {
				helpText = fmt.Sprintf("  ↑/↓ nav • enter %s • d dl • s src • / filter • esc back", action)
			}
		}
	}
	output += "\n" + styles.AniListHelpStyle.Render(helpText)

	return output
}

func (m MangalModel) renderEpisodeItem(episode providers.Episode, selected bool, marked bool) string {
	boxStyle := styles.AniListItemStyle
	titleStyle := styles.AniListTitleStyle
	metaStyle := styles.AniListMetadataStyle

	if selected {
		boxStyle = styles.AniListItemSelectedStyle
		titleStyle = titleStyle.Foreground(styles.OxocarbonPurple)
		metaStyle = metaStyle.Foreground(styles.OxocarbonMauve)
	}

	// Clean title
	cleanedTitle := episode.Title
	re := regexp.MustCompile(`^Eps \d+[:\-\s]*`)
	cleanedTitle = re.ReplaceAllString(cleanedTitle, "")
	cleanedTitle = strings.TrimSpace(cleanedTitle)

	prefix := "Episode"
	if m.mediaType == providers.MediaTypeManga {
		prefix = "Chapter"
	}

	var content string
	// Show selection indicator
	selIndicator := "  "
	if marked {
		selIndicator = "✓ "
	}

	// Always show episode number
	episodeNum := metaStyle.Render(fmt.Sprintf("%s%s %d", selIndicator, prefix, episode.Number))

	// Show title if available, otherwise empty line to maintain height
	var title string
	if cleanedTitle != "" && cleanedTitle != fmt.Sprintf("%s %d", prefix, episode.Number) {
		title = titleStyle.Render(cleanedTitle)
	} else {
		title = " " // Empty line with space to maintain height
	}

	content = episodeNum + "\n" + title

	return boxStyle.Render(content)
}

// getFilteredIndices returns the indices of episodes that match the fuzzy search
func (m MangalModel) getFilteredIndices() []int {
	searchStrings := make([]string, len(m.episodes))
	prefix := "Episode"
	if m.mediaType == providers.MediaTypeManga {
		prefix = "Chapter"
	}
	for i, episode := range m.episodes {
		searchStrings[i] = fmt.Sprintf("%s %d %s", prefix, episode.Number, episode.Title)
	}
	return m.fuzzySearch.Filter(searchStrings)
}

func (m MangalModel) getVisibleRange(total int) (int, int) {
	// Default to showing 8 items if height not set yet
	maxVisible := 8

	if m.height > 0 {
		// Calculate max visible items
		// Each item takes ~3 lines (title + metadata + margin)
		// Overhead: header (2) + count (1) + spacing (1) + help (2) = 6 lines
		// We use a larger safety margin (10) to ensure no scrolling
		itemsSpace := m.height - 10
		if itemsSpace > 0 {
			maxVisible = itemsSpace / 3 // 3 lines per item
		}

		// Ensure at least 1 item visible
		if maxVisible < 1 {
			maxVisible = 1
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
