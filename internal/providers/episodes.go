package providers

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseEpisodeRange parses an episode range string (e.g., "1-5,7,9-12") and returns
// the matching episodes from the provided list.
//
// Supported formats:
//   - Single episode: "5"
//   - Range: "1-5"
//   - Multiple: "1-5,7,9-12"
func ParseEpisodeRange(allEpisodes []Episode, rangeStr string) ([]Episode, error) {
	var result []Episode

	// Split by commas
	parts := strings.Split(rangeStr, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Check if it's a range (contains -)
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range format: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start number in range: %s", rangeParts[0])
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end number in range: %s", rangeParts[1])
			}

			// Add all episodes in range
			for num := start; num <= end; num++ {
				for _, ep := range allEpisodes {
					if ep.Number == num {
						result = append(result, ep)
						break
					}
				}
			}
		} else {
			// Single episode number
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid episode number: %s", part)
			}

			// Find episode with this number
			for _, ep := range allEpisodes {
				if ep.Number == num {
					result = append(result, ep)
					break
				}
			}
		}
	}

	return result, nil
}
