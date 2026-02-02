package downloader

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/justchokingaround/greg/internal/providers"
)

// ParseTemplate parses a filename template and replaces variables with task data
// Supported variables:
//
//	{title} - Media title
//	{episode} or {episode:03d} - Episode number (with optional padding)
//	{season} or {season:02d} - Season number (with optional padding)
//	{quality} - Quality string
//	{provider} - Provider name
//	{year} - Year (for movies, if available in title)
func ParseTemplate(template string, task DownloadTask) (string, error) {
	if template == "" {
		return "", fmt.Errorf("template cannot be empty")
	}

	result := template

	// Extract year from title if present (e.g., "Movie Title (2024)")
	year := extractYear(task.MediaTitle)

	// Replace {title} - remove year from title if present
	title := task.MediaTitle
	if year != "" {
		title = regexp.MustCompile(`\s*\(\d{4}\)\s*`).ReplaceAllString(title, "")
		title = strings.TrimSpace(title)
	}
	result = strings.ReplaceAll(result, "{title}", title)

	// Replace {year}
	if year != "" {
		result = strings.ReplaceAll(result, "{year}", year)
	} else {
		result = strings.ReplaceAll(result, "{year}", "")
	}

	// Clean up empty parentheses and brackets that might have been left over
	// e.g. "Title () [1080p]" -> "Title [1080p]"
	result = regexp.MustCompile(`\(\s*\)`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`\[\s*\]`).ReplaceAllString(result, "")

	// Clean up double spaces created by removals
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	result = strings.TrimSpace(result)

	// Replace {provider}
	result = strings.ReplaceAll(result, "{provider}", task.Provider)

	// Replace {quality}
	result = strings.ReplaceAll(result, "{quality}", string(task.Quality))

	// Replace {episode} with optional padding format
	result = replaceNumberTemplate(result, "episode", task.Episode)

	// Replace {season} with optional padding format
	result = replaceNumberTemplate(result, "season", task.Season)

	// Sanitize the filename
	result = SanitizeFilename(result)

	// Add file extension based on quality/type
	// Most streaming sources are MP4 or MKV
	if !strings.HasSuffix(result, ".mp4") && !strings.HasSuffix(result, ".mkv") {
		// Use mkv for files with subtitles to embed, mp4 otherwise
		if task.EmbedSubs && len(task.Subtitles) > 0 {
			result += ".mkv"
		} else {
			result += ".mp4"
		}
	}

	return result, nil
}

// replaceNumberTemplate replaces number templates like {episode} or {episode:03d}
func replaceNumberTemplate(template, variable string, value int) string {
	// Pattern matches {variable} or {variable:format}
	pattern := regexp.MustCompile(fmt.Sprintf(`\{%s(?::(\d+)d)?\}`, variable))

	result := pattern.ReplaceAllStringFunc(template, func(match string) string {
		// Extract format if present
		matches := pattern.FindStringSubmatch(match)
		if len(matches) > 1 && matches[1] != "" {
			// Has padding format like {episode:03d}
			padding, err := strconv.Atoi(matches[1])
			if err != nil {
				padding = 0
			}
			// Format with zero padding
			format := fmt.Sprintf("%%0%dd", padding)
			return fmt.Sprintf(format, value)
		}
		// No padding, just return the number
		return strconv.Itoa(value)
	})

	return result
}

// extractYear extracts a 4-digit year from a title string
// Looks for patterns like "(2024)" or "2024"
func extractYear(title string) string {
	// Look for year in parentheses first (more reliable)
	yearPattern := regexp.MustCompile(`\((\d{4})\)`)
	if matches := yearPattern.FindStringSubmatch(title); len(matches) > 1 {
		year, _ := strconv.Atoi(matches[1])
		// Sanity check: year should be reasonable (1900-2100)
		if year >= 1900 && year <= 2100 {
			return matches[1]
		}
	}
	return ""
}

