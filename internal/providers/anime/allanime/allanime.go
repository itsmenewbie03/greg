package allanime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/pkg/types"
)

type AllAnime struct {
	BaseURL     string
	APIURL      string
	Client      *http.Client
	searchCache sync.Map
	infoCache   sync.Map
}

func New() *AllAnime {
	return &AllAnime{
		BaseURL: "https://allanime.to",
		APIURL:  "https://api.allanime.day",
		Client:  &http.Client{},
	}
}

func (a *AllAnime) Name() string {
	return "allanime"
}

func (a *AllAnime) Type() providers.MediaType {
	return providers.MediaTypeAnime
}

// GraphQL response structures
type searchResponse struct {
	Data struct {
		Shows struct {
			Edges []struct {
				ID                string      `json:"_id"`
				Name              string      `json:"name"`
				EnglishName       string      `json:"englishName"`
				AvailableEpisodes interface{} `json:"availableEpisodes"`
				Thumbnail         string      `json:"thumbnail"`
			} `json:"edges"`
		} `json:"shows"`
	} `json:"data"`
}

type infoResponse struct {
	Data struct {
		Show struct {
			ID                string      `json:"_id"`
			Name              string      `json:"name"`
			EnglishName       string      `json:"englishName"`
			Description       string      `json:"description"`
			Thumbnail         string      `json:"thumbnail"`
			AvailableEpisodes interface{} `json:"availableEpisodes"`
			Status            string      `json:"status"`
			Genres            []string    `json:"genres"`
		} `json:"show"`
	} `json:"data"`
}

type episodeResponse struct {
	Data struct {
		Episode struct {
			SourceUrls []struct {
				SourceURL string `json:"sourceUrl"`
			} `json:"sourceUrls"`
		} `json:"episode"`
	} `json:"data"`
}

type linkData struct {
	Links []struct {
		Link string `json:"link"`
	} `json:"links"`
}

// Search searches for anime by query
func (a *AllAnime) Search(ctx context.Context, query string) ([]providers.Media, error) {
	if cached, ok := a.searchCache.Load(query); ok {
		return cached.([]providers.Media), nil
	}

	searchGQL := `query($search: SearchInput, $limit: Int, $page: Int, $translationType: VaildTranslationTypeEnumType, $countryOrigin: VaildCountryOriginEnumType) {
		shows(search: $search, limit: $limit, page: $page, translationType: $translationType, countryOrigin: $countryOrigin) {
			edges {
				_id
				name
				englishName
				availableEpisodes
				thumbnail
				__typename
			}
		}
	}`

	variables := map[string]interface{}{
		"search": map[string]interface{}{
			"allowAdult":   false,
			"allowUnknown": false,
			"query":        query,
		},
		"limit":           40,
		"page":            1,
		"translationType": "sub",
		"countryOrigin":   "ALL",
	}

	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal variables: %w", err)
	}

	reqURL := fmt.Sprintf("%s/api?variables=%s&query=%s",
		a.APIURL,
		url.QueryEscape(string(variablesJSON)),
		url.QueryEscape(searchGQL))

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0")
	req.Header.Set("Referer", a.BaseURL)

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch search results: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var searchResp searchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var results []providers.Media

	for _, anime := range searchResp.Data.Shows.Edges {
		title := anime.Name
		if anime.EnglishName != "" {
			title = anime.EnglishName
		}

		results = append(results, providers.Media{
			ID:        anime.ID,
			Title:     strings.TrimSpace(title),
			Type:      providers.MediaTypeAnime,
			PosterURL: anime.Thumbnail,
		})
	}

	a.searchCache.Store(query, results)
	return results, nil
}

