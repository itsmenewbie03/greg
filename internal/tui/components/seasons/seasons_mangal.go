package seasons

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

// MangalModel is a mangal-style seasons view
type MangalModel struct {
	seasons      []providers.Season
	currentIndex int
	width        int
	height       int
	mediaType    providers.MediaType
	fuzzySearch  *common.FuzzySearch
}

func NewMangal() MangalModel {
	return MangalModel{
		seasons:      []providers.Season{},
		currentIndex: 0,
		mediaType:    providers.MediaTypeMovieTV,
		fuzzySearch:  common.NewFuzzySearch(),
	}
}

func (m *MangalModel) SetSeasons(seasons []providers.Season) {
	m.seasons = seasons
	m.currentIndex = 0
}

func (m *MangalModel) SetMediaType(mediaType providers.MediaType) {
	m.mediaType = mediaType
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
						if actualIndex < len(m.seasons) {
							selected := m.seasons[actualIndex]
							return m, func() tea.Msg {
								return common.SeasonSelectedMsg{
									SeasonID: selected.ID,
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
						if actualIndex < len(m.seasons) {
							selected := m.seasons[actualIndex]
							return m, func() tea.Msg {
								return common.SeasonSelectedMsg{
									SeasonID: selected.ID,
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
		maxIndex := len(m.seasons) - 1

		switch msg.String() {
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
		case "down", "j":
			if m.currentIndex < maxIndex {
				m.currentIndex++
			}
			// Ensure currentIndex never exceeds maxIndex
			if m.currentIndex > maxIndex {
				m.currentIndex = maxIndex
			}
		case "enter":
			if len(m.seasons) > 0 {
				selected := m.seasons[m.currentIndex]
				return m, func() tea.Msg {
					return common.SeasonSelectedMsg{
						SeasonID: selected.ID,
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
	if len(m.seasons) == 0 {
		msg := "\nNo seasons found.\n\nPress 'esc' to go back."
		if m.mediaType == providers.MediaTypeManga {
			msg = "\nNo volumes found.\n\nPress 'esc' to go back."
		}
		return styles.SubtitleStyle.Render(msg)
	}

	var output string

	// Static header - always at top
	headerText := "SEASONS"
	if m.mediaType == providers.MediaTypeManga {
		headerText = "VOLUMES"
	}
	output += styles.TitleStyle.Render(headerText) + "\n"

	// Get filtered indices if fuzzy search is active
	filteredIndices := m.getFilteredIndices()
	displayCount := len(filteredIndices)

	count := styles.SubtitleStyle.Render(fmt.Sprintf("%d available", displayCount))
	if m.fuzzySearch.IsActive() && m.fuzzySearch.Query() != "" {
		count += styles.AniListMetadataStyle.Render(fmt.Sprintf(" (filtered from %d)", len(m.seasons)))
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
		if actualIndex >= len(m.seasons) {
			break
		}
		season := m.seasons[actualIndex]
		output += m.renderSeasonItem(season, i == m.currentIndex) + "\n\n"
	}

	// Help text at bottom
	helpText := "  ↑/↓ navigate • enter select • / filter • m manga info • p provider • esc back"
	if m.fuzzySearch.IsActive() {
		if m.fuzzySearch.IsLocked() {
			helpText = "  ↑/↓ navigate • enter select • m manga info • / edit • p provider • esc clear"
		} else {
			helpText = "  Type to filter • ↑/↓ nav • esc lock"
		}
	}
	output += "\n" + styles.AniListHelpStyle.Render(helpText)

	return output
}

func (m MangalModel) renderSeasonItem(season providers.Season, selected bool) string {
	boxStyle := styles.AniListItemStyle
	titleStyle := styles.AniListTitleStyle

	if selected {
		boxStyle = styles.AniListItemSelectedStyle
		titleStyle = titleStyle.Foreground(styles.OxocarbonPurple)
	}

	// Season title with high contrast (already contains "Season N")
	title := titleStyle.Render(season.Title)

	return boxStyle.Render(title)
}

// getFilteredIndices returns the indices of seasons that match the fuzzy search
func (m MangalModel) getFilteredIndices() []int {
	searchStrings := make([]string, len(m.seasons))
	for i, season := range m.seasons {
		searchStrings[i] = season.Title
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
