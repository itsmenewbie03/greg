package seasons

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/justchokingaround/greg/internal/providers"
)

type Model struct {
	mangal MangalModel
}

func New() Model {
	return Model{
		mangal: NewMangal(),
	}
}

func (m *Model) SetSeasons(seasons []providers.Season) {
	// Recreate mangal model to ensure clean state (currentIndex = 0)
	// But preserve dimensions
	width := m.mangal.width
	height := m.mangal.height

	m.mangal = NewMangal()
	m.mangal.width = width
	m.mangal.height = height
	m.mangal.SetSeasons(seasons)
}

func (m *Model) SetMediaType(mediaType providers.MediaType) {
	m.mangal.SetMediaType(mediaType)
}

// IsInputActive returns true if the fuzzy search input is active and not locked
func (m Model) IsInputActive() bool {
	return m.mangal.fuzzySearch.IsActive() && !m.mangal.fuzzySearch.IsLocked()
}

func (m Model) Init() tea.Cmd {
	return m.mangal.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedModel, cmd := m.mangal.Update(msg)
	m.mangal = updatedModel.(MangalModel)
	return m, cmd
}

func (m Model) View() string {
	return m.mangal.View()
}