// GetInfo fetches detailed info for an anime
func (a *AllAnime) GetInfo(id string) (interface{}, error) {
	if cached, ok := a.infoCache.Load(id); ok {
		return cached.(*types.AnimeInfo), nil
	}

	infoGQL := `query($showId: String!) {
		show(_id: $showId) {
			_id
			name
			englishName
			description
			thumbnail
			availableEpisodes
			status
			genres
		}
	}`

	variables := map[string]string{
		"showId": id,
	}

	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal variables: %w", err)
	}

	reqURL := fmt.Sprintf("%s/api?variables=%s&query=%s",
		a.APIURL,
		url.QueryEscape(string(variablesJSON)),
		url.QueryEscape(infoGQL))

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0")
	req.Header.Set("Referer", a.BaseURL)

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch anime info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var infoResp infoResponse
	if err := json.Unmarshal(body, &infoResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	show := infoResp.Data.Show
	title := show.Name
	if show.EnglishName != "" {
		title = show.EnglishName
	}

	// Parse available episodes
	totalEpisodes := 0
	if episodes, ok := show.AvailableEpisodes.(map[string]interface{}); ok {
		if subEpisodes, ok := episodes["sub"].(float64); ok {
			totalEpisodes = int(subEpisodes)
		}
	}

	// Parse status
	status := types.MediaStatusUnknown
	switch strings.ToUpper(show.Status) {
	case "RELEASING":
		status = types.MediaStatusOngoing
	case "FINISHED":
		status = types.MediaStatusCompleted
	}

	info := &types.AnimeInfo{
		ID:            id,
		Title:         strings.TrimSpace(title),
		URL:           fmt.Sprintf("%s/anime/%s", a.BaseURL, id),
		Image:         show.Thumbnail,
		Description:   strings.TrimSpace(show.Description),
		Genres:        show.Genres,
		Status:        status,
		TotalEpisodes: totalEpisodes,
		Episodes:      []types.Episode{},
	}

	// Generate episode list
	for i := 1; i <= totalEpisodes; i++ {
		info.Episodes = append(info.Episodes, types.Episode{
			ID:     fmt.Sprintf("%s-%d", id, i),
			Number: i,
			Title:  fmt.Sprintf("Episode %d", i),
		})
	}

	a.infoCache.Store(id, info)
	return info, nil
}

func (a *AllAnime) GetMediaDetails(ctx context.Context, id string) (*providers.MediaDetails, error) {
	info, err := a.GetInfo(id)
	if err != nil {
		return nil, err
	}

	animeInfo, ok := info.(*types.AnimeInfo)
	if !ok {
		return nil, fmt.Errorf("invalid info type")
	}

	return &providers.MediaDetails{
		Media: providers.Media{
			ID:            id,
			Title:         animeInfo.Title,
			Type:          providers.MediaTypeAnime,
			PosterURL:     animeInfo.Image,
			Synopsis:      animeInfo.Description,
			Genres:        animeInfo.Genres,
			Status:        string(animeInfo.Status),
			TotalEpisodes: animeInfo.TotalEpisodes,
		},
	}, nil
}

func (a *AllAnime) GetSeasons(ctx context.Context, mediaID string) ([]providers.Season, error) {
	return []providers.Season{{
		ID:     mediaID,
		Number: 1,
		Title:  "Season 1",
	}}, nil
}

func (a *AllAnime) GetEpisodes(ctx context.Context, seasonID string) ([]providers.Episode, error) {
	mediaID := seasonID
	info, err := a.GetInfo(mediaID)
	if err != nil {
		return nil, err
	}

	animeInfo, ok := info.(*types.AnimeInfo)
	if !ok {
		return nil, fmt.Errorf("invalid info type")
	}

	var episodes []providers.Episode
	for _, ep := range animeInfo.Episodes {
		episodes = append(episodes, providers.Episode{
			ID:     ep.ID,
			Number: ep.Number,
			Title:  ep.Title,
			Season: 1,
		})
	}

	return episodes, nil
}

func (a *AllAnime) GetStreamURL(ctx context.Context, episodeID string, quality providers.Quality) (*providers.StreamURL, error) {
	res, err := a.GetSources(episodeID)
	if err != nil {
		return nil, err
	}

	v, ok := res.(*types.VideoSources)
	if !ok {
		return nil, fmt.Errorf("invalid source type")
	}

	if len(v.Sources) == 0 {
		return nil, fmt.Errorf("no sources found")
	}

	selectedSource := v.Sources[0]

	streamType := providers.StreamTypeHLS
	if !selectedSource.IsM3U8 {
		streamType = providers.StreamTypeMP4
	}

	return &providers.StreamURL{
		URL:     selectedSource.URL,
		Quality: providers.Quality(selectedSource.Quality),
		Type:    streamType,
		Referer: selectedSource.Referer,
		Headers: map[string]string{
			"Referer": selectedSource.Referer,
		},
	}, nil
}

func (a *AllAnime) GetAvailableQualities(ctx context.Context, episodeID string) ([]providers.Quality, error) {
	res, err := a.GetSources(episodeID)
	if err != nil {
		return nil, err
	}

	var qualities []providers.Quality
	if v, ok := res.(*types.VideoSources); ok {
		for _, src := range v.Sources {
			qualities = append(qualities, providers.Quality(src.Quality))
		}
	}
	return qualities, nil
}

func (a *AllAnime) GetTrending(ctx context.Context) ([]providers.Media, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *AllAnime) GetRecent(ctx context.Context) ([]providers.Media, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *AllAnime) HealthCheck(ctx context.Context) error {
	return nil
}

