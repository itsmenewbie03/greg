package anilist

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.searchInput.Width = m.width - 20
		m.fuzzySearch.SetWidth(m.width)
	case AniListSearchResultMsg:
		// Handle search results
		m.searchResults = msg.Results
		m.currentSearchIndex = 0
		if len(msg.Results) > 0 {
			m.viewMode = ViewSearchResults
		} else {
			// Stay in search view but show no results
			m.viewMode = ViewSearchResults
		}
	case AniListDeleteConfirmationMsg:
		// Store the media to be deleted and show confirmation dialog in main app
		m.animeToAdd = msg.Media // Reusing this field to store the media for deletion confirmation
		// Don't handle here - let the main app handle the dialog
	}

	// Update text input if in search mode
	if m.viewMode == ViewSearch {
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.searchQuery = m.searchInput.Value()
	}

	return m, cmd
}

// handleKeyPress handles keyboard input
func (m Model) handleKeyPress(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch m.viewMode {
	case ViewLibrary, ViewWatching:
		return m.handleLibraryViewKeyPress(msg)
	case ViewSearch:
		return m.handleSearchViewKeyPress(msg)
	case ViewSearchResults:
		return m.handleSearchResultsViewKeyPress(msg)
	default:
		return m.handleLibraryViewKeyPress(msg)
	}
}

