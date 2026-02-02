package mangainfo

import "github.com/charmbracelet/lipgloss"

type Styles struct {
	// You can define specific styles for your mangainfo component here.
	// For now, it will use global styles.
	InfoBox lipgloss.Style
}

func DefaultStyles() *Styles {
	return &Styles{
		InfoBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1).
			MarginTop(1),
	}
}
