package providerstatus

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

type Model struct {
	list          list.Model
	err           error
	showingDetail bool
	selectedItem  *item
}

type item struct {
	status *providers.ProviderStatus
}

func (i item) Title() string {
	return i.status.ProviderName
}

func (i item) Description() string {
	status := i.status.Status
	if i.status.Healthy {
		return fmt.Sprintf("✅ %s", status)
	}
	return fmt.Sprintf("❌ %s", status)
}

func (i item) FilterValue() string { return i.status.ProviderName }

func New() Model {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Provider Health Status"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = lipgloss.NewStyle().Bold(true)
	return Model{list: l}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchStatuses,
		tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
			return common.TickMsg(t)
		}),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := styles.AppStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter && !m.showingDetail {
			if i, ok := m.list.SelectedItem().(item); ok {
				m.showingDetail = true
				m.selectedItem = &i
				return m, nil
			}
		}
		if (msg.Type == tea.KeyEscape || msg.String() == "q") && m.showingDetail {
			m.showingDetail = false
			m.selectedItem = nil
			return m, nil
		}
	case common.TickMsg:
		return m, fetchStatuses
	case common.ProviderStatusesMsg:
		items := make([]list.Item, len(msg))
		for i, s := range msg {
			items[i] = item{status: s}
		}
		m.list.SetItems(items)
		return m, nil
	case common.ErrMsg:
		m.err = msg
		return m, nil
	}

	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v", m.err)
	}
	if m.showingDetail {
		return m.renderDetail()
	}
	return m.list.View()
}

func (m Model) renderDetail() string {
	if m.selectedItem == nil || m.selectedItem.status.LastResult == nil {
		return "No health check details available\n\n[Press Escape or q to go back]"
	}
	r := m.selectedItem.status.LastResult
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Provider: %s\n\n", m.selectedItem.status.ProviderName))
	b.WriteString(fmt.Sprintf("Status: %s\n", m.selectedItem.status.Status))
	b.WriteString(fmt.Sprintf("Duration: %s\n\n", r.Duration))
	b.WriteString("Curl Command:\n")
	b.WriteString(r.CurlCommand)
	b.WriteString("\n\n[Press Escape or q to go back]")
	return b.String()
}

func fetchStatuses() tea.Msg {
	statuses := providers.GetProviderStatuses()
	if statuses == nil {
		return common.ErrMsg{Err: fmt.Errorf("failed to get provider statuses")}
	}
	return common.ProviderStatusesMsg(statuses)
}
