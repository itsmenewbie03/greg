package common

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/justchokingaround/greg/internal/tui/styles"
	"github.com/sahilm/fuzzy"
)

// FuzzySearch provides fuzzy search functionality for list views
type FuzzySearch struct {
	input  textinput.Model
	active bool
	locked bool // When true, filter is applied but not editable (allows action keys)
	query  string
}

// NewFuzzySearch creates a new fuzzy search component
func NewFuzzySearch() *FuzzySearch {
	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.Prompt = ""
	ti.CharLimit = 200
	ti.PromptStyle = styles.AniListTitleStyle
	ti.TextStyle = styles.AniListMetadataStyle
	ti.PlaceholderStyle = styles.AniListMetadataStyle

	return &FuzzySearch{
		input:  ti,
		active: false,
		query:  "",
	}
}

// Activate enables fuzzy search mode
func (f *FuzzySearch) Activate() tea.Cmd {
	f.active = true
	f.locked = false
	f.input.Focus()
	f.input.SetValue("")
	f.query = ""
	return textinput.Blink
}

// Deactivate disables fuzzy search mode
func (f *FuzzySearch) Deactivate() {
	f.active = false
	f.locked = false
	f.input.Blur()
	f.input.SetValue("")
	f.query = ""
}

// Lock locks the filter (stops editing, enables action keys)
func (f *FuzzySearch) Lock() {
	if f.active {
		f.locked = true
		f.input.Blur()
	}
}

// Unlock unlocks the filter (allows editing again)
func (f *FuzzySearch) Unlock() tea.Cmd {
	if f.active {
		f.locked = false
		f.input.Focus()
		return textinput.Blink
	}
	return nil
}

// IsActive returns whether fuzzy search is currently active
func (f *FuzzySearch) IsActive() bool {
	return f.active
}

// IsLocked returns whether the filter is locked (not editable)
func (f *FuzzySearch) IsLocked() bool {
	return f.locked
}

// Query returns the current search query
func (f *FuzzySearch) Query() string {
	return f.query
}

// Update handles input updates for fuzzy search
// Only processes input when active and not locked
func (f *FuzzySearch) Update(msg tea.Msg) tea.Cmd {
	if !f.active || f.locked {
		return nil
	}

	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	f.query = f.input.Value()
	return cmd
}

// View renders the fuzzy search input
func (f *FuzzySearch) View() string {
	if !f.active {
		return ""
	}

	prompt := styles.AniListTitleStyle.Render("┃")
	searchLabel := styles.AniListMetadataStyle.Render("Filter: ")

	if f.locked {
		// Show locked state with the current query
		queryText := styles.AniListTitleStyle.Render(f.query)
		hint := styles.AniListHelpStyle.Render(" (locked • / to edit • esc to clear)")
		return searchLabel + prompt + " " + queryText + hint
	}

	// Show editing state
	hint := styles.AniListHelpStyle.Render(" (esc to lock)")
	return searchLabel + prompt + " " + f.input.View() + hint
}

// SetWidth sets the width of the fuzzy search input
func (f *FuzzySearch) SetWidth(width int) {
	f.input.Width = width - 20
}

// Filter performs fuzzy matching on a list of strings and returns matching indices
// The strings slice should contain the searchable text for each item
func (f *FuzzySearch) Filter(strings []string) []int {
	if !f.active || f.query == "" {
		// Return all indices if not filtering
		indices := make([]int, len(strings))
		for i := range indices {
			indices[i] = i
		}
		return indices
	}

	// Perform fuzzy search
	matches := fuzzy.Find(f.query, strings)

	// Extract matched indices
	indices := make([]int, len(matches))
	for i, match := range matches {
		indices[i] = match.Index
	}

	return indices
}
