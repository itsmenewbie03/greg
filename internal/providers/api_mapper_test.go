package providers

import (
	"fmt"
	"testing"

	"github.com/justchokingaround/greg/internal/providers/api"
)

func TestSeasonOrdering(t *testing.T) {
	// Create a mock InfoResponse with episodes from multiple seasons in random order
	info := api.InfoResponse{
		ID:    "test-show",
		Title: "Test Show",
		Episodes: []api.APIEpisode{
			{Season: 3, Number: 1, Title: "S3E1"},
			{Season: 1, Number: 1, Title: "S1E1"},
			{Season: 5, Number: 1, Title: "S5E1"},
			{Season: 2, Number: 1, Title: "S2E1"},
			{Season: 4, Number: 1, Title: "S4E1"},
			{Season: 1, Number: 2, Title: "S1E2"},
		},
	}

	details := APIInfoToMediaDetails(info, MediaTypeTV)

	// Verify we have 5 seasons
	if len(details.Seasons) != 5 {
		t.Errorf("Expected 5 seasons, got %d", len(details.Seasons))
	}

	// Verify they're in ascending order
	expectedOrder := []int{1, 2, 3, 4, 5}
	for i, season := range details.Seasons {
		if season.Number != expectedOrder[i] {
			t.Errorf("Season at index %d: expected number %d, got %d", i, expectedOrder[i], season.Number)
		}
	}

	// Verify each season has the correct ID format
	for _, season := range details.Seasons {
		expectedID := fmt.Sprintf("test-show-season-%d", season.Number)
		if season.ID != expectedID {
			t.Errorf("Season %d: expected ID %s, got %s", season.Number, expectedID, season.ID)
		}
	}
}

func TestSeasonOrderingWithZeroSeason(t *testing.T) {
	// Test that season 0 is converted to season 1
	info := api.InfoResponse{
		ID:    "test-show-2",
		Title: "Test Show 2",
		Episodes: []api.APIEpisode{
			{Season: 0, Number: 1, Title: "S0E1"}, // Should become season 1
			{Season: 2, Number: 1, Title: "S2E1"},
		},
	}

	details := APIInfoToMediaDetails(info, MediaTypeTV)

	// Should have 2 seasons (1 and 2)
	if len(details.Seasons) != 2 {
		t.Errorf("Expected 2 seasons, got %d", len(details.Seasons))
	}

	// Verify order
	expectedOrder := []int{1, 2}
	for i, season := range details.Seasons {
		if season.Number != expectedOrder[i] {
			t.Errorf("Season at index %d: expected number %d, got %d", i, expectedOrder[i], season.Number)
		}
	}
}

func TestNoSeasonsForMovie(t *testing.T) {
	// Movies should not have seasons
	info := api.InfoResponse{
		ID:    "test-movie",
		Title: "Test Movie",
		Episodes: []api.APIEpisode{
			{Season: 1, Number: 1, Title: "Movie"},
		},
	}

	details := APIInfoToMediaDetails(info, MediaTypeMovie)

	// Should have no seasons
	if len(details.Seasons) != 0 {
		t.Errorf("Expected 0 seasons for movie, got %d", len(details.Seasons))
	}
}