// SanitizeFilename removes or replaces invalid characters from a filename
// Replaces filesystem-unsafe characters with safe alternatives
func SanitizeFilename(filename string) string {
	// Replace common problematic characters
	replacements := map[rune]string{
		'/':  "-",  // Path separator
		'\\': "-",  // Windows path separator
		':':  " -", // Colon (problematic on Windows)
		'*':  "",   // Wildcard
		'?':  "",   // Wildcard
		'"':  "'",  // Quote
		'<':  "",   // Redirect
		'>':  "",   // Redirect
		'|':  "-",  // Pipe
		'\n': " ",  // Newline
		'\r': " ",  // Carriage return
		'\t': " ",  // Tab
	}

	var result strings.Builder
	result.Grow(len(filename))

	for _, ch := range filename {
		if replacement, exists := replacements[ch]; exists {
			result.WriteString(replacement)
		} else if !unicode.IsPrint(ch) {
			// Skip non-printable characters
			continue
		} else {
			result.WriteRune(ch)
		}
	}

	cleaned := result.String()

	// Collapse multiple spaces
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")

	// Trim spaces and dots from start/end (problematic on Windows)
	cleaned = strings.Trim(cleaned, " .")

	// Ensure filename is not empty
	if cleaned == "" {
		cleaned = "download"
	}

	// Limit filename length to 200 characters (leaving room for extension and path)
	if len(cleaned) > 200 {
		// Try to cut at a word boundary
		cleaned = cleaned[:200]
		if lastSpace := strings.LastIndex(cleaned, " "); lastSpace > 150 {
			cleaned = cleaned[:lastSpace]
		}
		cleaned = strings.TrimRight(cleaned, " .-")
	}

	return cleaned
}

// EnsureUniqueFilename ensures the filename is unique by appending a number if necessary
// Returns the original path if unique, or path with (1), (2), etc. if file exists
func EnsureUniqueFilename(path string) string {
	// If file doesn't exist, use it as-is
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	// File exists, find a unique name
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	nameWithoutExt := strings.TrimSuffix(filepath.Base(path), ext)

	for i := 1; i < 1000; i++ {
		newName := fmt.Sprintf("%s (%d)%s", nameWithoutExt, i, ext)
		newPath := filepath.Join(dir, newName)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}

	// If we somehow have 1000 files with the same name, give up and overwrite
	return path
}

// GetTemplateForMediaType returns the appropriate filename template based on media type
func GetTemplateForMediaType(mediaType providers.MediaType, animeTemplate, movieTemplate, tvTemplate string) string {
	switch mediaType {
	case providers.MediaTypeAnime:
		return animeTemplate
	case providers.MediaTypeMovie:
		return movieTemplate
	case providers.MediaTypeTV:
		return tvTemplate
	default:
		// Default to TV show template
		return tvTemplate
	}
}

// ValidateTemplate checks if a template string is valid
// Returns an error if the template contains invalid syntax
func ValidateTemplate(template string) error {
	if template == "" {
		return fmt.Errorf("template cannot be empty")
	}

	// Check for balanced braces
	openBraces := strings.Count(template, "{")
	closeBraces := strings.Count(template, "}")
	if openBraces != closeBraces {
		return fmt.Errorf("unbalanced braces in template: %d open, %d close", openBraces, closeBraces)
	}

	// Check for valid variable names
	validVars := map[string]bool{
		"title":    true,
		"episode":  true,
		"season":   true,
		"quality":  true,
		"provider": true,
		"year":     true,
	}

	// Extract all variables
	varPattern := regexp.MustCompile(`\{([a-z]+)(?::[^}]+)?\}`)
	matches := varPattern.FindAllStringSubmatch(template, -1)
	for _, match := range matches {
		if len(match) > 1 {
			varName := match[1]
			if !validVars[varName] {
				return fmt.Errorf("invalid template variable: {%s}", varName)
			}
		}
	}

	return nil
}
