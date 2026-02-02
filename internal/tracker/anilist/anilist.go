package anilist

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tracker"
	"golang.org/x/oauth2"
)

const (
	// AniList API endpoints
	apiEndpoint  = "https://graphql.anilist.co"
	authEndpoint = "https://anilist.co/api/v2/oauth/authorize"

	// Rate limiting
	rateLimitDelay = 1 * time.Second
)

var (
	// tokenEndpoint is a variable to allow mocking in tests
	tokenEndpoint = "https://anilist.co/api/v2/oauth/token"
)

const (
	// AuthMethodBrowser defines the browser callback method credentials
	AuthBrowserClientID     = "32178"
	AuthBrowserClientSecret = "xBOaLrVFPQPQXfTnG7cGLEPEvhkd5UgKyt8HOlj3"
	AuthBrowserRedirectURI  = "http://localhost:8000/oauth/callback"

	// AuthMethodManual defines the manual token copy method credentials (Greg Link)
	AuthManualClientID     = "34154"
	AuthManualClientSecret = "IZjGxXyCzzlysyQMDn4qay1fcm3UnB1SfQOKbzRH"
	AuthManualRedirectURI  = "https://anilist.co/api/v2/oauth/pin"
)

// Client implements the tracker.Tracker interface for AniList
type Client struct {
	clientID     string
	redirectURI  string
	token        *oauth2.Token
	httpClient   *http.Client
	oauth2Config *oauth2.Config
	lastRequest  time.Time
	mu           sync.Mutex

	// Storage callbacks for token persistence
	saveToken func(*oauth2.Token) error
	loadToken func() (*oauth2.Token, error)
}

// Config contains configuration for the AniList client
type Config struct {
	ClientID    string
	RedirectURI string
	HTTPClient  *http.Client
	SaveToken   func(*oauth2.Token) error
	LoadToken   func() (*oauth2.Token, error)
}

// NewClient creates a new AniList client
func NewClient(cfg Config) *Client {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	oauth2Config := &oauth2.Config{
		ClientID:    cfg.ClientID,
		RedirectURL: cfg.RedirectURI,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authEndpoint,
			TokenURL: tokenEndpoint,
		},
	}

	client := &Client{
		clientID:     cfg.ClientID,
		redirectURI:  cfg.RedirectURI,
		httpClient:   cfg.HTTPClient,
		oauth2Config: oauth2Config,
		saveToken:    cfg.SaveToken,
		loadToken:    cfg.LoadToken,
	}

	// Try to load existing token
	if cfg.LoadToken != nil {
		if token, err := cfg.LoadToken(); err == nil {
			client.token = token
		}
	}

	return client
}

// Authenticate initiates the OAuth2 authentication flow
func (c *Client) Authenticate(ctx context.Context) error {
	// Generate authorization URL
	authURL := c.oauth2Config.AuthCodeURL("state", oauth2.AccessTypeOffline)
	return fmt.Errorf("please visit this URL to authenticate: %s", authURL)
}

// GetAuthURL returns the OAuth2 authorization URL
// For AniList PIN flow, we use response_type=token (implicit grant)
func (c *Client) GetAuthURL() string {
	// Use implicit grant flow for public clients (no client secret)
	return fmt.Sprintf("%s?client_id=%s&response_type=token", authEndpoint, c.clientID)
}

// ExchangeCode handles the token from AniList's implicit grant flow
// For the implicit flow, the "code" parameter is actually the access token itself
func (c *Client) ExchangeCode(ctx context.Context, accessToken string) error {
	// In implicit grant flow, we receive the token directly (no exchange needed)
	token := &oauth2.Token{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		// AniList tokens don't expire, so we set a far future date
		Expiry: time.Now().Add(365 * 24 * time.Hour),
	}

	c.mu.Lock()
	c.token = token
	c.mu.Unlock()

	// Save token if callback is provided
	if c.saveToken != nil {
		if err := c.saveToken(token); err != nil {
			return fmt.Errorf("failed to save token: %w", err)
		}
	}

	return nil
}

