package episodes

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

func (m *Model) SetMediaType(mediaType providers.MediaType) {
	m.mangal.SetMediaType(mediaType)
}

func (m *Model) SetEpisodes(episodes []providers.Episode) {
	// Recreate mangal model to ensure clean state (currentIndex = 0)
	// But preserve the media type and dimensions
	currentMediaType := m.mangal.mediaType
	width := m.mangal.width
	height := m.mangal.height

	m.mangal = NewMangal()
	m.mangal.SetMediaType(currentMediaType)
	m.mangal.width = width
	m.mangal.height = height
	m.mangal.SetEpisodes(episodes)
}

// SetCursorToEpisode sets the cursor to the episode with the given episode number
func (m *Model) SetCursorToEpisode(episodeNumber int) {
	m.mangal.SetCursorToEpisode(episodeNumber)
}

// GetCurrentIndex returns the current selected index in the episodes list
func (m Model) GetCurrentIndex() int {
	return m.mangal.currentIndex
}

// GetEpisodes returns the episodes list
func (m Model) GetEpisodes() []providers.Episode {
	return m.mangal.episodes
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
