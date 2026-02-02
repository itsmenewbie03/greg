package downloads

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"

	"github.com/justchokingaround/greg/internal/downloader"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

// SortMode represents how to sort downloads
type SortMode int

const (
	SortByTitle SortMode = iota
	SortByStatus
	SortByProgress
)

// Model represents the downloads view
type Model struct {
	downloads        []downloader.DownloadTask
	groupedDownloads map[string]*DownloadGroup // MediaTitle -> group info
	expandedGroups   map[string]bool           // Which groups are expanded
	displayItems     []displayItem             // Flattened list for rendering (groups + episodes)
	currentIndex     int
	manager          *downloader.Manager
	width            int
	height           int
	autoRefresh      bool
	tickerRunning    bool // Track if auto-refresh ticker is running
	fuzzySearch      *common.FuzzySearch
	progressBar      progress.Model // Beautiful gradient progress bar
	groupedView      bool           // Toggle between grouped and flat view
	sortMode         SortMode       // Current sort mode

	// Delete confirmation dialog
	showDeleteDialog bool
	deleteTaskID     string
	deleteTaskTitle  string
}

// DownloadGroup represents a group of downloads for the same show
type DownloadGroup struct {
	MediaTitle     string
	Tasks          []downloader.DownloadTask
	TotalProgress  float64 // Average progress across all tasks
	ActiveCount    int     // Number of active downloads
	CompletedCount int     // Number of completed downloads
	FailedCount    int     // Number of failed downloads
}

// displayItem represents an item in the display list (either a group header or an episode)
type displayItem struct {
	isGroup    bool
	groupTitle string
	taskIndex  int // Index in downloads array (for episodes)
}

// getStatusIcon returns an icon for the download status
func getStatusIcon(status downloader.DownloadStatus) string {
	switch status {
	case downloader.StatusQueued:
		return "⏳"
	case downloader.StatusDownloading:
		return "▶"
	case downloader.StatusPaused:
		return "⏸"
	case downloader.StatusCompleted:
		return "✓"
	case downloader.StatusFailed:
		return "✗"
	case downloader.StatusCancelled:
		return "⦸"
	case downloader.StatusProcessing:
		return "⚙"
	default:
		return "?"
	}
}

// renderProgressBar renders a beautiful gradient progress bar (matching manga style)
func (m *Model) renderProgressBar(progressValue float64) string {
	// Use the Bubble Tea progress component with gradient for beautiful rendering
	percent := progressValue / 100.0
	if percent < 0 {
		percent = 0
	}
	if percent > 1.0 {
		percent = 1.0
	}

	return m.progressBar.ViewAs(percent)
}

// formatDuration formats a duration in a human-readable format
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// New creates a new downloads model
func New(manager *downloader.Manager) Model {
	// Create beautiful gradient progress bar (matching manga downloads)
	prog := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(20),
		progress.WithoutPercentage(),
	)

	return Model{
		manager:          manager,
		autoRefresh:      true,
		currentIndex:     0,
		fuzzySearch:      common.NewFuzzySearch(),
		progressBar:      prog,
		groupedView:      true,        // Start with grouped view
		sortMode:         SortByTitle, // Default sort
		groupedDownloads: make(map[string]*DownloadGroup),
		expandedGroups:   make(map[string]bool),
		displayItems:     []displayItem{},
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchDownloads(),
		m.startRefreshTicker(),
		m.progressBar.Init(),
	)
}

