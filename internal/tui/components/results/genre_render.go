package results

import (
	"fmt"
	"strings"

	"github.com/justchokingaround/greg/internal/tui/styles"
)

// RenderGenres renders genre tags as styled badges.
// Displays up to maxGenres, with an overflow indicator if there are more.
func RenderGenres(genres []string, selected bool, maxGenres int) string {
	if len(genres) == 0 {
		return ""
	}

	badgeStyle := styles.GenreBadgeStyle
	if selected {
		badgeStyle = styles.GenreBadgeSelectedStyle
	}

	displayGenres := genres
	overflow := false
	if len(genres) > maxGenres {
		displayGenres = genres[:maxGenres]
		overflow = true
	}

	var parts []string
	for _, genre := range displayGenres {
		parts = append(parts, badgeStyle.Render(genre))
	}

	result := strings.Join(parts, " ")

	if overflow {
		moreText := fmt.Sprintf("+%d more", len(genres)-maxGenres)
		result += " " + badgeStyle.Render(moreText)
	}

	return result
}
