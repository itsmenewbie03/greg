package utils

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	yearRegex       = regexp.MustCompile(`\b(19|20)\d{2}\b`)
	whitespaceRegex = regexp.MustCompile(`\s+`)
)

// CleanText removes extra whitespace and trims a string
func CleanText(text string) string {
	// Replace multiple whitespace with single space
	text = whitespaceRegex.ReplaceAllString(text, " ")
	// Trim leading/trailing whitespace
	return strings.TrimSpace(text)
}

// ExtractYear extracts a 4-digit year from a string
// Returns 0 if no year is found
func ExtractYear(text string) int {
	matches := yearRegex.FindString(text)
	if matches == "" {
		return 0
	}

	year, err := strconv.Atoi(matches)
	if err != nil {
		return 0
	}

	return year
}

// ParseInt safely parses a string to int, returning 0 on error
func ParseInt(s string) int {
	val, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return val
}

// ParseFloat safely parses a string to float64, returning 0.0 on error
func ParseFloat(s string) float64 {
	val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0.0
	}
	return val
}

// TruncateString truncates a string to maxLen characters
// If truncated, appends "..." to the end
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// SplitGenres splits a genre string by common delimiters
func SplitGenres(genres string) []string {
	// Split by comma, slash, or pipe
	parts := regexp.MustCompile(`[,/|]`).Split(genres, -1)

	result := make([]string, 0, len(parts))
	for _, part := range parts {
		cleaned := CleanText(part)
		if cleaned != "" {
			result = append(result, cleaned)
		}
	}

	return result
}

// Contains checks if a slice contains a string (case-insensitive)
func Contains(slice []string, item string) bool {
	itemLower := strings.ToLower(item)
	for _, s := range slice {
		if strings.ToLower(s) == itemLower {
			return true
		}
	}
	return false
}

// RemoveDuplicates removes duplicate strings from a slice while preserving order
func RemoveDuplicates(slice []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(slice))

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// DefaultString returns the first non-empty string
func DefaultString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// DefaultInt returns the first non-zero int
func DefaultInt(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

// NormalizeTitle normalizes a title for comparison
// Removes special characters, converts to lowercase, removes season/part patterns
func NormalizeTitle(title string) string {
	// Convert to lowercase
	title = strings.ToLower(title)

	// Remove "season N" patterns
	title = regexp.MustCompile(`\s*season\s+\d+\s*`).ReplaceAllString(title, " ")

	// Remove "part N" patterns
	title = regexp.MustCompile(`\s*part\s+\d+\s*`).ReplaceAllString(title, " ")

	// Remove special characters except spaces and alphanumeric (including unicode)
	result := strings.Builder{}
	for _, r := range title {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' || r > 127 {
			result.WriteRune(r)
		}
	}

	// Clean whitespace
	title = CleanText(result.String())
	return title
}
