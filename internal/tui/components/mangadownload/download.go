package mangadownload

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

// State represents the download state
type State int

const (
	StateDownloading State = iota
	StateComplete
)

// Model represents the manga download progress view
type Model struct {
	totalChapters    int
	completedCount   int
	failedCount      int
	currentChapter   string
	currentOperation string

	completedChapters []string
	failedChapters    []string

	spinner  spinner.Model
	progress progress.Model
	state    State

	width  int
	height int

	startTime time.Time
}

// New creates a new manga download model
func New() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.OxocarbonPurple)

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	return Model{
		spinner:   s,
		progress:  p,
		state:     StateDownloading,
		startTime: time.Now(),
	}
}

func (m *Model) SetTotal(total int) {
	m.totalChapters = total
}

func (m *Model) SetCurrentChapter(name string) {
	m.currentChapter = name
}

func (m *Model) SetOperation(op string) {
	m.currentOperation = op
}

func (m *Model) ChapterComplete(name string) {
	m.completedCount++
	m.completedChapters = append(m.completedChapters, name)
}

func (m *Model) ChapterFailed(name string) {
	m.failedCount++
	m.failedChapters = append(m.failedChapters, name)
}

func (m Model) IsComplete() bool {
	return m.completedCount+m.failedCount >= m.totalChapters
}

func (m Model) ProgressPercent() float64 {
	if m.totalChapters == 0 {
		return 0.0
	}
	return float64(m.completedCount+m.failedCount) / float64(m.totalChapters)
}

func (m Model) ElapsedTime() time.Duration {
	return time.Since(m.startTime)
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.progress.Init(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		progressWidth := msg.Width - 20
		if progressWidth < 20 {
			progressWidth = 20
		}
		if progressWidth > 60 {
			progressWidth = 60
		}
		m.progress.Width = progressWidth
		return m, nil

	case tea.KeyMsg:
		if m.state == StateComplete {
			switch msg.String() {
			case "enter", "esc":
				return m, func() tea.Msg {
					return DownloadCompleteAckMsg{}
				}
			case "r":
				if m.failedCount > 0 {
					return m, func() tea.Msg {
						return RetryFailedMsg{
							FailedChapters: m.failedChapters,
						}
					}
				}
			}
		}
		return m, nil

	case ProgressUpdateMsg:
		m.currentChapter = msg.ChapterName
		m.currentOperation = msg.Operation
		cmd := m.progress.SetPercent(m.ProgressPercent())
		return m, cmd

	case ChapterCompleteMsg:
		m.ChapterComplete(msg.ChapterName)
		if m.IsComplete() {
			m.state = StateComplete
		}
		cmd := m.progress.SetPercent(m.ProgressPercent())
		return m, cmd

	case ChapterFailedMsg:
		m.ChapterFailed(msg.ChapterName)
		if m.IsComplete() {
			m.state = StateComplete
		}
		cmd := m.progress.SetPercent(m.ProgressPercent())
		return m, cmd

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		var cmd tea.Cmd
		progressModel, progressCmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		cmd = progressCmd
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	if m.state == StateComplete {
		return m.viewComplete()
	}
	return m.viewDownloading()
}

func (m Model) viewDownloading() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonPurple).
		Bold(true).
		MarginBottom(1)

	title := titleStyle.Render("Downloading Manga")

	chapterStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonCyan).
		Bold(true)

	currentInfo := ""
	if m.currentChapter != "" {
		currentInfo = fmt.Sprintf("📥 %s", chapterStyle.Render(m.currentChapter))
	}

	progressBar := m.progress.View()

	progressText := fmt.Sprintf("Chapters: %d/%d complete", m.completedCount+m.failedCount, m.totalChapters)
	if m.failedCount > 0 {
		progressText += lipgloss.NewStyle().
			Foreground(styles.OxocarbonPink).
			Render(fmt.Sprintf(" (%d failed)", m.failedCount))
	}

	operationText := ""
	if m.currentOperation != "" {
		operationText = fmt.Sprintf("%s %s", m.spinner.View(), m.currentOperation)
	}

	elapsed := m.ElapsedTime()
	timeText := lipgloss.NewStyle().
		Foreground(styles.OxocarbonBase05).
		Render(fmt.Sprintf("Elapsed: %s", formatDuration(elapsed)))

	output := lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		title,
		currentInfo,
		"",
		progressBar,
		"",
		progressText,
		operationText,
		"",
		timeText,
		"",
	)

	return output
}

func (m Model) viewComplete() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonGreen).
		Bold(true).
		MarginBottom(1)

	title := titleStyle.Render("Download Complete!")

	successStyle := lipgloss.NewStyle().Foreground(styles.OxocarbonGreen)
	failStyle := lipgloss.NewStyle().Foreground(styles.OxocarbonPink)

	summary := fmt.Sprintf(
		"%s %d chapters downloaded successfully",
		successStyle.Render("✓"),
		m.completedCount,
	)

	if m.failedCount > 0 {
		summary += "\n" + fmt.Sprintf(
			"%s %d chapters failed",
			failStyle.Render("✗"),
			m.failedCount,
		)
	}

	elapsed := m.ElapsedTime()
	timeText := fmt.Sprintf("Total time: %s", formatDuration(elapsed))

	helpText := styles.HelpStyle.Render("[enter] back")
	if m.failedCount > 0 {
		helpText = styles.HelpStyle.Render("[enter] back  •  [r] retry failed")
	}

	output := lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		"",
		title,
		"",
		summary,
		"",
		timeText,
		"",
		"",
		helpText,
		"",
	)

	return output
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}

// Messages

type ProgressUpdateMsg struct {
	ChapterName string
	Operation   string
}

type ChapterCompleteMsg struct {
	ChapterName string
	FilePath    string
}

type ChapterFailedMsg struct {
	ChapterName string
	Error       error
}

type DownloadCompleteAckMsg struct{}

type RetryFailedMsg struct {
	FailedChapters []string
}
