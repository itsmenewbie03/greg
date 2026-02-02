package help

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

// HelpContext represents which view the help is being shown in
type HelpContext int

const (
	GlobalContext HelpContext = iota
	HomeContext
	SearchContext
	ResultsContext
	EpisodesContext
	SeasonsContext
	AniListContext
	DownloadsContext
	SettingsContext
	HistoryContext
	ProviderStatusContext
)

// Shortcut represents a keyboard shortcut with its description
type Shortcut struct {
	Key         string
	Description string
	Context     []HelpContext
}

// Model represents the help panel state
type Model struct {
	context      HelpContext
	width        int
	height       int
	visible      bool
	providerName string
	scrollOffset int // Scroll position for help content
}

// all shortcuts organized by context
var allShortcuts = []Shortcut{
	// Global shortcuts (available in all contexts)
	{Key: "↑/↓ or j/k", Description: "Navigate up/down", Context: []HelpContext{GlobalContext}},
	{Key: "enter", Description: "Select item", Context: []HelpContext{GlobalContext}},
	{Key: "esc", Description: "Go back / Cancel", Context: []HelpContext{GlobalContext}},
	{Key: "ctrl+h", Description: "Return to home", Context: []HelpContext{GlobalContext}},
	{Key: "q", Description: "Quit application", Context: []HelpContext{GlobalContext}},
	{Key: "d", Description: "Go to downloads (from home)", Context: []HelpContext{GlobalContext}},
	{Key: "?", Description: "Show/hide this help", Context: []HelpContext{GlobalContext}},

	// Home context
	{Key: "s", Description: "Open search", Context: []HelpContext{HomeContext}},
	{Key: "l", Description: "Open AniList library", Context: []HelpContext{HomeContext}},
	{Key: "d", Description: "View downloads", Context: []HelpContext{HomeContext}},
	{Key: "h", Description: "View watch history", Context: []HelpContext{HomeContext}},
	{Key: "P", Description: "View provider health status", Context: []HelpContext{HomeContext}},
	{Key: "p", Description: "Switch provider", Context: []HelpContext{HomeContext}},
	{Key: "tab", Description: "Toggle anime/movies/manga", Context: []HelpContext{HomeContext}},
	{Key: "1", Description: "Switch to movies/TV", Context: []HelpContext{HomeContext}},
	{Key: "2", Description: "Switch to anime", Context: []HelpContext{HomeContext}},
	{Key: "3", Description: "Switch to manga", Context: []HelpContext{HomeContext}},
	{Key: "w", Description: "Share recent item via WatchParty", Context: []HelpContext{HomeContext}},

	// Search context (when not typing)
	{Key: "p", Description: "Switch provider", Context: []HelpContext{SearchContext}},
	{Key: "1", Description: "Switch to movies/TV", Context: []HelpContext{SearchContext}},
	{Key: "2", Description: "Switch to anime", Context: []HelpContext{SearchContext}},
	{Key: "3", Description: "Switch to manga", Context: []HelpContext{SearchContext}},

	// Results/Episodes/Seasons context
	{Key: "/", Description: "Filter results", Context: []HelpContext{ResultsContext, EpisodesContext, SeasonsContext}},
	{Key: "p", Description: "Switch provider", Context: []HelpContext{ResultsContext, EpisodesContext, SeasonsContext}},
	{Key: "d", Description: "Download episode", Context: []HelpContext{ResultsContext, EpisodesContext}},
	{Key: "i", Description: "Show info", Context: []HelpContext{ResultsContext, EpisodesContext}},
	{Key: "s", Description: "Show sources", Context: []HelpContext{ResultsContext, EpisodesContext}},
	{Key: "w", Description: "Share via WatchParty", Context: []HelpContext{ResultsContext, EpisodesContext, SeasonsContext}},
	{Key: "m", Description: "Manga info", Context: []HelpContext{EpisodesContext, SeasonsContext}},

	// AniList context
	{Key: "enter/→", Description: "Play from library", Context: []HelpContext{AniListContext}},
	{Key: "u", Description: "Update status", Context: []HelpContext{AniListContext}},
	{Key: "s", Description: "Update score", Context: []HelpContext{AniListContext}},
	{Key: "p", Description: "Update progress", Context: []HelpContext{AniListContext}},
	{Key: "n", Description: "Search new anime", Context: []HelpContext{AniListContext}},
	{Key: "del", Description: "Delete from library", Context: []HelpContext{AniListContext}},
	{Key: "w", Description: "Filter: watching", Context: []HelpContext{AniListContext}},
	{Key: "a", Description: "Filter: all", Context: []HelpContext{AniListContext}},
	{Key: "/", Description: "Fuzzy search", Context: []HelpContext{AniListContext}},

	// History context
	{Key: "/", Description: "Search history", Context: []HelpContext{HistoryContext}},
	{Key: "0", Description: "Show all media", Context: []HelpContext{HistoryContext}},
	{Key: "1", Description: "Show anime only", Context: []HelpContext{HistoryContext}},
	{Key: "2", Description: "Show movies only", Context: []HelpContext{HistoryContext}},
	{Key: "3", Description: "Show TV shows only", Context: []HelpContext{HistoryContext}},
	{Key: "r", Description: "Sort by recent", Context: []HelpContext{HistoryContext}},
	{Key: "t", Description: "Sort by title", Context: []HelpContext{HistoryContext}},
	{Key: "p", Description: "Sort by progress", Context: []HelpContext{HistoryContext}},
	{Key: "x", Description: "Delete selected item", Context: []HelpContext{HistoryContext}},
	{Key: "X", Description: "Delete all history", Context: []HelpContext{HistoryContext}},
	{Key: "w", Description: "Share via WatchParty", Context: []HelpContext{HistoryContext}},
	{Key: "enter", Description: "Play selected item", Context: []HelpContext{HistoryContext}},

	// Downloads context
	{Key: "p", Description: "Pause download", Context: []HelpContext{DownloadsContext}},
	{Key: "r", Description: "Resume download", Context: []HelpContext{DownloadsContext}},
	{Key: "c", Description: "Cancel download", Context: []HelpContext{DownloadsContext}},
	{Key: "x", Description: "Clear completed", Context: []HelpContext{DownloadsContext}},
	{Key: "ctrl+r", Description: "Refresh list", Context: []HelpContext{DownloadsContext}},
	{Key: "/", Description: "Filter downloads", Context: []HelpContext{DownloadsContext}},

	// Settings context (for future use)
	{Key: "s", Description: "Save settings", Context: []HelpContext{SettingsContext}},
	{Key: "r", Description: "Reset to defaults", Context: []HelpContext{SettingsContext}},
}