// renderDownloadItem renders a single download item with bordered styling
func (m Model) renderDownloadItem(task downloader.DownloadTask, selected bool) string {
	boxStyle := styles.AniListItemStyle
	titleStyle := styles.AniListTitleStyle
	metaStyle := styles.AniListMetadataStyle

	if selected {
		boxStyle = styles.AniListItemSelectedStyle
		titleStyle = titleStyle.Foreground(styles.OxocarbonPurple)
		metaStyle = metaStyle.Foreground(styles.OxocarbonMauve)
	}

	// Build title
	title := task.MediaTitle
	if task.MediaType == providers.MediaTypeManga {
		// For manga, show chapter instead of episode
		if task.Episode > 0 {
			title = fmt.Sprintf("%s - Chapter %d", title, task.Episode)
		}
	} else {
		// For video content
		if task.Episode > 0 {
			title = fmt.Sprintf("%s - Episode %d", title, task.Episode)
		}
		if task.Season > 0 {
			title = fmt.Sprintf("%s (S%02d)", title, task.Season)
		}
		if task.Quality != "" {
			title = fmt.Sprintf("%s [%s]", title, task.Quality)
		}
	}
	titleStr := titleStyle.Render(title)

	// Build status metadata
	statusIcon := getStatusIcon(task.Status)
	var metaParts []string

	// Status badge with color
	statusColor := styles.OxocarbonBase04
	switch task.Status {
	case downloader.StatusDownloading:
		statusColor = styles.OxocarbonGreen
	case downloader.StatusCompleted:
		statusColor = styles.OxocarbonBlue
	case downloader.StatusFailed:
		statusColor = styles.OxocarbonPink
	case downloader.StatusPaused:
		statusColor = styles.OxocarbonPink
	case downloader.StatusQueued:
		statusColor = styles.OxocarbonPurple
	}
	statusBadge := styles.StatusBadgeStyle.Foreground(statusColor).Render(fmt.Sprintf("%s %s", statusIcon, task.Status))
	metaParts = append(metaParts, statusBadge)

	// Progress and additional info based on status
	if task.Status == downloader.StatusDownloading || task.Status == downloader.StatusProcessing {
		progressBar := m.renderProgressBar(task.Progress)
		// Don't use fmt.Sprintf with styled string - append directly
		metaParts = append(metaParts, progressBar)
		metaParts = append(metaParts, fmt.Sprintf("%.1f%%", task.Progress))
		if task.Speed > 0 {
			metaParts = append(metaParts, humanize.Bytes(uint64(task.Speed))+"/s")
		}
		if task.ETA > 0 {
			metaParts = append(metaParts, "ETA: "+formatDuration(task.ETA))
		}
	} else if task.Status == downloader.StatusCompleted {
		// For manga, show file size info
		if task.MediaType == providers.MediaTypeManga {
			// Manga chapters - show as CBZ file
			if task.TotalBytes > 0 {
				metaParts = append(metaParts, humanize.Bytes(uint64(task.TotalBytes)))
			}
			metaParts = append(metaParts, "CBZ")
		} else {
			// Video content
			if task.TotalBytes > 0 {
				metaParts = append(metaParts, humanize.Bytes(uint64(task.TotalBytes)))
			}
		}
		if task.CompletedAt != nil {
			metaParts = append(metaParts, humanize.Time(*task.CompletedAt))
		}
	} else if task.Status == downloader.StatusFailed && task.Error != "" {
		errMsg := task.Error
		if len(errMsg) > 40 {
			errMsg = errMsg[:37] + "..."
		}
		metaParts = append(metaParts, errMsg)
	} else if task.Status == downloader.StatusPaused && task.Progress > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%.1f%%", task.Progress))
	}

	metaStr := metaStyle.Render(strings.Join(metaParts, " • "))
	content := titleStr + "\n" + metaStr

	return boxStyle.Render(content)
}

// renderGroupItem renders a group header with aggregate statistics
func (m Model) renderGroupItem(group *DownloadGroup, selected bool) string {
	boxStyle := styles.AniListItemStyle
	titleStyle := styles.AniListTitleStyle
	metaStyle := styles.AniListMetadataStyle

	if selected {
		boxStyle = styles.AniListItemSelectedStyle
		titleStyle = titleStyle.Foreground(styles.OxocarbonPurple)
		metaStyle = metaStyle.Foreground(styles.OxocarbonMauve)
	}

	// Group title with expand/collapse indicator
	expandIndicator := "▶"
	if m.expandedGroups[group.MediaTitle] {
		expandIndicator = "▼"
	}
	titleStr := titleStyle.Render(fmt.Sprintf("%s %s", expandIndicator, group.MediaTitle))

	// Build statistics - more compact
	var metaParts []string

	// Episode count
	metaParts = append(metaParts, fmt.Sprintf("%d eps", len(group.Tasks)))

	// Status badges - compact
	if group.ActiveCount > 0 {
		badge := styles.StatusBadgeStyle.Foreground(styles.OxocarbonGreen).Render(fmt.Sprintf("▶%d", group.ActiveCount))
		metaParts = append(metaParts, badge)
	}
	if group.CompletedCount > 0 {
		badge := styles.StatusBadgeStyle.Foreground(styles.OxocarbonBlue).Render(fmt.Sprintf("✓%d", group.CompletedCount))
		metaParts = append(metaParts, badge)
	}
	if group.FailedCount > 0 {
		badge := styles.StatusBadgeStyle.Foreground(styles.OxocarbonRed).Render(fmt.Sprintf("✗%d", group.FailedCount))
		metaParts = append(metaParts, badge)
	}

	// Compact progress - only show if active or in progress
	if group.ActiveCount > 0 || (group.CompletedCount > 0 && group.CompletedCount < len(group.Tasks)) {
		// Use smaller progress bar
		progressWidth := 15
		if m.width > 80 {
			progressWidth = 20
		}
		oldWidth := m.progressBar.Width
		m.progressBar.Width = progressWidth
		progressBar := m.renderProgressBar(group.TotalProgress)
		m.progressBar.Width = oldWidth
		metaParts = append(metaParts, progressBar)
		metaParts = append(metaParts, fmt.Sprintf("%.0f%%", group.TotalProgress))
	}

	metaStr := metaStyle.Render(strings.Join(metaParts, " "))
	content := titleStr + " " + metaStr

	return boxStyle.Render(content)
}

