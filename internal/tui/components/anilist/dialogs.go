package anilist

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/justchokingaround/greg/internal/tui/styles"
)

// DialogMode represents which dialog is open
type DialogMode int

const (
	DialogNone DialogMode = iota
	DialogStatus
	DialogScore
	DialogProgress
	DialogAddToList
	DialogDelete
)

// StatusOption represents a status choice
type StatusOption struct {
	Value   string
	Display string
}

var statusOptions = []StatusOption{
	{"CURRENT", "Watching"},
	{"COMPLETED", "Completed"},
	{"PAUSED", "On Hold"},
	{"DROPPED", "Dropped"},
	{"PLANNING", "Plan to Watch"},
	{"REPEATING", "Rewatching"},
}

// Dialog manager fields (add to Model)
type DialogState struct {
	Mode          DialogMode
	StatusIndex   int
	ScoreInput    textinput.Model
	ProgressInput textinput.Model
}

// RenderStatusDialog renders the status selection dialog
func RenderStatusDialog(currentStatus string, selectedIndex int, mediaType string) string {
	var output string

	// Title
	title := styles.AniListHeaderStyle.Render("Update Status")
	output += title + "\n\n"

	// Options
	for i, opt := range statusOptions {
		var prefix string
		if i == selectedIndex {
			prefix = lipgloss.NewStyle().Foreground(styles.OxocarbonPurple).Render("▸ ")
		} else {
			prefix = "  "
		}

		optionStyle := lipgloss.NewStyle()
		if i == selectedIndex {
			optionStyle = optionStyle.Foreground(styles.OxocarbonPurple).Bold(true)
		} else if opt.Value == currentStatus {
			optionStyle = optionStyle.Foreground(styles.OxocarbonBase04)
		}

		displayText := formatStatusText(opt.Value, mediaType)
		output += prefix + optionStyle.Render(displayText) + "\n"
	}

	output += "\n" + styles.AniListHelpStyle.Render("↑/↓ navigate • enter confirm • esc cancel")

	// Box it
	boxStyle := styles.PopupStyle.Width(40)

	return boxStyle.Render(output)
}

// RenderScoreDialog renders the score input dialog
func (m Model) RenderScoreDialog(currentScore float64, input textinput.Model) string {
	var output string

	// Title
	title := styles.AniListHeaderStyle.Render("Update Score")
	output += title + "\n\n"

	// Current score
	if currentScore > 0 {
		output += styles.AniListMetadataStyle.Render(fmt.Sprintf("Current: %.1f/10", currentScore)) + "\n\n"
	}

	// Input
	output += styles.AniListTitleStyle.Render("New score (0-10):") + "\n"
	output += input.View() + "\n\n"

	output += styles.AniListHelpStyle.Render("enter confirm • esc cancel")

	// Box it
	boxStyle := styles.PopupStyle.Width(40)

	return boxStyle.Render(output)
}

// RenderProgressDialog renders the progress input dialog
func (m Model) RenderProgressDialog(currentProgress, totalEpisodes int, input textinput.Model) string {
	var output string

	// Title
	title := styles.AniListHeaderStyle.Render("Update Progress")
	output += title + "\n\n"

	// Current progress
	progressStr := fmt.Sprintf("Current: %d", currentProgress)
	if totalEpisodes > 0 {
		progressStr += fmt.Sprintf("/%d episodes", totalEpisodes)
	} else {
		progressStr += " episodes"
	}
	output += styles.AniListMetadataStyle.Render(progressStr) + "\n\n"

	// Input
	output += styles.AniListTitleStyle.Render("Episodes watched:") + "\n"
	output += input.View() + "\n\n"

	// Quick buttons
	output += styles.AniListMetadataStyle.Render("Quick: ") + "\n"
	output += styles.AniListMetadataStyle.Render(fmt.Sprintf("  +1: %d  ", currentProgress+1))
	if totalEpisodes > 0 && currentProgress < totalEpisodes {
		output += styles.AniListMetadataStyle.Render(fmt.Sprintf("  All: %d  ", totalEpisodes))
	}
	output += "\n\n"

	helpText := "enter confirm • +/- adjust"
	if totalEpisodes > 0 {
		helpText += " • a all"
	}
	helpText += " • esc cancel"
	output += styles.AniListHelpStyle.Render(helpText)

	// Box it
	boxStyle := styles.PopupStyle.Width(50)

	return boxStyle.Render(output)
}