// New creates a new help model
func New() Model {
	return Model{
		context:      GlobalContext,
		visible:      false,
		providerName: "Unknown",
	}
}

// SetProviderName sets the current provider name
func (m *Model) SetProviderName(name string) {
	m.providerName = name
}

// Init initializes the help model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if !m.visible {
			return m, nil
		}
		// Handle scrolling when help is visible (less-style navigation)
		switch msg.String() {
		case "up", "k":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case "down", "j":
			m.scrollOffset++
		case "d", "ctrl+d": // Half page down (like less)
			m.scrollOffset += 10
		case "u", "ctrl+u": // Half page up (like less)
			m.scrollOffset -= 10
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		case "pgup", "b": // Page up (like less uses 'b' for back)
			m.scrollOffset -= 20
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		case "pgdown", "space", "f": // Page down (like less uses space/f)
			m.scrollOffset += 20
		case "home", "g": // Jump to top (like less uses 'g')
			m.scrollOffset = 0
		case "end", "G": // Jump to bottom (like less uses 'G')
			m.scrollOffset = 999999 // Will be clamped in View
		}
	}
	return m, nil
}

// View renders the help panel
func (m Model) View() string {
	if !m.visible {
		return ""
	}

	// If width or height are not set, return empty to avoid rendering issues
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Get relevant shortcuts for current context
	shortcuts := m.getRelevantShortcuts()

	// Build help content
	var content strings.Builder

	// Provider info section (if available)
	if m.providerName != "" && m.providerName != "Unknown" {
		providerInfo := fmt.Sprintf("Current Provider: %s", m.providerName)
		providerStyle := lipgloss.NewStyle().
			Foreground(styles.OxocarbonPurple).
			Bold(true).
			Width(60).
			Align(lipgloss.Center)
		content.WriteString(providerStyle.Render(providerInfo))
		content.WriteString("\n")
	}

	// Navigation instructions at the TOP (always visible)
	navInstructions := styles.AniListHelpStyle.Render("↑/↓ j/k scroll • d/u half page • space/b page • g/G top/bottom • esc/? close")
	content.WriteString(lipgloss.NewStyle().Width(60).Align(lipgloss.Center).Render(navInstructions))
	content.WriteString("\n")

	// Group shortcuts by category
	globalShortcuts := filterByContext(shortcuts, GlobalContext)
	contextShortcuts := filterBySpecificContext(shortcuts, m.context)

	// Render global shortcuts
	if len(globalShortcuts) > 0 {
		header := styles.AniListHeaderStyle.Render("Navigation & General")
		content.WriteString(header)
		content.WriteString("\n")
		for _, sc := range globalShortcuts {
			line := m.renderShortcutLine(sc)
			content.WriteString(line)
			content.WriteString("\n")
		}
	}

	// Render context-specific shortcuts
	if len(contextShortcuts) > 0 {
		contextName := m.getContextName()
		if contextName != "" {
			content.WriteString("\n")
			header := styles.AniListHeaderStyle.Render(contextName + " Actions")
			content.WriteString(header)
			content.WriteString("\n")
			for _, sc := range contextShortcuts {
				line := m.renderShortcutLine(sc)
				content.WriteString(line)
				content.WriteString("\n")
			}
		}
	}

	// No footer needed - instructions are at the top now
	content.WriteString("\n")

	contentStr := content.String()
	contentLines := strings.Split(contentStr, "\n")

	// Calculate available height for content (terminal height - 6 for border/padding/title)
	availableHeight := m.height - 6
	if availableHeight < 10 {
		availableHeight = 10 // Minimum visible lines
	}

	// Apply scrolling
	totalLines := len(contentLines)
	if m.scrollOffset >= totalLines-availableHeight {
		m.scrollOffset = totalLines - availableHeight
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	// Get visible slice of content
	startLine := m.scrollOffset
	endLine := startLine + availableHeight
	if endLine > totalLines {
		endLine = totalLines
	}

	visibleContent := strings.Join(contentLines[startLine:endLine], "\n")

	// Add scroll indicators if needed (show percentage or line range)
	scrollInfo := ""
	if totalLines > availableHeight {
		// Show which lines are visible
		scrollInfo = fmt.Sprintf(" (%d-%d/%d)", startLine+1, endLine, totalLines)
	}

	// Calculate box dimensions based on terminal size
	boxWidth := 64
	if m.width > 0 && m.width < boxWidth+4 {
		boxWidth = m.width - 4
		if boxWidth < 40 {
			boxWidth = 40 // Minimum width
		}
	}

	// Wrap in box with scroll indicator in title
	boxTitle := "KEYBOARD SHORTCUTS" + scrollInfo
	titleBar := lipgloss.NewStyle().
		Foreground(styles.OxocarbonWhite).
		Background(styles.OxocarbonPurple).
		Padding(0, 2).
		Bold(true).
		Width(boxWidth - 4).
		Align(lipgloss.Center).
		Render(boxTitle)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OxocarbonPurple).
		Padding(0, 2).
		Width(boxWidth).
		Render(titleBar + "\n\n" + visibleContent)

	// If the box is taller than the terminal, just return it without centering
	boxHeight := lipgloss.Height(box)
	if m.height > 0 && boxHeight >= m.height {
		return box
	}

	// Center in terminal
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		box,
	)
}