// getFilteredIndices returns indices of downloads matching fuzzy search
func (m Model) getFilteredIndices() []int {
	searchStrings := make([]string, len(m.downloads))
	for i, task := range m.downloads {
		title := task.MediaTitle
		if task.Episode > 0 {
			title = fmt.Sprintf("%s Episode %d", title, task.Episode)
		}
		searchStrings[i] = title
	}
	return m.fuzzySearch.Filter(searchStrings)
}

// getVisibleRange returns the start and end indices for visible items
func (m Model) getVisibleRange(total int) (int, int) {
	maxVisible := 8
	if m.height > 0 {
		itemsSpace := m.height - 8
		if itemsSpace > 0 {
			maxVisible = itemsSpace / 3
		}
		if maxVisible < 3 {
			maxVisible = 3
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

// handleAction performs an action on the currently selected download
func (m Model) handleAction(action string) (Model, tea.Cmd) {
	if m.currentIndex >= len(m.displayItems) {
		return m, nil
	}

	item := m.displayItems[m.currentIndex]

	// If it's a group, handle group actions
	if item.isGroup {
		switch action {
		case "enter", " ":
			// Toggle between grouped and flat view
			m.groupedView = !m.groupedView
			m.buildDisplayItems()
			// Position cursor at first item
			m.currentIndex = 0
		case "D", "delete":
			// Delete entire show folder
			group := m.groupedDownloads[item.groupTitle]
			if group != nil && len(group.Tasks) > 0 {
				m.showDeleteDialog = true
				m.deleteTaskID = "" // Empty means delete folder, not single file
				m.deleteTaskTitle = fmt.Sprintf("%s (all %d episodes)", group.MediaTitle, len(group.Tasks))
				return m, nil
			}
		case "c":
			// Cancel all active downloads in this group
			group := m.groupedDownloads[item.groupTitle]
			if group != nil {
				var cmds []tea.Cmd
				for _, task := range group.Tasks {
					if !task.Status.IsComplete() && task.Status != downloader.StatusCancelled {
						cmds = append(cmds, m.cancelDownload(task.ID))
					}
				}
				if len(cmds) > 0 {
					return m, tea.Batch(cmds...)
				}
			}
		}
		return m, nil
	}

	// It's an episode task
	if item.taskIndex >= len(m.downloads) {
		return m, nil
	}

	task := m.downloads[item.taskIndex]

	switch action {
	case "enter", " ":
		// Open the downloaded file if completed
		if task.Status == downloader.StatusCompleted && task.OutputPath != "" {
			return m, openFile(task.OutputPath)
		}
	case "p":
		if task.Status == downloader.StatusDownloading {
			return m, m.pauseDownload(task.ID)
		}
	case "r":
		if task.Status == downloader.StatusPaused {
			return m, m.resumeDownload(task.ID)
		}
	case "retry":
		// Retry failed or cancelled downloads
		if task.Status == downloader.StatusFailed || task.Status == downloader.StatusCancelled {
			return m, m.retryDownload(task.ID)
		}
	case "c":
		if !task.Status.IsComplete() {
			return m, m.cancelDownload(task.ID)
		}
	case "D", "delete":
		// Show delete confirmation
		m.showDeleteDialog = true
		m.deleteTaskID = task.ID
		m.deleteTaskTitle = task.MediaTitle
		if task.Episode > 0 {
			m.deleteTaskTitle += fmt.Sprintf(" - Episode %d", task.Episode)
		}
		return m, nil
	}

	return m, nil
}

// Refresh fetches the latest downloads from the manager and starts auto-refresh
func (m Model) Refresh() tea.Cmd {
	// Always fetch downloads, but only start ticker if not already running
	if m.tickerRunning {
		return m.fetchDownloads()
	}
	return tea.Batch(
		m.fetchDownloads(),
		m.startRefreshTicker(),
	)
}

// startRefreshTicker starts the periodic refresh ticker
func (m Model) startRefreshTicker() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(300 * time.Millisecond) // Refresh every 300ms for snappy updates
		return common.DownloadsTickMsg{}
	}
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.fuzzySearch.SetWidth(msg.Width)

		// Update progress bar width based on terminal size
		progressWidth := 20
		if msg.Width > 80 {
			progressWidth = 30
		}
		if msg.Width > 120 {
			progressWidth = 40
		}
		m.progressBar.Width = progressWidth

	case tea.KeyMsg:
		// Handle delete dialog
		if m.showDeleteDialog {
			switch msg.String() {
			case "y", "Y", "enter":
				// Confirm delete
				var cmd tea.Cmd
				if m.deleteTaskID == "" {
					// Delete entire folder (all episodes of show)
					cmd = m.deleteShowFolder(m.deleteTaskTitle)
				} else {
					// Delete single file
					cmd = m.deleteFile(m.deleteTaskID)
				}
				m.showDeleteDialog = false
				m.deleteTaskID = ""
				m.deleteTaskTitle = ""
				return m, cmd
			case "n", "N", "esc":
				// Cancel delete
				m.showDeleteDialog = false
				m.deleteTaskID = ""
				m.deleteTaskTitle = ""
				return m, nil
			}
			return m, nil
		}

		// Fuzzy search mode
		if m.fuzzySearch.IsActive() {
			switch msg.String() {
			case "esc":
				m.fuzzySearch.Deactivate()
				m.currentIndex = 0
				return m, nil
			case "up", "k":
				if m.currentIndex > 0 {
					m.currentIndex--
				}
				return m, nil
			case "down", "j":
				filteredIndices := m.getFilteredIndices()
				if m.currentIndex < len(filteredIndices)-1 {
					m.currentIndex++
				}
				return m, nil
			case "p", "r", "c", "D", "delete":
				return m.handleAction(msg.String())
			default:
				// Pass other keys to fuzzy search input
				cmd := m.fuzzySearch.Update(msg)
				m.currentIndex = 0
				return m, cmd
			}
		}

		// Normal mode (fuzzy search not active)
		maxIndex := len(m.displayItems) - 1
		if maxIndex < 0 {
			maxIndex = 0
		}

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
		case "down", "j":
			if m.currentIndex < maxIndex {
				m.currentIndex++
			}
		case "esc":
			// If in flat view, go back to grouped view
			if !m.groupedView {
				m.groupedView = true
				m.buildDisplayItems()
				m.currentIndex = 0
				return m, nil
			}
			// In grouped view, do nothing (stay in downloads)
			return m, nil
		case "q":
			// Always go back to home
			m.tickerRunning = false
			return m, func() tea.Msg {
				return common.GoToHomeMsg{}
			}
		case "p", "r", "c", "D", "delete":
			return m.handleAction(msg.String())
		case "R":
			// Retry failed/cancelled download
			return m.handleAction("retry")
		case "x":
			// Clear completed downloads
			return m, m.clearCompleted()
		case "ctrl+r":
			// Refresh list
			return m, m.fetchDownloads()
		case "s":
			// Cycle through sort modes
			m.sortMode = (m.sortMode + 1) % 3
			// Force rebuild of grouped view and display
			m.buildGroupedView()
			m.currentIndex = 0
		case "enter", " ":
			// Toggle group expansion, open file, or perform action
			return m.handleAction(msg.String())
		}

	case common.DownloadProgressUpdateMsg:
		// Update the specific task
		for i := range m.downloads {
			if m.downloads[i].ID == msg.TaskID {
				m.downloads[i].Progress = msg.Progress
				m.downloads[i].Speed = msg.Speed
				if msg.ETA != "" {
					// Parse ETA string
					if eta, err := time.ParseDuration(msg.ETA); err == nil {
						m.downloads[i].ETA = eta
					}
				}
				m.downloads[i].Status = downloader.DownloadStatus(msg.Status)
				break
			}
		}

	case common.DownloadCompleteMsg, common.DownloadErrorMsg:
		// Refresh the downloads list
		return m, m.fetchDownloads()

	case downloadsRefreshMsg:
		m.downloads = msg.downloads
		m.buildGroupedView()
		// Keep currentIndex valid
		if m.currentIndex >= len(m.displayItems) {
			m.currentIndex = len(m.displayItems) - 1
		}
		if m.currentIndex < 0 {
			m.currentIndex = 0
		}

	case common.DownloadsTickMsg:
		// Mark ticker as running
		if !m.tickerRunning {
			m.tickerRunning = true
		}

		// ALWAYS auto-refresh to show live progress updates
		// This ensures real-time progress is visible when user navigates to downloads view
		return m, tea.Batch(
			m.fetchDownloads(),
			m.startRefreshTicker(),
		)

	case progress.FrameMsg:
		// Update progress bar animation
		progressModel, cmd := m.progressBar.Update(msg)
		m.progressBar = progressModel.(progress.Model)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the model
func (m Model) View() string {
	if len(m.downloads) == 0 {
		return styles.AniListMetadataStyle.Render("\nNo downloads yet.\n\nPress 'd' on any episode to start downloading.")
	}

	var output string
	output += "\n"

	// Header
	viewMode := "GROUPED"
	if !m.groupedView {
		viewMode = "FLAT"
	}
	sortModeStr := ""
	switch m.sortMode {
	case SortByTitle:
		sortModeStr = "by Title"
	case SortByStatus:
		sortModeStr = "by Status"
	case SortByProgress:
		sortModeStr = "by Progress"
	}
	header := styles.TitleStyle.Render(fmt.Sprintf("  DOWNLOADS (%s • %s)  ", viewMode, sortModeStr))
	output += header + "\n"

	// Count
	displayCount := len(m.displayItems)
	if !m.groupedView {
		displayCount = len(m.downloads)
	}

	count := styles.SubtitleStyle.Render(fmt.Sprintf("  %d items", displayCount))
	if m.groupedView {
		count += styles.AniListMetadataStyle.Render(fmt.Sprintf(" • %d shows", len(m.groupedDownloads)))
	}
	output += count + "\n"

	// Fuzzy search
	if m.fuzzySearch.IsActive() {
		output += "\n" + m.fuzzySearch.View() + "\n\n"
	} else {
		output += "\n"
	}

	// Render items
	visibleStart, visibleEnd := m.getVisibleRange(len(m.displayItems))

	for i := visibleStart; i < visibleEnd; i++ {
		if i >= len(m.displayItems) {
			break
		}
		item := m.displayItems[i]

		if item.isGroup {
			// Render group header
			group := m.groupedDownloads[item.groupTitle]
			output += m.renderGroupItem(group, i == m.currentIndex) + "\n"
		} else {
			// Render episode
			if item.taskIndex < len(m.downloads) {
				task := m.downloads[item.taskIndex]
				// Add indentation for episodes under groups in grouped view
				itemStr := m.renderDownloadItem(task, i == m.currentIndex)
				if m.groupedView {
					itemStr = "  " + itemStr
				}
				output += itemStr + "\n"
			}
		}
	}

	// Help text - ultra compact to fit on screen
	helpText := "  ↑/↓ • ⏎ expand/open • s sort • p/r pause/resume • R retry • c cancel • D del • x clear • esc back • q quit"
	if !m.groupedView {
		// In flat view, show that esc goes back to grouped
		helpText = "  ↑/↓ • ⏎ open • s sort • p/r • R retry • c cancel • D del • x clear • esc grouped • q quit"
	}
	if m.fuzzySearch.IsActive() {
		if m.fuzzySearch.IsLocked() {
			helpText = "  ↑/↓ • p/r • R retry • c cancel • D del • / edit • esc clear • q quit"
		} else {
			helpText = "  Type to filter • ↑/↓ • esc lock • q quit"
		}
	}
	output += "\n" + styles.AniListHelpStyle.Render(helpText)

	// Render delete confirmation dialog
	if m.showDeleteDialog {
		dialog := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.OxocarbonRed).
			Padding(1, 2).
			Render(fmt.Sprintf(
				"%s\n\nAre you sure you want to delete this file?\n%s\n\n%s",
				styles.TitleStyle.Foreground(styles.OxocarbonRed).Render("DELETE FILE"),
				styles.AniListTitleStyle.Render(m.deleteTaskTitle),
				styles.AniListHelpStyle.Render("(y) Confirm • (n/esc) Cancel"),
			))

		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			dialog,
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceForeground(lipgloss.Color("#161616")),
		)
	}

	return output
}