// InitDialogState initializes dialog state
func InitDialogState() DialogState {
	scoreInput := textinput.New()
	scoreInput.Placeholder = "0.0"
	scoreInput.CharLimit = 4
	scoreInput.Width = 20

	progressInput := textinput.New()
	progressInput.Placeholder = "0"
	progressInput.CharLimit = 5
	progressInput.Width = 20

	return DialogState{
		Mode:          DialogNone,
		ScoreInput:    scoreInput,
		ProgressInput: progressInput,
	}
}

// ParseScore parses score from string
func ParseScore(s string) (float64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty score")
	}
	score, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if score < 0 || score > 10 {
		return 0, fmt.Errorf("score must be between 0 and 10")
	}
	return score, nil
}

// RenderAddToListDialog renders the add to list dialog
func RenderAddToListDialog(animeTitle string, statusOptions []string, selectedIndex int, mediaType string) string {
	var outputLines []string

	// Title
	outputLines = append(outputLines, styles.AniListHeaderStyle.Render("Add to Anilist"))
	outputLines = append(outputLines, "") // empty line

	outputLines = append(outputLines, styles.AniListMetadataStyle.Render(fmt.Sprintf("How would you like to add \"%s\"?", animeTitle)))
	outputLines = append(outputLines, "") // empty line

	// Options - create each option as a properly formatted single line
	for i, option := range statusOptions {
		var line string
		if i == selectedIndex {
			// Selected option
			indicatorStyle := lipgloss.NewStyle().Foreground(styles.OxocarbonPurple)
			textStyle := lipgloss.NewStyle().Foreground(styles.OxocarbonPurple).Bold(true)
			line = indicatorStyle.Render("▸ ") + textStyle.Render(formatStatusText(option, mediaType))
		} else {
			// Unselected option
			line = "  " + formatStatusText(option, mediaType)
		}
		outputLines = append(outputLines, line)
	}

	outputLines = append(outputLines, "") // empty line
	outputLines = append(outputLines, styles.AniListHelpStyle.Render("↑/↓ navigate • enter confirm • esc cancel"))

	// Join all lines with newline
	fullOutput := strings.Join(outputLines, "\n")

	// Box it like the other dialogs
	boxStyle := styles.PopupStyle.
		Padding(0). // Use no internal padding, handle spacing in content
		Width(70).
		Margin(0) // Ensure no margin issues

	finalOutput := lipgloss.NewStyle().Padding(1, 2).Render(fullOutput)

	return boxStyle.Render(finalOutput)
}

// formatStatusText converts AniList status to user-friendly text (matches the implementation in model.go)
func formatStatusText(status string, mediaType string) string {
	isManga := strings.EqualFold(mediaType, "manga") || strings.EqualFold(mediaType, "MANGA")

	switch status {
	case "CURRENT":
		if isManga {
			return "Reading"
		}
		return "Watching"
	case "PLANNING":
		if isManga {
			return "Plan to Read"
		}
		return "Plan to Watch"
	case "COMPLETED":
		return "Completed"
	case "PAUSED":
		if isManga {
			return "Paused"
		}
		return "On Hold"
	case "DROPPED":
		return "Dropped"
	case "REPEATING":
		if isManga {
			return "Rereading"
		}
		return "Rewatching"
	default:
		return status
	}
}

// ParseProgress parses progress from string
func ParseProgress(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty progress")
	}
	progress, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if progress < 0 {
		return 0, fmt.Errorf("progress must be positive")
	}
	return progress, nil
}

// RenderDeleteConfirmationDialog renders the delete confirmation dialog
func RenderDeleteConfirmationDialog(mediaTitle string) string {
	var output string

	// Title
	title := styles.AniListHeaderStyle.Render("Confirm Deletion")
	output += title + "\n\n"

	// Message
	output += styles.AniListMetadataStyle.Render(fmt.Sprintf("Are you sure you want to remove \"%s\" from your Anilist?\n\nThis action cannot be undone.", mediaTitle)) + "\n\n"

	// Options
	output += styles.AniListTitleStyle.Render("y - Confirm deletion") + "\n"
	output += styles.AniListMetadataStyle.Render("n - Cancel") + "\n"

	// Box it like the other dialogs
	boxStyle := styles.PopupStyle.Width(50)

	return boxStyle.Render(output)
}