// GetServers fetches available servers for an episode
func (a *AllAnime) GetServers(episodeID string) ([]types.EpisodeServer, error) {
	// AllAnime doesn't have a traditional server selection
	// The servers are embedded in the source URLs
	// Return a generic server entry
	return []types.EpisodeServer{
		{
			Name: "AllAnime",
			URL:  episodeID,
		},
	}, nil
}

// isValidStreamURL validates if a URL is a valid streaming URL
func (a *AllAnime) isValidStreamURL(urlStr string) bool {
	// Parse the URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Check if it has a valid scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}

	// Check if it has a valid host
	if parsedURL.Host == "" {
		return false
	}

	// Filter out known invalid patterns
	// Reject URLs that end with just a number after the last slash (like /5, /sub/5)
	path := strings.TrimSuffix(parsedURL.Path, "/")
	if path == "" {
		return false
	}

	lastSegment := path[strings.LastIndex(path, "/")+1:]

	// Check if the last segment is just a number (common pattern in invalid URLs)
	if _, err := strconv.Atoi(lastSegment); err == nil {
		return false
	}

	// Check for valid video file extensions or streaming protocols
	validExtensions := []string{".m3u8", ".mp4", ".mkv", ".webm", ".ts", ".mpd", ".m3u"}
	hasValidExtension := false
	lowerPath := strings.ToLower(path)

	for _, ext := range validExtensions {
		if strings.HasSuffix(lowerPath, ext) {
			hasValidExtension = true
			break
		}
	}

	// If it doesn't have a valid extension, check if it's a streaming path
	// (some m3u8 playlists don't have extensions in the path)
	if !hasValidExtension {
		// Allow paths that contain streaming-related keywords
		streamingKeywords := []string{"playlist", "master", "stream", "hls", "dash", "manifest"}
		for _, keyword := range streamingKeywords {
			if strings.Contains(lowerPath, keyword) {
				hasValidExtension = true
				break
			}
		}
	}

	// Additional check: reject URLs with suspicious patterns
	// like ending with /sub/number or /dub/number
	suspiciousPatterns := []string{
		`/sub/\d+$`,
		`/dub/\d+$`,
		`/videos/[^/]+/sub/\d+$`,
		`/videos/[^/]+/dub/\d+$`,
	}

	for _, pattern := range suspiciousPatterns {
		if matched, _ := regexp.MatchString(pattern, path); matched {
			return false
		}
	}

	return hasValidExtension
}

// GetSources fetches video sources for an episode
func (a *AllAnime) GetSources(episodeID string) (interface{}, error) {
	// Parse episodeID format: "animeID-episodeNumber"
	parts := strings.Split(episodeID, "-")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid episode ID format: %s", episodeID)
	}

	episodeNum := parts[len(parts)-1]
	animeID := strings.Join(parts[:len(parts)-1], "-")

	// Fetch source URLs from API
	links, err := a.getEpisodeLinks(animeID, episodeNum)
	if err != nil {
		return nil, fmt.Errorf("failed to get episode links: %w", err)
	}

	if len(links) == 0 {
		return &types.VideoSources{
			Sources:   []types.Source{},
			Subtitles: []types.Subtitle{},
		}, nil
	}

	sources := &types.VideoSources{
		Sources:   []types.Source{},
		Subtitles: []types.Subtitle{},
	}

	// Filter and add valid links as sources
	for _, link := range links {
		// Validate the URL before adding
		if !a.isValidStreamURL(link) {
			continue
		}

		// Determine if it's M3U8
		isM3U8 := strings.Contains(link, ".m3u8") || strings.Contains(link, "/hls/")

		sources.Sources = append(sources.Sources, types.Source{
			URL:     link,
			Quality: "auto",
			IsM3U8:  isM3U8,
			Referer: a.BaseURL,
		})
	}

	// If no valid sources were found, return an error
	if len(sources.Sources) == 0 {
		return nil, fmt.Errorf("no valid streaming sources found for episode %s", episodeID)
	}

	return sources, nil
}

