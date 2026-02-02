package mangainfo

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

type Model struct {
	viewport viewport.Model
	content  string
	ready    bool
	parent   common.Component
	Height   int
	Width    int
}

func New(parent common.Component) *Model {
	vp := viewport.New(0, 0)
	vp.YPosition = styles.TitleStyle.GetVerticalFrameSize()
	return &Model{
		parent:   parent,
		viewport: vp,
	}
}

func (m *Model) SetParent(parent common.Component) {
	m.parent = parent
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// headerHeight := styles.TitleStyle.GetVerticalFrameSize() + 1

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height)
			m.viewport.YPosition = styles.TitleStyle.GetVerticalFrameSize() + 1
			m.ready = true
		}
		m.SetSize(msg.Width, msg.Height)
	case common.MangaInfoResultMsg:
		var infoText string
		if msg.Err != nil {
			infoText = fmt.Sprintf("Error fetching manga info: %s", msg.Err.Error())
		} else {
			infoText = msg.Info
			if infoText == "" {
				infoText = "No information found for this anime."
			}
		}

		// Ensure size is set before setting content
		if m.Width == 0 || m.Height == 0 {
			m.SetSize(0, 0) // Trigger fallback
		}

		m.SetContent(infoText)

		// Force update to render content
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg {
				return common.BackMsg{}
			}
		}
	}
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m *Model) SetSize(width, height int) {
	if width <= 0 {
		width = 80 // Fallback width
	}
	if height <= 0 {
		height = 24 // Fallback height
	}

	m.Width = width
	m.Height = height

	// Calculate inner size for viewport
	// Border (2) + Padding (2) = 4 horizontal
	// Header (2) + Border (2) = 4 vertical

	vpWidth := width - 4
	if vpWidth < 20 {
		vpWidth = 20
	}

	vpHeight := height - 4
	if vpHeight < 5 {
		vpHeight = 5
	}

	if !m.ready {
		m.viewport = viewport.New(vpWidth, vpHeight)
		m.ready = true
	} else {
		m.viewport.Width = vpWidth
		m.viewport.Height = vpHeight
	}
}

func (m *Model) SetContent(content string) {
	m.content = content

	// Calculate safe width for wrapping
	width := m.Width - 6 // Account for border(2) + padding(2) + safety(2)
	if width < 20 {
		width = 76 // Fallback if width is invalid
	}

	// Wrap content using lipgloss
	// We apply the style here so it's stored in the viewport
	styledContent := lipgloss.NewStyle().
		Foreground(styles.OxocarbonBase05).
		Width(width).
		Render(content)

	m.viewport.SetContent(styledContent)
	m.viewport.GotoTop()
}

func (m *Model) View() string {
	if !m.ready {
		return "Loading manga info..."
	}

	header := styles.TitleStyle.Render("Manga Info")

	// Get viewport content
	body := m.viewport.View()

	// Check if body is effectively empty (only whitespace/ansi codes)
	// This handles cases where viewport renders empty lines
	if strings.TrimSpace(body) == "" && m.content != "" {
		// Fallback: Render content directly
		width := m.Width - 6
		if width < 20 {
			width = 76
		}

		body = lipgloss.NewStyle().
			Foreground(styles.OxocarbonBase05).
			Width(width).
			Render(m.content)
	}

	// Wrap in a nice box
	// Ensure box width matches the viewport/content width logic
	boxWidth := m.Width - 2 // Total width minus outer margins if any?
	// Actually SetSize sets m.Width to window width.
	// Box takes full width usually.
	if boxWidth < 20 {
		boxWidth = 78
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OxocarbonPurple).
		Padding(0, 1).
		Width(boxWidth).
		Render(body)

	return fmt.Sprintf("%s\n%s", header, box)
}

func (m *Model) ShouldShowMangaInfo() bool {
	return !strings.HasPrefix(m.content, "Loading") && !strings.HasPrefix(m.content, "Error")
}