// handleLibraryViewKeyPress handles key presses in library view
func (m Model) handleLibraryViewKeyPress(msg tea.KeyMsg) (Model, tea.Cmd) {
	// If info dialog is open, handle it first
	if m.showInfoDialog {
		switch msg.String() {
		case "esc", "i", "enter":
			m.showInfoDialog = false
			m.dialogScroll = 0
			return m, nil
		case "up", "k":
			if m.dialogScroll > 0 {
				m.dialogScroll--
			}
			return m, nil
		case "down", "j":
			m.dialogScroll++
			return m, nil
		}
		return m, nil
	}

	// If fuzzy search is active, handle it based on locked state
	if m.fuzzySearch.IsActive() {
		if m.fuzzySearch.IsLocked() {
			// Filter is locked - allow action keys
			switch msg.String() {
			case "esc":
				// Clear the filter entirely
				m.fuzzySearch.Deactivate()
				m.currentIndex = 0
				return m, nil
			case "/":
				// Unlock to edit filter again
				cmd := m.fuzzySearch.Unlock()
				return m, cmd
			case "i":
				// Show info dialog
				filteredIndices := m.getFilteredLibraryIndices()
				if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
					m.showInfoDialog = true
					m.dialogScroll = 0
				}
				return m, nil
			case "up", "k":
				if m.currentIndex > 0 {
					m.currentIndex--
				}
				return m, nil
			case "down", "j":
				filteredIndices := m.getFilteredLibraryIndices()
				maxIndex := len(filteredIndices) - 1
				if m.currentIndex < maxIndex {
					m.currentIndex++
				}
				return m, nil
			case "enter":
				// Play from filtered results
				filteredIndices := m.getFilteredLibraryIndices()
				if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
					filtered := m.GetFilteredLibrary()
					actualIndex := filteredIndices[m.currentIndex]
					if actualIndex < len(filtered) {
						return m, func() tea.Msg {
							return SelectMediaMsg{Media: &filtered[actualIndex]}
						}
					}
				}
				return m, nil
			case "s":
				// Status update
				filteredIndices := m.getFilteredLibraryIndices()
				if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
					filtered := m.GetFilteredLibrary()
					actualIndex := filteredIndices[m.currentIndex]
					if actualIndex < len(filtered) {
						return m, func() tea.Msg {
							return OpenStatusUpdateMsg{Media: &filtered[actualIndex]}
						}
					}
				}
				return m, nil
			case "r":
				// Score update
				filteredIndices := m.getFilteredLibraryIndices()
				if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
					filtered := m.GetFilteredLibrary()
					actualIndex := filteredIndices[m.currentIndex]
					if actualIndex < len(filtered) {
						return m, func() tea.Msg {
							return OpenScoreUpdateMsg{Media: &filtered[actualIndex]}
						}
					}
				}
				return m, nil
			case "p":
				// Progress update
				filteredIndices := m.getFilteredLibraryIndices()
				if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
					filtered := m.GetFilteredLibrary()
					actualIndex := filteredIndices[m.currentIndex]
					if actualIndex < len(filtered) {
						return m, func() tea.Msg {
							return OpenProgressUpdateMsg{Media: &filtered[actualIndex]}
						}
					}
				}
				return m, nil
			case "d":
				// Delete
				filteredIndices := m.getFilteredLibraryIndices()
				if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
					filtered := m.GetFilteredLibrary()
					actualIndex := filteredIndices[m.currentIndex]
					if actualIndex < len(filtered) {
						return m, func() tea.Msg {
							return AniListDeleteConfirmationMsg{Media: &filtered[actualIndex]}
						}
					}
				}
				return m, nil
			case "m":
				// Remap provider
				filteredIndices := m.getFilteredLibraryIndices()
				if len(filteredIndices) > 0 && m.currentIndex < len(filteredIndices) {
					filtered := m.GetFilteredLibrary()
					actualIndex := filteredIndices[m.currentIndex]
					if actualIndex < len(filtered) {
						return m, func() tea.Msg {
							return RemapRequestedMsg{Media: &filtered[actualIndex]}
						}
					}
				}
				return m, nil
			case "w":
				// Switch to watching filter
				m.statusFilter = "CURRENT"
				m.currentIndex = 0
				m.fuzzySearch.Deactivate()
				return m, nil
			case "a":
				// Switch to all anime
				m.statusFilter = ""
				m.currentIndex = 0
				m.fuzzySearch.Deactivate()
				return m, nil
			case "n":
				// Search new anime
				m.fuzzySearch.Deactivate()
				m.searchInput.SetValue("")
				m.searchQuery = ""
				m.viewMode = ViewSearch
				m.searchInput.Focus()
				return m, textinput.Blink
			case "q":
				// Quit
				m.fuzzySearch.Deactivate()
				return m, tea.Quit
			case "ctrl+r":
				// Refresh
				return m, func() tea.Msg {
					return RefreshLibraryMsg{}
				}
			}
			return m, nil
		} else {
			// Filter is being edited - only handle esc, pass everything else to input
			switch msg.String() {
			case "esc":
				// Lock the filter (stop editing)
				m.fuzzySearch.Lock()
				return m, nil
			default:
				// Pass all keys (including j/k) to fuzzy search input for editing
				cmd := m.fuzzySearch.Update(msg)
				m.currentIndex = 0
				return m, cmd
			}
		}
	}

	// Normal mode (fuzzy search not active)
	filtered := m.GetFilteredLibrary()

	switch {
	case msg.String() == "i":
		// Show info dialog
		if len(filtered) > 0 && m.currentIndex < len(filtered) {
			m.showInfoDialog = true
			m.dialogScroll = 0
		}
		return m, nil

	case msg.String() == "/":
		// Activate fuzzy search
		cmd := m.fuzzySearch.Activate()
		m.currentIndex = 0
		return m, cmd

	case key.Matches(msg, m.keys.Up):
		if m.currentIndex > 0 {
			m.currentIndex--
		}

	case key.Matches(msg, m.keys.Down):
		if m.currentIndex < len(filtered)-1 {
			m.currentIndex++
		}

	case key.Matches(msg, m.keys.FilterWatching):
		m.statusFilter = "CURRENT"
		m.currentIndex = 0

	case key.Matches(msg, m.keys.FilterAll):
		m.statusFilter = ""
		m.currentIndex = 0

	case key.Matches(msg, m.keys.Select):
		// Will be handled by parent to initiate playback
		return m, func() tea.Msg {
			return SelectMediaMsg{Media: m.GetSelectedMedia()}
		}

	case key.Matches(msg, m.keys.UpdateStatus):
		return m, func() tea.Msg {
			return OpenStatusUpdateMsg{Media: m.GetSelectedMedia()}
		}

	case key.Matches(msg, m.keys.UpdateScore):
		return m, func() tea.Msg {
			return OpenScoreUpdateMsg{Media: m.GetSelectedMedia()}
		}

	case key.Matches(msg, m.keys.UpdateProgress):
		return m, func() tea.Msg {
			return OpenProgressUpdateMsg{Media: m.GetSelectedMedia()}
		}

	case key.Matches(msg, m.keys.Refresh):
		return m, func() tea.Msg {
			return RefreshLibraryMsg{}
		}

	case key.Matches(msg, m.keys.Remap):
		return m, func() tea.Msg {
			return RemapRequestedMsg{Media: m.GetSelectedMedia()}
		}

	case key.Matches(msg, m.keys.Back):
		return m, func() tea.Msg {
			return BackMsg{}
		}

	case key.Matches(msg, m.keys.SearchNew):
		// Switch to search view
		m.searchInput.SetValue("")
		m.searchQuery = ""
		m.viewMode = ViewSearch
		m.searchInput.Focus()
		return m, textinput.Blink

	case key.Matches(msg, m.keys.Delete):
		// Get selected media to delete
		selectedMedia := m.GetSelectedMedia()
		if selectedMedia != nil {
			return m, func() tea.Msg {
				return AniListDeleteConfirmationMsg{Media: selectedMedia}
			}
		}
	}

	return m, nil
}

