package utils

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// TruncateToLines truncates text to a maximum number of lines with proper word wrapping.
// The last line will be truncated with "..." if it exceeds the limit.
func TruncateToLines(text string, maxLines int, maxWidth int) string {
	lines := WrapText(text, maxWidth)
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}

	// Take all but the last line
	result := strings.Join(lines[:maxLines-1], "\n")

	// Truncate the last line
	lastLine := lines[maxLines-1]
	if runewidth.StringWidth(lastLine) > maxWidth-3 {
		lastLine = TruncateWithWidth(lastLine, maxWidth)
	} else {
		lastLine += "..."
	}

	return result + "\n" + lastLine
}

// WrapText wraps text at word boundaries to fit within maxWidth.
// Returns a slice of lines.
func WrapText(text string, maxWidth int) []string {
	// Clean text first
	text = strings.TrimSpace(text)
	words := strings.Fields(text)

	var lines []string
	var currentLine strings.Builder
	currentWidth := 0

	for _, word := range words {
		wordWidth := runewidth.StringWidth(word)
		spaceWidth := 1

		if currentWidth == 0 {
			// First word on line
			currentLine.WriteString(word)
			currentWidth = wordWidth
		} else if currentWidth+spaceWidth+wordWidth <= maxWidth {
			// Word fits on current line
			currentLine.WriteString(" ")
			currentLine.WriteString(word)
			currentWidth += spaceWidth + wordWidth
		} else {
			// Start new line
			lines = append(lines, currentLine.String())
			currentLine.Reset()
			currentLine.WriteString(word)
			currentWidth = wordWidth
		}
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
}

// TruncateWithWidth truncates text to fit within maxWidth, accounting for Unicode character widths.
// Adds "..." if the text is truncated.
func TruncateWithWidth(text string, maxWidth int) string {
	if runewidth.StringWidth(text) <= maxWidth {
		return text
	}

	width := 0
	for i, r := range text {
		width += runewidth.RuneWidth(r)
		if width > maxWidth-3 {
			return text[:i] + "..."
		}
	}
	return text
}