// SetDownloads updates the downloads list
func (m *Model) SetDownloads(downloads []downloader.DownloadTask) {
	m.downloads = downloads
	m.buildGroupedView()

	// Keep currentIndex valid
	maxIndex := len(m.displayItems)
	if m.currentIndex >= maxIndex {
		m.currentIndex = maxIndex - 1
	}
	if m.currentIndex < 0 {
		m.currentIndex = 0
	}
}

// buildGroupedView organizes downloads by show
func (m *Model) buildGroupedView() {
	m.groupedDownloads = make(map[string]*DownloadGroup)

	// Group downloads by MediaTitle
	for _, task := range m.downloads {
		title := task.MediaTitle
		if group, exists := m.groupedDownloads[title]; exists {
			group.Tasks = append(group.Tasks, task)
		} else {
			m.groupedDownloads[title] = &DownloadGroup{
				MediaTitle: title,
				Tasks:      []downloader.DownloadTask{task},
			}
		}
	}

	// Calculate group statistics and sort tasks within each group by episode
	for _, group := range m.groupedDownloads {
		// Sort tasks by season and episode
		tasks := group.Tasks
		for i := 0; i < len(tasks)-1; i++ {
			for j := i + 1; j < len(tasks); j++ {
				if tasks[i].Season > tasks[j].Season ||
					(tasks[i].Season == tasks[j].Season && tasks[i].Episode > tasks[j].Episode) {
					tasks[i], tasks[j] = tasks[j], tasks[i]
				}
			}
		}
		group.Tasks = tasks

		// Calculate statistics
		var totalProgress float64

		for _, task := range group.Tasks {
			// Count completed episodes for total progress
			if task.Status == downloader.StatusCompleted {
				totalProgress += 100.0
			} else {
				totalProgress += task.Progress
			}

			// Track status counts
			switch task.Status {
			case downloader.StatusDownloading, downloader.StatusProcessing, downloader.StatusQueued:
				group.ActiveCount++
			case downloader.StatusCompleted:
				group.CompletedCount++
			case downloader.StatusFailed, downloader.StatusCancelled:
				group.FailedCount++
			}
		}

		// Total progress = sum of all episode progress / total episodes
		// This gives overall completion: 3 of 10 episodes done = 30%
		if len(group.Tasks) > 0 {
			group.TotalProgress = totalProgress / float64(len(group.Tasks))
		}
	}

	// Build display items list
	m.buildDisplayItems()
}

