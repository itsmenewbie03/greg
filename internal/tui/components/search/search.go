package search

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

type Model struct {
	textInput textinput.Model
	width     int
	height    int
}

func New() Model {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.Prompt = ""
	ti.Focus()
	ti.CharLimit = 200
	ti.Width = 80

	// Oxocarbon styling: clean look with purple accent
	ti.PromptStyle = lipgloss.NewStyle().Foreground(styles.OxocarbonPurple)
	ti.TextStyle = lipgloss.NewStyle().Foreground(styles.OxocarbonBase05)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(styles.OxocarbonPurple)

	return Model{
		textInput: ti,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Set input width accounting for borders and padding
		if m.width > 20 {
			m.textInput.Width = m.width - 20
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m, func() tea.Msg {
				return common.PerformSearchMsg{Query: m.textInput.Value()}
			}
		case "esc":
			return m, func() tea.Msg {
				return common.GoToHomeMsg{}
			}
		}
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	var output string
	output += "\n"

	// Header
	header := styles.TitleStyle.Render("  SEARCH  ")
	output += header + "\n"

	// Subtitle
	subtitle := styles.SubtitleStyle.Render("  Find your next watch")
	hint := styles.AniListMetadataStyle.Render("  (Press / in results to filter)")
	output += subtitle + "\n" + hint + "\n\n"

	// Search input in bordered container (no extra prompt - textInput has cursor)
	searchBox := styles.AniListItemSelectedStyle.Render(m.textInput.View())
	output += searchBox + "\n"

	// Help text - never show p/1/2/3 in search mode as user needs to type freely
	// User can use these keys before entering search (from home view)
	helpText := "  enter search • esc back"
	output += "\n" + styles.AniListHelpStyle.Render(helpText)

	return output
}

// SetValue sets the value of the search input
func (m *Model) SetValue(value string) {
	m.textInput.SetValue(value)
}

// GetValue returns the value of the search input
func (m Model) GetValue() string {
	return m.textInput.Value()
}
