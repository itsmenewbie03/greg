package scraper

import (
	"testing"
)

func TestGetMangaInfo(t *testing.T) {
	testCases := []struct {
		name      string
		anime     string
		expectErr bool
	}{
		{
			name:      "Chainsaw Man",
			anime:     "Chainsaw Man",
			expectErr: false,
		},
		{
			name:      "Jujutsu Kaisen",
			anime:     "Jujutsu Kaisen",
			expectErr: false,
		},
		{
			name:      "Non Existent Anime",
			anime:     "Non Existent Anime XYZ",
			expectErr: false, // The function should handle this gracefully and return a "not found" message
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := GetMangaInfo(tc.anime)
			if (err != nil) != tc.expectErr {
				t.Errorf("GetMangaInfo() for %s error = %v, expectErr %v", tc.anime, err, tc.expectErr)
				return
			}
			if !tc.expectErr {
				if info == "" {
					t.Errorf("GetMangaInfo() for %s returned empty info", tc.anime)
				}
				t.Logf("Result for %s:\n%s", tc.anime, info)
			}
		})
	}
}