// buildDisplayItems creates the flattened list of items to display
func (m *Model) buildDisplayItems() {
	m.displayItems = []displayItem{}

	if m.groupedView {
		// Get list of groups
		groups := make([]*DownloadGroup, 0, len(m.groupedDownloads))
		for _, group := range m.groupedDownloads {
			groups = append(groups, group)
		}

		// Sort groups based on sort mode
		m.sortGroups(groups)

		// Build display items
		for _, group := range groups {
			// Add group header
			m.displayItems = append(m.displayItems, displayItem{
				isGroup:    true,
				groupTitle: group.MediaTitle,
			})

			// Add episodes if expanded
			if m.expandedGroups[group.MediaTitle] {
				for taskIdx := range m.downloads {
					if m.downloads[taskIdx].MediaTitle == group.MediaTitle {
						m.displayItems = append(m.displayItems, displayItem{
							isGroup:   false,
							taskIndex: taskIdx,
						})
					}
				}
			}
		}
	} else {
		// Flat view - sort all downloads by sort mode
		sortedDownloads := make([]int, len(m.downloads))
		for i := range m.downloads {
			sortedDownloads[i] = i
		}

		// Sort indices based on sort mode
		for i := 0; i < len(sortedDownloads)-1; i++ {
			for j := i + 1; j < len(sortedDownloads); j++ {
				shouldSwap := false
				task1 := m.downloads[sortedDownloads[i]]
				task2 := m.downloads[sortedDownloads[j]]

				switch m.sortMode {
				case SortByTitle:
					// Sort by MediaTitle, then episode
					if task1.MediaTitle != task2.MediaTitle {
						shouldSwap = task1.MediaTitle > task2.MediaTitle
					} else {
						shouldSwap = task1.Episode > task2.Episode
					}
				case SortByStatus:
					// Sort by status (failed, active, completed), then episode
					status1 := m.statusPriority(task1.Status)
					status2 := m.statusPriority(task2.Status)
					if status1 != status2 {
						shouldSwap = status1 > status2
					} else {
						shouldSwap = task1.Episode > task2.Episode
					}
				case SortByProgress:
					// Sort by progress (ascending), then episode
					if task1.Progress != task2.Progress {
						shouldSwap = task1.Progress > task2.Progress
					} else {
						shouldSwap = task1.Episode > task2.Episode
					}
				}

				if shouldSwap {
					sortedDownloads[i], sortedDownloads[j] = sortedDownloads[j], sortedDownloads[i]
				}
			}
		}

		// Build display items in sorted order
		for _, idx := range sortedDownloads {
			m.displayItems = append(m.displayItems, displayItem{
				isGroup:   false,
				taskIndex: idx,
			})
		}
	}
}