// IsAuthenticated checks if the client has a valid token
func (c *Client) IsAuthenticated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token != nil && c.token.Valid()
}

// GetCurrentUser retrieves the currently authenticated user's information
func (c *Client) GetCurrentUser(ctx context.Context) (int, string, error) {
	if !c.IsAuthenticated() {
		return 0, "", fmt.Errorf("not authenticated")
	}

	query := `
	query {
		Viewer {
			id
			name
		}
	}
	`

	var response struct {
		Data struct {
			Viewer struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"Viewer"`
		} `json:"data"`
	}

	if err := c.query(ctx, query, nil, &response); err != nil {
		return 0, "", fmt.Errorf("failed to get current user: %w", err)
	}

	return response.Data.Viewer.ID, response.Data.Viewer.Name, nil
}

// Logout clears the authentication token
func (c *Client) Logout() error {
	c.mu.Lock()
	c.token = nil
	c.mu.Unlock()

	// Clear stored token if callback is provided
	if c.saveToken != nil {
		return c.saveToken(nil)
	}

	return nil
}

// GetUserLibrary retrieves the user's anime/manga list
func (c *Client) GetUserLibrary(ctx context.Context, mediaType providers.MediaType) ([]tracker.TrackedMedia, error) {
	if !c.IsAuthenticated() {
		return nil, fmt.Errorf("not authenticated")
	}

	// Get current user ID first
	userID, _, err := c.GetCurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user ID: %w", err)
	}

	query := `
	query ($userId: Int, $type: MediaType) {
		MediaListCollection(userId: $userId, type: $type) {
			lists {
				entries {
					id
					status
					score
					progress
					startedAt { year month day }
					completedAt { year month day }
					updatedAt
					media {
						id
						title {
							romaji
							english
							native
						}
						episodes
						chapters
						description
						coverImage {
							large
						}
						type
					}
				}
			}
		}
	}
	`

	variables := map[string]interface{}{
		"userId": userID,
		"type":   anilistMediaType(mediaType),
	}

	var response struct {
		Data struct {
			MediaListCollection struct {
				Lists []struct {
					Entries []anilistEntry `json:"entries"`
				} `json:"lists"`
			} `json:"MediaListCollection"`
		} `json:"data"`
	}

	if err := c.query(ctx, query, variables, &response); err != nil {
		return nil, err
	}

	var result []tracker.TrackedMedia
	for _, list := range response.Data.MediaListCollection.Lists {
		for _, entry := range list.Entries {
			tracked := entryToTrackedMedia(entry)
			result = append(result, tracked)
		}
	}

	return result, nil
}

// SearchMedia searches for anime/manga on AniList
func (c *Client) SearchMedia(ctx context.Context, query string, mediaType providers.MediaType) ([]tracker.TrackedMedia, error) {
	graphqlQuery := `
	query(
		$page: Int = 1
		$perPage: Int = 20
		$id: Int
		$type: MediaType
		$isAdult: Boolean = false
		$search: String
		$format: [MediaFormat]
		$status: MediaStatus
		$countryOfOrigin: CountryCode
		$source: MediaSource
		$season: MediaSeason
		$seasonYear: Int
		$year: String
		$onList: Boolean
		$yearLesser: FuzzyDateInt
		$yearGreater: FuzzyDateInt
		$episodeLesser: Int
		$episodeGreater: Int
		$durationLesser: Int
		$durationGreater: Int
		$chapterLesser: Int
		$chapterGreater: Int
		$volumeLesser: Int
		$volumeGreater: Int
		$licensedBy: [Int]
		$isLicensed: Boolean
		$genres: [String]
		$excludedGenres: [String]
		$tags: [String]
		$excludedTags: [String]
		$minimumTagRank: Int
		$sort: [MediaSort] = [SEARCH_MATCH]
	) {
		Page(page: $page, perPage: $perPage) {
			pageInfo {
				total
				perPage
				currentPage
				lastPage
				hasNextPage
			}
			media(
				id: $id
				type: $type
				season: $season
				format_in: $format
				status: $status
				countryOfOrigin: $countryOfOrigin
				source: $source
				search: $search
				onList: $onList
				seasonYear: $seasonYear
				startDate_like: $year
				startDate_lesser: $yearLesser
				startDate_greater: $yearGreater
				episodes_lesser: $episodeLesser
				episodes_greater: $episodeGreater
				duration_lesser: $durationLesser
				duration_greater: $durationGreater
				chapters_lesser: $chapterLesser
				chapters_greater: $chapterGreater
				volumes_lesser: $volumeLesser
				volumes_greater: $volumeGreater
				licensedById_in: $licensedBy
				isLicensed: $isLicensed
				genre_in: $genres
				genre_not_in: $excludedGenres
				tag_in: $tags
				tag_not_in: $excludedTags
				minimumTagRank: $minimumTagRank
				sort: $sort
				isAdult: $isAdult
			) {
				id
				title {
					userPreferred
					romaji
					english
					native
				}
				coverImage {
					extraLarge
					large
					color
				}
				startDate {
					year
					month
					day
				}
				endDate {
					year
					month
					day
				}
				bannerImage
				season
				seasonYear
				description
				type
				format
				status(version: 2)
				episodes
				duration
				chapters
				volumes
				genres
				isAdult
				averageScore
				popularity
				nextAiringEpisode {
					airingAt
					timeUntilAiring
					episode
				}
				mediaListEntry {
					id
					status
				}
				studios(isMain: true) {
					edges {
						isMain
						node {
							id
							name
						}
					}
				}
			}
		}
	}
	`

	variables := map[string]interface{}{
		"search":  query,
		"type":    anilistMediaType(mediaType),
		"sort":    []string{"SEARCH_MATCH"},
		"isAdult": false,
	}

	var response struct {
		Data struct {
			Page struct {
				PageInfo struct {
					Total       int  `json:"total"`
					PerPage     int  `json:"perPage"`
					CurrentPage int  `json:"currentPage"`
					LastPage    int  `json:"lastPage"`
					HasNextPage bool `json:"hasNextPage"`
				} `json:"pageInfo"`
				Media []anilistMedia `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}

	if err := c.query(ctx, graphqlQuery, variables, &response); err != nil {
		return nil, err
	}

	var result []tracker.TrackedMedia
	for _, media := range response.Data.Page.Media {
		// For search results, determine status from media list entry if available
		status := tracker.StatusPlanToWatch // Default for search results
		if media.MediaListEntry != nil {
			parsedStatus, err := tracker.ParseWatchStatus(mapAniListStatus(media.MediaListEntry.Status))
			if err == nil {
				status = parsedStatus
			}
		}

		totalUnits := media.Episodes
		if media.Type == "MANGA" {
			totalUnits = media.Chapters
		}

		trackedMedia := tracker.TrackedMedia{
			ServiceID:     fmt.Sprintf("%d", media.ID),
			Title:         getBestTitle(media.Title),
			Type:          mediaType,
			TotalEpisodes: totalUnits,
			Synopsis:      media.Description,
			PosterURL:     media.CoverImage.ExtraLarge,
			Status:        status,
			Score:         float64(media.AverageScore) / 10.0,
			StartDate:     media.StartDate.ToTime(),
		}

		// Additional fields that might be useful
		if media.StartDate.Year > 0 {
			startDate := time.Date(
				media.StartDate.Year,
				time.Month(media.StartDate.Month),
				media.StartDate.Day,
				0, 0, 0, 0, time.UTC,
			)
			trackedMedia.StartDate = &startDate
		}

		result = append(result, trackedMedia)
	}

	return result, nil
}

// UpdateProgress updates the watch progress for a media item
func (c *Client) UpdateProgress(ctx context.Context, mediaID string, episode int, progress float64) error {
	if !c.IsAuthenticated() {
		return fmt.Errorf("not authenticated")
	}

	mutation := `
	mutation ($mediaId: Int, $progress: Int, $status: MediaListStatus) {
		SaveMediaListEntry(mediaId: $mediaId, progress: $progress, status: $status) {
			id
			progress
			status
		}
	}
	`

	// Determine status based on progress
	status := "CURRENT"
	if progress >= 0.85 {
		status = "CURRENT" // Will be set to COMPLETED by UpdateStatus if needed
	}

	variables := map[string]interface{}{
		"mediaId":  mustParseInt(mediaID),
		"progress": episode,
		"status":   status,
	}

	var response struct {
		Data struct {
			SaveMediaListEntry struct {
				ID       int    `json:"id"`
				Progress int    `json:"progress"`
				Status   string `json:"status"`
			} `json:"SaveMediaListEntry"`
		} `json:"data"`
	}

	return c.query(ctx, mutation, variables, &response)
}

// GetProgress retrieves the current progress for a media item
func (c *Client) GetProgress(ctx context.Context, mediaID string) (*tracker.Progress, error) {
	if !c.IsAuthenticated() {
		return nil, fmt.Errorf("not authenticated")
	}

	query := `
	query ($mediaId: Int) {
		Media(id: $mediaId) {
			mediaListEntry {
				progress
				status
				updatedAt
			}
		}
	}
	`

	variables := map[string]interface{}{
		"mediaId": mustParseInt(mediaID),
	}

	var response struct {
		Data struct {
			Media struct {
				MediaListEntry *struct {
					Progress  int    `json:"progress"`
					Status    string `json:"status"`
					UpdatedAt int64  `json:"updatedAt"`
				} `json:"mediaListEntry"`
			} `json:"Media"`
		} `json:"data"`
	}

	if err := c.query(ctx, query, variables, &response); err != nil {
		return nil, err
	}

	if response.Data.Media.MediaListEntry == nil {
		return &tracker.Progress{
			MediaID: mediaID,
			Episode: 0,
		}, nil
	}

	entry := response.Data.Media.MediaListEntry
	return &tracker.Progress{
		MediaID:       mediaID,
		Episode:       entry.Progress,
		LastWatchedAt: time.Unix(entry.UpdatedAt, 0),
	}, nil
}

// UpdateStatus updates the watch status for a media item
func (c *Client) UpdateStatus(ctx context.Context, mediaID string, status tracker.WatchStatus) error {
	if !c.IsAuthenticated() {
		return fmt.Errorf("not authenticated")
	}

	mutation := `
	mutation ($mediaId: Int, $status: MediaListStatus) {
		SaveMediaListEntry(mediaId: $mediaId, status: $status) {
			id
			status
		}
	}
	`

	variables := map[string]interface{}{
		"mediaId": mustParseInt(mediaID),
		"status":  anilistStatus(status),
	}

	var response struct {
		Data struct {
			SaveMediaListEntry struct {
				ID     int    `json:"id"`
				Status string `json:"status"`
			} `json:"SaveMediaListEntry"`
		} `json:"data"`
	}

	return c.query(ctx, mutation, variables, &response)
}

// UpdateScore updates the score for a media item
func (c *Client) UpdateScore(ctx context.Context, mediaID string, score float64) error {
	if !c.IsAuthenticated() {
		return fmt.Errorf("not authenticated")
	}

	mutation := `
	mutation ($mediaId: Int, $score: Float) {
		SaveMediaListEntry(mediaId: $mediaId, score: $score) {
			id
			score
		}
	}
	`

	variables := map[string]interface{}{
		"mediaId": mustParseInt(mediaID),
		"score":   score,
	}

	var response struct {
		Data struct {
			SaveMediaListEntry struct {
				ID    int     `json:"id"`
				Score float64 `json:"score"`
			} `json:"SaveMediaListEntry"`
		} `json:"data"`
	}

	return c.query(ctx, mutation, variables, &response)
}

// UpdateDates updates the start and end dates for a media item
func (c *Client) UpdateDates(ctx context.Context, mediaID string, startDate, endDate *time.Time) error {
	if !c.IsAuthenticated() {
		return fmt.Errorf("not authenticated")
	}

	mutation := `
	mutation ($mediaId: Int, $startedAt: FuzzyDateInput, $completedAt: FuzzyDateInput) {
		SaveMediaListEntry(mediaId: $mediaId, startedAt: $startedAt, completedAt: $completedAt) {
			id
		}
	}
	`

	variables := map[string]interface{}{
		"mediaId": mustParseInt(mediaID),
	}

	if startDate != nil {
		variables["startedAt"] = map[string]int{
			"year":  startDate.Year(),
			"month": int(startDate.Month()),
			"day":   startDate.Day(),
		}
	}

	if endDate != nil {
		variables["completedAt"] = map[string]int{
			"year":  endDate.Year(),
			"month": int(endDate.Month()),
			"day":   endDate.Day(),
		}
	}

	var response struct {
		Data struct {
			SaveMediaListEntry struct {
				ID int `json:"id"`
			} `json:"SaveMediaListEntry"`
		} `json:"data"`
	}

	return c.query(ctx, mutation, variables, &response)
}

// DeleteFromList removes a media item from the user's AniList
func (c *Client) DeleteFromList(ctx context.Context, mediaListID int) error {
	if !c.IsAuthenticated() {
		return fmt.Errorf("not authenticated")
	}

	mutation := `
	mutation ($id: Int) {
		DeleteMediaListEntry(id: $id) {
			deleted
		}
	}
	`

	variables := map[string]interface{}{
		"id": mediaListID,
	}

	var response struct {
		Data struct {
			DeleteMediaListEntry struct {
				Deleted bool `json:"deleted"`
			} `json:"DeleteMediaListEntry"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := c.query(ctx, mutation, variables, &response); err != nil {
		return fmt.Errorf("failed to delete from AniList: %w", err)
	}

	if len(response.Errors) > 0 {
		return fmt.Errorf("AniList API error: %s", response.Errors[0].Message)
	}

	if !response.Data.DeleteMediaListEntry.Deleted {
		return fmt.Errorf("failed to delete from AniList: operation returned false")
	}

	return nil
}

// SyncHistory syncs local watch history to AniList
func (c *Client) SyncHistory(ctx context.Context) error {
	if !c.IsAuthenticated() {
		return fmt.Errorf("not authenticated")
	}

	// TODO: implement
	// This is a placeholder - actual implementation would need to:
	// 1. Get pending sync items from database
	// 2. Update each item on AniList
	// 3. Mark as synced in database
	return fmt.Errorf("not implemented")
}

// query executes a GraphQL query/mutation
func (c *Client) query(ctx context.Context, query string, variables map[string]interface{}, result interface{}) error {
	// Rate limiting
	c.mu.Lock()
	if time.Since(c.lastRequest) < rateLimitDelay {
		time.Sleep(rateLimitDelay - time.Since(c.lastRequest))
	}
	c.lastRequest = time.Now()
	c.mu.Unlock()

	requestBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiEndpoint, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Add authorization header if authenticated
	c.mu.Lock()
	if c.token != nil {
		req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)
	}
	c.mu.Unlock()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResponse struct {
			Errors []struct {
				Message string `json:"message"`
				Status  int    `json:"status"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResponse); err == nil && len(errorResponse.Errors) > 0 {
			return fmt.Errorf("AniList API error: %s", errorResponse.Errors[0].Message)
		}
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return nil
}
