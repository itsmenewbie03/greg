package scraper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const anilistURL = "https://graphql.anilist.co"

type anilistResponse struct {
	Data struct {
		Media anilistMedia `json:"Media"`
	} `json:"data"`
}

type anilistMedia struct {
	Title     anilistTitle     `json:"title"`
	Relations anilistRelations `json:"relations"`
}

type anilistTitle struct {
	Romaji  string `json:"romaji"`
	English string `json:"english"`
	Native  string `json:"native"`
}

type anilistRelations struct {
	Edges []anilistEdge `json:"edges"`
}

type anilistEdge struct {
	RelationType string      `json:"relationType"`
	Node         anilistNode `json:"node"`
}

type anilistNode struct {
	Title anilistTitle `json:"title"`
}

func getAnimeParentSeries(animeTitle string) (string, error) {
	query := `
    query ($search: String) {
      Media (search: $search, type: ANIME) {
        title {
          romaji
          english
          native
        }
        relations {
          edges {
            relationType(version: 2)
            node {
              title {
                romaji
                english
                native
              }
            }
          }
        }
      }
    }
    `

	variables := map[string]interface{}{
		"search": animeTitle,
	}

	requestBody, err := json.Marshal(map[string]interface{}{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal graphQL request body: %w", err)
	}

	resp, err := http.Post(anilistURL, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to send request to anilist: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anilist API returned non-200 status code: %d", resp.StatusCode)
	}

	var anilistResp anilistResponse
	if err := json.NewDecoder(resp.Body).Decode(&anilistResp); err != nil {
		return "", fmt.Errorf("failed to decode anilist response: %w", err)
	}

	if len(anilistResp.Data.Media.Relations.Edges) > 0 {
		for _, edge := range anilistResp.Data.Media.Relations.Edges {
			// Also check for "SOURCE" in case it's a direct adaptation.
			if edge.RelationType == "PREQUEL" || edge.RelationType == "PARENT" || edge.RelationType == "SOURCE" {
				if edge.Node.Title.English != "" {
					// Prefer English title if available and not the same as original
					if !strings.EqualFold(edge.Node.Title.English, animeTitle) {
						return edge.Node.Title.English, nil
					}
				}
				if edge.Node.Title.Romaji != "" {
					if !strings.EqualFold(edge.Node.Title.Romaji, animeTitle) {
						return edge.Node.Title.Romaji, nil
					}
				}
			}
		}
	}

	return "", nil // No parent found
}