// sortGroups sorts groups based on current sort mode
func (m *Model) sortGroups(groups []*DownloadGroup) {
	for i := 0; i < len(groups)-1; i++ {
		for j := i + 1; j < len(groups); j++ {
			shouldSwap := false
			switch m.sortMode {
			case SortByTitle:
				// Alphabetical
				shouldSwap = groups[i].MediaTitle > groups[j].MediaTitle
			case SortByStatus:
				// Sort by failed count (descending - MOST failed first), then active, then completed
				if groups[i].FailedCount != groups[j].FailedCount {
					shouldSwap = groups[i].FailedCount < groups[j].FailedCount // Swap if i has fewer failures
				} else if groups[i].ActiveCount != groups[j].ActiveCount {
					shouldSwap = groups[i].ActiveCount < groups[j].ActiveCount // Then by active
				} else {
					shouldSwap = groups[i].CompletedCount < groups[j].CompletedCount // Then by completed
				}
			case SortByProgress:
				// Sort by progress (ascending - less complete first)
				shouldSwap = groups[i].TotalProgress > groups[j].TotalProgress
			}
			if shouldSwap {
				groups[i], groups[j] = groups[j], groups[i]
			}
		}
	}
}

// statusPriority assigns priority for sorting by status
// Lower number = higher priority (shows first)
func (m *Model) statusPriority(status downloader.DownloadStatus) int {
	switch status {
	case downloader.StatusFailed:
		return 0 // Show failed first
	case downloader.StatusCancelled:
		return 1
	case downloader.StatusDownloading:
		return 2
	case downloader.StatusProcessing:
		return 3
	case downloader.StatusQueued:
		return 4
	case downloader.StatusPaused:
		return 5
	case downloader.StatusCompleted:
		return 6 // Show completed last
	default:
		return 7
	}
}

