package results

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

func (m *Model) SetMediaResults(results []providers.Media) {
	// Recreate mangal model to ensure clean state (currentIndex = 0)
	// But preserve width and height from previous model
	oldWidth := m.mangal.width
	oldHeight := m.mangal.height
	m.mangal = NewMangal()
	m.mangal.width = oldWidth
	m.mangal.height = oldHeight
	m.mangal.SetMediaResults(results)
}

func (m *Model) SetEpisodeResults(results []providers.Episode) {
	// Recreate mangal model to ensure clean state (currentIndex = 0)
	// But preserve width and height from previous model
	oldWidth := m.mangal.width
	oldHeight := m.mangal.height
	m.mangal = NewMangal()
	m.mangal.width = oldWidth
	m.mangal.height = oldHeight
	m.mangal.SetEpisodeResults(results)
}

func (m *Model) UpdateMediaItem(index int, media providers.Media) {
	m.mangal.UpdateMediaItem(index, media)
}

func (m Model) GetMediaResults() []providers.Media {
	return m.mangal.GetMediaResults()
}

func (m Model) GetSelectedMedia() *providers.Media {
	return m.mangal.GetSelectedMedia()
}

func (m Model) GetSelectedIndex() int {
	return m.mangal.GetSelectedIndex()
}

func (m Model) GetItems() []providers.Media {
	return m.mangal.GetItems()
}

func (m *Model) SetShowMangaInfo(show bool) {
	m.mangal.showMangaInfo = show
}

func (m *Model) SetProviderName(name string) {
	m.mangal.providerName = name
}

func (m *Model) SetIsProviderSelection(isProviderSelection bool) {
	m.mangal.isProviderSelection = isProviderSelection
}

func (m Model) IsInfoOpen() bool {
	return m.mangal.showInfoDialog
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