// handleSearchViewKeyPress handles key presses in search view
func (m Model) handleSearchViewKeyPress(msg tea.KeyMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "enter":
		// Submit search
		m.searchQuery = m.searchInput.Value()
		if m.searchQuery != "" {
			m.viewMode = ViewSearchResults
			return m, func() tea.Msg {
				return AniListSearchRequestedMsg{Query: m.searchQuery}
			}
		}
	case "esc":
		// Go back to library view
		m.viewMode = ViewLibrary
		m.searchInput.Blur()
		return m, nil
	}

	// Update text input
	m.searchInput, cmd = m.searchInput.Update(msg)
	m.searchQuery = m.searchInput.Value()

	return m, cmd
}

// handleSearchResultsViewKeyPress handles key presses in search results view
func (m Model) handleSearchResultsViewKeyPress(msg tea.KeyMsg) (Model, tea.Cmd) {
	// If fuzzy search is active, handle it based on locked state
	if m.fuzzySearch.IsActive() {
		if m.fuzzySearch.IsLocked() {
			// Filter is locked - allow action keys
			switch msg.String() {
			case "esc":
				// Clear the filter entirely
				m.fuzzySearch.Deactivate()
				m.currentSearchIndex = 0
				return m, nil
			case "/":
				// Unlock to edit filter again
				cmd := m.fuzzySearch.Unlock()
				return m, cmd
			case "up", "k":
				if m.currentSearchIndex > 0 {
					m.currentSearchIndex--
				}
				return m, nil
			case "down", "j":
				filteredIndices := m.getFilteredSearchIndices()
				maxIndex := len(filteredIndices) - 1
				if m.currentSearchIndex < maxIndex {
					m.currentSearchIndex++
				}
				return m, nil
			case "enter":
				// Add to list from filtered results
				filteredIndices := m.getFilteredSearchIndices()
				if len(filteredIndices) > 0 && m.currentSearchIndex < len(filteredIndices) {
					actualIndex := filteredIndices[m.currentSearchIndex]
					if actualIndex < len(m.searchResults) {
						selectedMedia := m.searchResults[actualIndex]
						return m, func() tea.Msg {
							return AniListAddToListDialogOpenMsg{Media: &selectedMedia}
						}
					}
				}
				return m, nil
			case "q":
				// Quit
				m.fuzzySearch.Deactivate()
				return m, tea.Quit
			}
			return m, nil
		} else {
			// Filter is being edited - only handle esc, pass everything else to input
			switch msg.String() {
			case "esc":
				// Lock the filter (stop editing)
				m.fuzzySearch.Lock()
				return m, nil
			default:
				// Pass all keys (including j/k) to fuzzy search input for editing
				cmd := m.fuzzySearch.Update(msg)
				m.currentSearchIndex = 0
				return m, cmd
			}
		}
	}

	// Normal mode (fuzzy search not active)
	switch {
	case msg.String() == "/":
		// Activate fuzzy search
		cmd := m.fuzzySearch.Activate()
		m.currentSearchIndex = 0
		return m, cmd

	case key.Matches(msg, m.keys.Up):
		if len(m.searchResults) > 0 && m.currentSearchIndex > 0 {
			m.currentSearchIndex--
		}

	case key.Matches(msg, m.keys.Down):
		if len(m.searchResults) > 0 && m.currentSearchIndex < len(m.searchResults)-1 {
			m.currentSearchIndex++
		}

	case key.Matches(msg, m.keys.Select):
		// User selected a search result - ask for status to add to AniList
		if len(m.searchResults) > 0 && m.currentSearchIndex < len(m.searchResults) {
			selectedMedia := m.searchResults[m.currentSearchIndex]
			// Set the anime to add and trigger the add to list dialog in the main app
			return m, func() tea.Msg {
				return AniListAddToListDialogOpenMsg{Media: &selectedMedia}
			}
		}

	case key.Matches(msg, m.keys.Back):
		// Go back to search input or library
		if m.searchQuery != "" {
			// Go back to search input view
			m.viewMode = ViewSearch
			m.searchInput.SetValue(m.searchQuery)
			m.searchInput.Focus()
			return m, textinput.Blink
		} else {
			// Go back to library
			m.viewMode = ViewLibrary
			m.searchInput.Blur()
			return m, nil
		}
	}

	return m, nil
}