// SetContext sets the current help context
func (m *Model) SetContext(ctx HelpContext) {
	m.context = ctx
}

// Toggle toggles the visibility of the help panel
func (m *Model) Toggle() {
	m.visible = !m.visible
}

// Show shows the help panel
func (m *Model) Show() {
	m.visible = true
	m.scrollOffset = 0 // Reset scroll when showing
}

// Hide hides the help panel
func (m *Model) Hide() {
	m.visible = false
	m.scrollOffset = 0 // Reset scroll when hiding
}

// IsVisible returns whether the help panel is visible
func (m Model) IsVisible() bool {
	return m.visible
}

// getRelevantShortcuts returns shortcuts relevant to the current context
func (m Model) getRelevantShortcuts() []Shortcut {
	var relevant []Shortcut
	for _, sc := range allShortcuts {
		for _, ctx := range sc.Context {
			if ctx == GlobalContext || ctx == m.context {
				relevant = append(relevant, sc)
				break
			}
		}
	}
	return relevant
}

// renderShortcutLine renders a single shortcut line
func (m Model) renderShortcutLine(sc Shortcut) string {
	keyStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonPurple).
		Bold(true).
		Width(18)

	descStyle := lipgloss.NewStyle().
		Foreground(styles.OxocarbonBase05)

	return "  " + keyStyle.Render(sc.Key) + descStyle.Render(sc.Description)
}

