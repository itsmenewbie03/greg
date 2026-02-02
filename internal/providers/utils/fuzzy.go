package utils

import (
	"strings"

	"github.com/justchokingaround/greg/internal/providers"
)

// LevenshteinDistance calculates the Levenshtein distance between two strings
// It returns the minimum number of single-character edits (insertions, deletions, or substitutions)
// required to change one string into the other
func LevenshteinDistance(s1, s2 string) int {
	len1 := len(s1)
	len2 := len(s2)

	// Create a 2D matrix to store distances
	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
	}

	// Initialize first row and column
	for i := 0; i <= len1; i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len2; j++ {
		matrix[0][j] = j
	}

	// Fill in the rest of the matrix
	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len1][len2]
}

// min returns the minimum of three integers
func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// SimilarityScore calculates a similarity score between two titles (0.0 to 1.0)
// 1.0 means perfect match, 0.0 means completely different
func SimilarityScore(title1, title2 string) float64 {
	// Normalize both titles
	norm1 := NormalizeTitle(title1)
	norm2 := NormalizeTitle(title2)

	// Handle empty strings
	if norm1 == "" || norm2 == "" {
		return 0.0
	}

	// Calculate Levenshtein distance
	distance := LevenshteinDistance(norm1, norm2)

	// Calculate similarity as 1 - (distance / max_length)
	maxLen := len(norm1)
	if len(norm2) > maxLen {
		maxLen = len(norm2)
	}

	similarity := 1.0 - float64(distance)/float64(maxLen)

	// Ensure similarity is between 0 and 1
	if similarity < 0 {
		similarity = 0
	}

	return similarity
}

// MatchResult represents a search result with its similarity score
type MatchResult struct {
	Media   providers.Media
	Score   float64
	IsExact bool
}

// FindBestMatches finds the best matching media items from a list based on title similarity
// Returns sorted list of matches with similarity scores
// minScore is the minimum similarity threshold (0.0 to 1.0)
func FindBestMatches(query string, results []providers.Media, minScore float64) []MatchResult {
	if len(results) == 0 {
		return nil
	}

	matches := make([]MatchResult, 0, len(results))
	normalizedQuery := NormalizeTitle(query)

	for _, media := range results {
		score := SimilarityScore(query, media.Title)

		// Check for exact match on normalized title
		normIsExact := NormalizeTitle(media.Title) == normalizedQuery

		// Also check for exact match on original title (case-insensitive)
		origIsExact := strings.EqualFold(query, media.Title)

		// Consider it exact if either normalized or original matches exactly
		isExact := normIsExact || origIsExact

		if score >= minScore || isExact {
			// Boost score if original title is exact match (add small bonus on top of 1.0)
			// This ensures original exact matches always rank higher than normalized-only matches
			if origIsExact {
				score = 1.1
			}

			matches = append(matches, MatchResult{
				Media:   media,
				Score:   score,
				IsExact: isExact,
			})
		}
	}

	// Sort by score (descending), exact matches first
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			// Prioritize exact matches
			if matches[j].IsExact && !matches[i].IsExact {
				matches[i], matches[j] = matches[j], matches[i]
			} else if matches[i].IsExact == matches[j].IsExact {
				// If both are exact or both are not, sort by score
				if matches[j].Score > matches[i].Score {
					matches[i], matches[j] = matches[j], matches[i]
				}
			}
		}
	}

	return matches
}

// FindBestMatch finds the single best matching media item
// Returns nil if no match meets the minimum score threshold
func FindBestMatch(query string, results []providers.Media, minScore float64) *MatchResult {
	matches := FindBestMatches(query, results, minScore)
	if len(matches) == 0 {
		return nil
	}
	return &matches[0]
}