// fetchDownloads fetches the current downloads from the manager
func (m Model) fetchDownloads() tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return downloadsRefreshMsg{downloads: []downloader.DownloadTask{}}
		}

		downloads, err := m.manager.GetQueue(context.Background())
		if err != nil {
			return downloadsRefreshMsg{downloads: []downloader.DownloadTask{}}
		}

		return downloadsRefreshMsg{downloads: downloads}
	}
}

// pauseDownload pauses a download
func (m Model) pauseDownload(id string) tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return nil
		}
		ctx := context.Background()
		_ = m.manager.Pause(ctx, id)
		// Fetch updated downloads
		downloads, err := m.manager.GetQueue(ctx)
		if err != nil {
			return downloadsRefreshMsg{downloads: []downloader.DownloadTask{}}
		}
		return downloadsRefreshMsg{downloads: downloads}
	}
}

// resumeDownload resumes a download
func (m Model) resumeDownload(id string) tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return nil
		}
		ctx := context.Background()
		_ = m.manager.Resume(ctx, id)
		// Fetch updated downloads
		downloads, err := m.manager.GetQueue(ctx)
		if err != nil {
			return downloadsRefreshMsg{downloads: []downloader.DownloadTask{}}
		}
		return downloadsRefreshMsg{downloads: downloads}
	}
}