// getContextName returns a human-readable name for the current context
func (m Model) getContextName() string {
	switch m.context {
	case HomeContext:
		return "Home"
	case SearchContext:
		return "Search"
	case ResultsContext:
		return "Results"
	case EpisodesContext:
		return "Episodes"
	case SeasonsContext:
		return "Seasons"
	case AniListContext:
		return "AniList"
	case DownloadsContext:
		return "Downloads"
	case SettingsContext:
		return "Settings"
	case HistoryContext:
		return "History"
	default:
		return ""
	}
}

// filterByContext filters shortcuts that include the given context
func filterByContext(shortcuts []Shortcut, ctx HelpContext) []Shortcut {
	var filtered []Shortcut
	for _, sc := range shortcuts {
		// For GlobalContext, we only want shortcuts that are explicitly marked as GlobalContext
		// AND we want to avoid duplicates if we are also showing context-specific shortcuts

		// If we are filtering for GlobalContext, check if the shortcut has GlobalContext
		if ctx == GlobalContext {
			for _, c := range sc.Context {
				if c == GlobalContext {
					filtered = append(filtered, sc)
					break
				}
			}
		} else {
			// For other contexts, we want shortcuts that match the context
			// BUT NOT if they are also GlobalContext (those are handled by the global section)
			isGlobal := false
			for _, c := range sc.Context {
				if c == GlobalContext {
					isGlobal = true
					break
				}
			}

			if !isGlobal {
				for _, c := range sc.Context {
					if c == ctx {
						filtered = append(filtered, sc)
						break
					}
				}
			}
		}
	}
	return filtered
}

// filterBySpecificContext filters shortcuts that are specific to a context (not global)
func filterBySpecificContext(shortcuts []Shortcut, ctx HelpContext) []Shortcut {
	var filtered []Shortcut
	for _, sc := range shortcuts {
		// Skip global shortcuts
		isGlobal := false
		for _, c := range sc.Context {
			if c == GlobalContext {
				isGlobal = true
				break
			}
		}
		if isGlobal {
			continue
		}

		// Include if it matches the context
		for _, c := range sc.Context {
			if c == ctx {
				filtered = append(filtered, sc)
				break
			}
		}
	}
	return filtered
}
