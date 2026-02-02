package utils

import (
	"testing"

	"github.com/justchokingaround/greg/internal/providers"
)

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		name     string
		s1       string
		s2       string
		expected int
	}{
		{"identical strings", "hello", "hello", 0},
		{"empty strings", "", "", 0},
		{"one empty", "hello", "", 5},
		{"single char diff", "hello", "hallo", 1},
		{"multiple diffs", "kitten", "sitting", 3},
		{"completely different", "abc", "xyz", 3},
		{"case sensitive", "Hello", "hello", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LevenshteinDistance(tt.s1, tt.s2)
			if result != tt.expected {
				t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d",
					tt.s1, tt.s2, result, tt.expected)
			}
		})
	}
}

func TestSimilarityScore(t *testing.T) {
	tests := []struct {
		name     string
		title1   string
		title2   string
		minScore float64 // Minimum expected score
	}{
		{"identical", "Attack on Titan", "Attack on Titan", 1.0},
		{"with season", "Attack on Titan", "Attack on Titan Season 2", 0.8},
		{"case diff", "attack on titan", "ATTACK ON TITAN", 1.0},
		{"similar", "Demon Slayer", "Demon Slayers", 0.9},
		{"very different", "Naruto", "One Piece", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := SimilarityScore(tt.title1, tt.title2)
			if score < tt.minScore {
				t.Errorf("SimilarityScore(%q, %q) = %.2f, want >= %.2f",
					tt.title1, tt.title2, score, tt.minScore)
			}
		})
	}
}

func TestFindBestMatches(t *testing.T) {
	results := []providers.Media{
		{ID: "1", Title: "Attack on Titan"},
		{ID: "2", Title: "Attack on Titan Season 2"},
		{ID: "3", Title: "Shingeki no Kyojin"},
		{ID: "4", Title: "Naruto"},
	}

	tests := []struct {
		name          string
		query         string
		minScore      float64
		expectedCount int
		expectFirst   string // Expected ID of first result
	}{
		{
			name:          "exact match",
			query:         "Attack on Titan",
			minScore:      0.5,
			expectedCount: 2,
			expectFirst:   "1", // Exact match should be first
		},
		{
			name:          "high threshold",
			query:         "Attack on Titan",
			minScore:      0.95,
			expectedCount: 2, // Both "Attack on Titan" and "...Season 2" normalize to same
			expectFirst:   "1",
		},
		{
			name:          "low threshold",
			query:         "Attack",
			minScore:      0.3,
			expectedCount: 2, // Should match both Attack on Titan entries
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := FindBestMatches(tt.query, results, tt.minScore)

			if len(matches) != tt.expectedCount {
				t.Errorf("FindBestMatches() returned %d results, want %d",
					len(matches), tt.expectedCount)
			}

			if len(matches) > 0 && tt.expectFirst != "" {
				if matches[0].Media.ID != tt.expectFirst {
					t.Errorf("First match has ID %q, want %q",
						matches[0].Media.ID, tt.expectFirst)
				}
			}
		})
	}
}

func TestFindBestMatch(t *testing.T) {
	results := []providers.Media{
		{ID: "1", Title: "Attack on Titan"},
		{ID: "2", Title: "Attack on Titan Season 2"},
		{ID: "3", Title: "Naruto"},
	}

	tests := []struct {
		name        string
		query       string
		minScore    float64
		expectMatch bool
		expectID    string
	}{
		{
			name:        "found exact",
			query:       "Attack on Titan",
			minScore:    0.5,
			expectMatch: true,
			expectID:    "1",
		},
		{
			name:        "not found",
			query:       "One Piece",
			minScore:    0.9,
			expectMatch: false,
		},
		{
			name:        "found with season",
			query:       "Attack on Titan Season 2",
			minScore:    0.5,
			expectMatch: true,
			expectID:    "2", // Exact match on original title
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := FindBestMatch(tt.query, results, tt.minScore)

			if tt.expectMatch && match == nil {
				t.Error("Expected to find a match, but got nil")
			}

			if !tt.expectMatch && match != nil {
				t.Error("Expected no match, but got one")
			}

			if match != nil && tt.expectID != "" {
				if match.Media.ID != tt.expectID {
					t.Errorf("Match has ID %q, want %q",
						match.Media.ID, tt.expectID)
				}
			}
		})
	}
}