// cancelDownload cancels a download
func (m Model) cancelDownload(id string) tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return nil
		}
		ctx := context.Background()
		_ = m.manager.Cancel(ctx, id)
		// Fetch updated downloads
		downloads, err := m.manager.GetQueue(ctx)
		if err != nil {
			return downloadsRefreshMsg{downloads: []downloader.DownloadTask{}}
		}
		return downloadsRefreshMsg{downloads: downloads}
	}
}

// retryDownload retries a failed or cancelled download
func (m Model) retryDownload(id string) tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return nil
		}
		ctx := context.Background()
		if err := m.manager.Retry(ctx, id); err != nil {
			// Log error but don't crash
			return nil
		}
		// Fetch updated downloads
		downloads, err := m.manager.GetQueue(ctx)
		if err != nil {
			return downloadsRefreshMsg{downloads: []downloader.DownloadTask{}}
		}
		return downloadsRefreshMsg{downloads: downloads}
	}
}

// clearCompleted clears completed downloads
func (m Model) clearCompleted() tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return nil
		}
		ctx := context.Background()
		_ = m.manager.ClearQueue(ctx)
		// Fetch updated downloads
		downloads, err := m.manager.GetQueue(ctx)
		if err != nil {
			return downloadsRefreshMsg{downloads: []downloader.DownloadTask{}}
		}
		return downloadsRefreshMsg{downloads: downloads}
	}
}

// deleteFile deletes a file and its task
func (m Model) deleteFile(id string) tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return nil
		}
		ctx := context.Background()
		_ = m.manager.DeleteTaskAndFile(ctx, id)
		// Fetch updated downloads
		downloads, err := m.manager.GetQueue(ctx)
		if err != nil {
			return downloadsRefreshMsg{downloads: []downloader.DownloadTask{}}
		}
		return downloadsRefreshMsg{downloads: downloads}
	}
}

// deleteShowFolder deletes the entire folder for a show (all episodes)
func (m Model) deleteShowFolder(showTitle string) tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return nil
		}

		// Extract just the show name (remove the "(all X episodes)" part)
		parts := strings.Split(showTitle, " (all ")
		actualTitle := showTitle
		if len(parts) > 0 {
			actualTitle = parts[0]
		}

		// Find all tasks for this show
		ctx := context.Background()
		downloads, err := m.manager.GetQueue(ctx)
		if err != nil {
			return downloadsRefreshMsg{downloads: []downloader.DownloadTask{}}
		}

		// Collect show folder path for cleanup
		var showFolder string

		// Delete each task and file
		for _, task := range downloads {
			if task.MediaTitle == actualTitle {
				// Get folder path before deleting
				if showFolder == "" && task.OutputPath != "" {
					showFolder = filepath.Dir(task.OutputPath)
				}
				// Delete task and file
				_ = m.manager.DeleteTaskAndFile(ctx, task.ID)
			}
		}

		// Try to remove the show folder (will only succeed if empty)
		if showFolder != "" {
			_ = os.RemoveAll(showFolder) // Use RemoveAll to recursively delete
		}

		// Give database a moment to commit changes
		time.Sleep(100 * time.Millisecond)

		// Fetch updated downloads
		downloads, err = m.manager.GetQueue(ctx)
		if err != nil {
			return downloadsRefreshMsg{downloads: []downloader.DownloadTask{}}
		}
		return downloadsRefreshMsg{downloads: downloads}
	}
}

// openFile opens a file with the system default application
func openFile(path string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", path)
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", path)
		default: // linux, bsd, etc
			cmd = exec.Command("xdg-open", path)
		}
		if err := cmd.Start(); err != nil {
			// Silently fail - file might not exist or no handler
			return nil
		}
		return nil
	}
}

// downloadsRefreshMsg is an internal message for refreshing downloads
type downloadsRefreshMsg struct {
	downloads []downloader.DownloadTask
}