// getEpisodeLinks fetches episode video links using GraphQL
func (a *AllAnime) getEpisodeLinks(animeID, episodeNum string) ([]string, error) {
	query := `query($showId:String!,$translationType:VaildTranslationTypeEnumType!,$episodeString:String!){episode(showId:$showId,translationType:$translationType,episodeString:$episodeString){episodeString sourceUrls}}`

	variables := map[string]string{
		"showId":          animeID,
		"translationType": "sub",
		"episodeString":   episodeNum,
	}

	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal variables: %w", err)
	}

	reqURL := fmt.Sprintf("%s/api?variables=%s&query=%s",
		a.APIURL,
		url.QueryEscape(string(variablesJSON)),
		url.QueryEscape(query))

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0")
	req.Header.Set("Referer", a.BaseURL)

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch episode sources: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var epResp episodeResponse
	if err := json.Unmarshal(body, &epResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract and decode provider IDs
	var allLinks []string
	resultChan := make(chan []string, len(epResp.Data.Episode.SourceUrls))

	validCount := 0
	for _, sourceURL := range epResp.Data.Episode.SourceUrls {
		if len(sourceURL.SourceURL) > 2 && unicode.IsDigit(rune(sourceURL.SourceURL[2])) {
			validCount++
			go func(encoded string) {
				decodedURL := a.decodeProviderID(encoded[2:])
				links := a.extractLinks(decodedURL)
				resultChan <- links
			}(sourceURL.SourceURL)
		}
	}

	// Collect results with timeout
	timeout := time.After(10 * time.Second)
	collected := 0

	for collected < validCount {
		select {
		case links := <-resultChan:
			allLinks = append(allLinks, links...)
			collected++
		case <-timeout:
			// Return what we have so far
			return allLinks, nil
		}
	}

	return allLinks, nil
}

// decodeProviderID decodes the hex-encoded provider ID
func (a *AllAnime) decodeProviderID(encoded string) string {
	re := regexp.MustCompile("..")
	pairs := re.FindAllString(encoded, -1)

	replacements := map[string]string{
		// Uppercase letters
		"79": "A", "7a": "B", "7b": "C", "7c": "D", "7d": "E", "7e": "F", "7f": "G",
		"70": "H", "71": "I", "72": "J", "73": "K", "74": "L", "75": "M", "76": "N", "77": "O",
		"68": "P", "69": "Q", "6a": "R", "6b": "S", "6c": "T", "6d": "U", "6e": "V", "6f": "W",
		"60": "X", "61": "Y", "62": "Z",
		// Lowercase letters
		"59": "a", "5a": "b", "5b": "c", "5c": "d", "5d": "e", "5e": "f", "5f": "g",
		"50": "h", "51": "i", "52": "j", "53": "k", "54": "l", "55": "m", "56": "n", "57": "o",
		"48": "p", "49": "q", "4a": "r", "4b": "s", "4c": "t", "4d": "u", "4e": "v", "4f": "w",
		"40": "x", "41": "y", "42": "z",
		// Numbers
		"08": "0", "09": "1", "0a": "2", "0b": "3", "0c": "4", "0d": "5", "0e": "6", "0f": "7",
		"00": "8", "01": "9",
		// Special characters
		"15": "-", "16": ".", "67": "_", "46": "~", "02": ":", "17": "/", "07": "?", "1b": "#",
		"63": "[", "65": "]", "78": "@", "19": "!", "1c": "$", "1e": "&", "10": "(", "11": ")",
		"12": "*", "13": "+", "14": ",", "03": ";", "05": "=", "1d": "%",
	}

	for i, pair := range pairs {
		if val, exists := replacements[pair]; exists {
			pairs[i] = val
		}
	}

	result := strings.Join(pairs, "")
	result = strings.ReplaceAll(result, "/clock", "/clock.json")

	return result
}

// extractLinks extracts video links from the decoded provider ID
func (a *AllAnime) extractLinks(providerID string) []string {
	// Check if it's already a full URL (external link)
	if strings.HasPrefix(providerID, "http://") || strings.HasPrefix(providerID, "https://") {
		// Clean up double slashes
		cleanedURL := providerID
		if strings.Contains(cleanedURL, "://") {
			parts := strings.SplitN(cleanedURL, "://", 2)
			if len(parts) == 2 {
				protocol := parts[0]
				rest := strings.ReplaceAll(parts[1], "//", "/")
				cleanedURL = protocol + "://" + rest
			}
		}

		// Validate before returning
		if a.isValidStreamURL(cleanedURL) {
			return []string{cleanedURL}
		}
		return []string{}
	}

	// It's a relative path for allanime API
	reqURL := "https://allanime.day" + providerID

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return []string{}
	}

	req.Header.Set("Referer", a.BaseURL)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0")

	resp, err := a.Client.Do(req)
	if err != nil {
		return []string{}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []string{}
	}

	var data linkData
	if err := json.Unmarshal(body, &data); err != nil {
		return []string{}
	}

	var links []string
	for _, link := range data.Links {
		// Validate each link before adding
		if link.Link != "" && a.isValidStreamURL(link.Link) {
			links = append(links, link.Link)
		}
	}

	return links
}
