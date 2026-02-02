package styles

import "github.com/charmbracelet/lipgloss"

// Oxocarbon color scheme - IBM Carbon inspired
// Following base16 oxocarbon-dark palette
var (
	// Base colors
	OxocarbonBlack  = lipgloss.Color("#161616") // Darkest background
	OxocarbonBase00 = lipgloss.Color("#262626") // UI elements (lighter than bg)
	OxocarbonBase01 = lipgloss.Color("#393939") // Borders, secondary UI
	OxocarbonBase02 = lipgloss.Color("#525252") // Disabled/muted elements
	OxocarbonBase03 = lipgloss.Color("#767676") // Disabled/muted elements
	OxocarbonBase04 = lipgloss.Color("#dde1e6") // Secondary foreground
	OxocarbonBase05 = lipgloss.Color("#f2f4f8") // Primary foreground
	OxocarbonBase06 = lipgloss.Color("#ffffff") // Brightest foreground
	OxocarbonWhite  = lipgloss.Color("#ffffff")

	// Accent colors
	OxocarbonTeal      = lipgloss.Color("#3ddbd9") // base08
	OxocarbonBlue      = lipgloss.Color("#78a9ff") // base09
	OxocarbonPink      = lipgloss.Color("#ee5396") // base0A
	OxocarbonRed       = lipgloss.Color("#ff5252") // Red
	OxocarbonCyan      = lipgloss.Color("#33b1ff") // base0B
	OxocarbonMagenta   = lipgloss.Color("#ff7eb6") // base0C
	OxocarbonGreen     = lipgloss.Color("#42be65") // base0D
	OxocarbonPurple    = lipgloss.Color("#be95ff") // base0E - main accent
	OxocarbonLightBlue = lipgloss.Color("#82cfff") // base0F
	OxocarbonMauve     = lipgloss.Color("#d1aaff")

	// Status colors using oxocarbon palette
	StatusWatching  = OxocarbonGreen  // Green for current
	StatusCompleted = OxocarbonBlue   // Blue for completed
	StatusOnHold    = OxocarbonPink   // Pink for paused
	StatusDropped   = OxocarbonPink   // Pink/red for dropped
	StatusPlanning  = OxocarbonPurple // Purple for planning
)

var (
	// App general style with a subtle border
	AppStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(OxocarbonBase01)

	// Title style
	TitleStyle = lipgloss.NewStyle().
			Foreground(OxocarbonWhite).
			Background(OxocarbonPurple).
			Padding(0, 1).
			Bold(true)

	// Subtitle style
	SubtitleStyle = lipgloss.NewStyle().
			Foreground(OxocarbonMauve).
			Bold(true)

	// Help style
	HelpStyle = lipgloss.NewStyle().
			Foreground(OxocarbonBase03).
			Italic(true)

	// List styles
	NormalItemStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(OxocarbonBase05)

	SelectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(OxocarbonPurple).
				Bold(true)

	ActiveItemStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(OxocarbonGreen).
			Bold(true)

	// List item with oxocarbon border (mangal style)
	AniListItemStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(OxocarbonBase02).
				BorderLeft(true).
				BorderTop(false).
				BorderRight(false).
				BorderBottom(false).
				PaddingLeft(2).
				PaddingRight(2).
				MarginLeft(3)

	// Selected item with highlighted border
	AniListItemSelectedStyle = lipgloss.NewStyle().
					BorderStyle(lipgloss.ThickBorder()).
					BorderForeground(OxocarbonPurple).
					BorderLeft(true).
					BorderTop(false).
					BorderRight(false).
					BorderBottom(false).
					PaddingLeft(2).
					PaddingRight(2).
					MarginLeft(3)

	// Title style with primary foreground
	AniListTitleStyle = lipgloss.NewStyle().
				Foreground(OxocarbonBase05).
				Bold(true)

	// Subtitle/metadata style - slightly muted but still readable
	AniListMetadataStyle = lipgloss.NewStyle().
				Foreground(OxocarbonBase04)

	// URL/link style
	AniListURLStyle = lipgloss.NewStyle().
			Foreground(OxocarbonCyan).
			Italic(true)

	// Progress bar style
	AniListProgressStyle = lipgloss.NewStyle().
				Foreground(OxocarbonPurple)

	// Score style
	AniListScoreStyle = lipgloss.NewStyle().
				Foreground(OxocarbonPink).
				Bold(true)

	// Header style
	AniListHeaderStyle = lipgloss.NewStyle().
				Foreground(OxocarbonPurple).
				Bold(true).
				Underline(true).
				MarginBottom(1).
				MarginTop(1)

	// Help text style - muted
	AniListHelpStyle = lipgloss.NewStyle().
				Foreground(OxocarbonBase03).
				MarginTop(1)

	// Status badge styles
	StatusBadgeStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Bold(true)

	// Category header (like "Mangas" in mangal)
	CategoryHeaderStyle = lipgloss.NewStyle().
				Foreground(OxocarbonBase05).
				Background(OxocarbonBase01).
				Padding(0, 1).
				Bold(true).
				MarginTop(1).
				MarginBottom(1)

	// Genre badge - pill-shaped tags
	GenreBadgeStyle = lipgloss.NewStyle().
			Foreground(OxocarbonBase05).
			Background(OxocarbonBase01).
			Padding(0, 1).
			MarginRight(1)

	// Selected genre badge (purple accent for selected items)
	GenreBadgeSelectedStyle = lipgloss.NewStyle().
				Foreground(OxocarbonPurple).
				Background(OxocarbonBase01).
				Padding(0, 1).
				MarginRight(1)

	// Synopsis style - italic, muted for readability
	SynopsisStyle = lipgloss.NewStyle().
			Foreground(OxocarbonBase04).
			Italic(true)

	// Home separator for visual grouping
	HomeSeparatorStyle = lipgloss.NewStyle().
				Foreground(OxocarbonBase02).
				MarginTop(1).
				MarginBottom(1)

	// Footer style for status messages
	FooterStyle = lipgloss.NewStyle().
			Foreground(OxocarbonBase05).
			Background(OxocarbonBase01).
			Padding(0, 1)

	// Popup style
	PopupStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(OxocarbonPurple).
			Padding(1, 2).
			Background(OxocarbonBase00).
			Foreground(OxocarbonBase05)
)

// GetStatusColor returns the color for a given watch status
func GetStatusColor(status string) lipgloss.Color {
	switch status {
	case "CURRENT", "watching", "reading", "Watching", "Reading":
		return StatusWatching
	case "COMPLETED", "completed", "Completed":
		return StatusCompleted
	case "PAUSED", "on_hold", "Paused", "On Hold":
		return StatusOnHold
	case "DROPPED", "dropped", "Dropped":
		return StatusDropped
	case "PLANNING", "plan_to_watch", "Plan to Watch", "Plan to Read":
		return StatusPlanning
	case "REPEATING", "rewatching", "rereading", "Rewatching", "Rereading":
		return StatusWatching // Or maybe a different color for rewatching? Using Watching color for now.
	default:
		return lipgloss.Color("#A0AEC0")
	}
}

// FormatStatusBadge creates a colored status badge
func FormatStatusBadge(status string) string {
	color := GetStatusBadgeColor(status)
	return StatusBadgeStyle.Foreground(color).Render(status)
}

// GetStatusBadgeColor returns the color for a given watch status for badges
func GetStatusBadgeColor(status string) lipgloss.Color {
	return GetStatusColor(status)
}
