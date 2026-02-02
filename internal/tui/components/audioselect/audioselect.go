package audioselect

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justchokingaround/greg/internal/audio"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

// SelectionMsg sent when user selects audio track
type SelectionMsg struct {
	Track     providers.AudioTrack
	AniListID int // For saving preference to database
}

// CancelMsg sent when user cancels selection
type CancelMsg struct{}

type Model struct {
	tracks      []providers.AudioTrack
	fuzzySearch *common.FuzzySearch
	aniListID   int
	selected    int // Current selection index in filtered results
	width       int
	height      int
}

func New(tracks []providers.AudioTrack, aniListID int) Model {
	fuzzySearch := common.NewFuzzySearch()
	// Start in active mode for immediate filtering
	fuzzySearch.Activate()

	return Model{
		tracks:      tracks,
		fuzzySearch: fuzzySearch,
		aniListID:   aniListID,
		selected:    0,
		width:       80,
		height:      20,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Unlock fuzzy search on '/'
		if msg.String() == "/" && m.fuzzySearch.IsLocked() {
			return m, m.fuzzySearch.Unlock()
		}

		// Handle input when fuzzy search unlocked
		if !m.fuzzySearch.IsLocked() {
			switch msg.String() {
			case "esc":
				// Lock fuzzy search, return to navigation
				m.fuzzySearch.Lock()
				return m, nil
			case "enter":
				// Confirm filter, lock and return to navigation
				m.fuzzySearch.Lock()
				m.selected = 0 // Reset selection to first filtered result
				return m, nil
			default:
				// All other keys go to fuzzy input (Phase 1 FIX-02 pattern)
				return m, m.fuzzySearch.Update(msg)
			}
		}

		// Navigation keys when locked (Phase 1 pattern)
		switch msg.String() {
		case "j", "down":
			filtered := m.getFilteredIndices()
			if m.selected < len(filtered)-1 {
				m.selected++
			}
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
		case "enter":
			// Confirm selection
			filtered := m.getFilteredIndices()
			if len(filtered) > 0 && m.selected < len(filtered) {
				trackIndex := filtered[m.selected]
				return m, func() tea.Msg {
					return SelectionMsg{
						Track:     m.tracks[trackIndex],
						AniListID: m.aniListID,
					}
				}
			}
		case "esc", "q":
			// Cancel selection
			return m, func() tea.Msg { return CancelMsg{} }
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.fuzzySearch.SetWidth(msg.Width)
	}

	return m, nil
}

func (m Model) View() string {
	var s strings.Builder

	// Title
	s.WriteString(styles.AniListTitleStyle.Render("Select Audio Track") + "\n\n")

	// Track list
	filtered := m.getFilteredIndices()
	if len(filtered) == 0 {
		// Use plain rendering for no matches message
		s.WriteString(styles.AniListMetadataStyle.Render("\nNo tracks match filter.\n\n"))
	} else {
		for i, idx := range filtered {
			track := m.tracks[idx]
			label := audio.NormalizeAudioLabel(track)

			if i == m.selected {
				// Selected item: use purple highlight (Phase 1 pattern)
				selectedStyle := styles.AniListTitleStyle.Foreground(styles.OxocarbonPurple)
				s.WriteString(selectedStyle.Render("> "+label) + "\n")
			} else {
				s.WriteString(styles.AniListMetadataStyle.Render("  "+label) + "\n")
			}
		}
	}

	// Fuzzy search input (with Phase 1 purple border when unlocked)
	s.WriteString("\n")
	fuzzyView := m.fuzzySearch.View()
	if !m.fuzzySearch.IsLocked() {
		// Phase 1 FIX-02 pattern - purple border when typing
		borderStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.OxocarbonPurple)
		fuzzyView = borderStyle.Render(fuzzyView)
	}
	s.WriteString(fuzzyView + "\n")

	// Help text
	if m.fuzzySearch.IsLocked() {
		s.WriteString("\n" + styles.HelpStyle.Render("j/k: navigate • /: filter • enter: select • esc/q: cancel"))
	} else {
		s.WriteString("\n" + styles.HelpStyle.Render("type to filter • enter: confirm • esc: stop filtering"))
	}

	return s.String()
}

// getFilteredIndices returns track indices matching fuzzy search
func (m Model) getFilteredIndices() []int {
	searchStrings := make([]string, len(m.tracks))
	for i, track := range m.tracks {
		searchStrings[i] = audio.NormalizeAudioLabel(track)
	}
	return m.fuzzySearch.Filter(searchStrings)
}
