package hianime

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

	"github.com/PuerkitoBio/goquery"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/pkg/extractors"
	"github.com/justchokingaround/greg/pkg/types"
)

type HiAnime struct {
	BaseURL     string
	Client      *http.Client
	searchCache sync.Map
	infoCache   sync.Map
}

func New() *HiAnime {
	return &HiAnime{
		BaseURL: "https://hianime.to",
		Client:  &http.Client{},
	}
}

func (h *HiAnime) Name() string {
	return "hianime"
}

func (h *HiAnime) Type() providers.MediaType {
	return providers.MediaTypeAnime
}

// Search searches for anime by query
func (h *HiAnime) Search(ctx context.Context, query string) ([]providers.Media, error) {
	if cached, ok := h.searchCache.Load(query); ok {
		return cached.([]providers.Media), nil
	}

	searchURL := fmt.Sprintf("%s/search?keyword=%s", h.BaseURL, url.QueryEscape(query))

	resp, err := h.Client.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch search results: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var results []providers.Media

	doc.Find("div.flw-item").Each(func(i int, s *goquery.Selection) {
		title := s.Find("h3.film-name a").Text()
		href, _ := s.Find("h3.film-name a").Attr("href")
		image, _ := s.Find("img").Attr("data-src")

		if href != "" {
			// Extract ID from href (e.g., /watch/naruto-100 -> naruto-100)
			parts := strings.Split(href, "/")
			id := ""
			if len(parts) > 1 {
				id = parts[len(parts)-1]
			}

			// Clean the ID by removing the last segment if it contains a number
			// e.g., naruto-100 -> just use as is
			idParts := strings.Split(id, "?")
			if len(idParts) > 0 {
				id = idParts[0]
			}

			results = append(results, providers.Media{
				ID:        id,
				Title:     strings.TrimSpace(title),
				Type:      providers.MediaTypeAnime,
				PosterURL: image,
			})
		}
	})

	h.searchCache.Store(query, results)
	return results, nil
}

// GetInfo fetches detailed info for an anime
func (h *HiAnime) GetInfo(id string) (interface{}, error) {
	if cached, ok := h.infoCache.Load(id); ok {
		return cached.(*types.AnimeInfo), nil
	}

	infoURL := fmt.Sprintf("%s/%s", h.BaseURL, id)

	req, err := http.NewRequest("GET", infoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122 Safari/537.36")
	req.Header.Set("Referer", h.BaseURL)

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch anime info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	info := &types.AnimeInfo{
		ID:       id,
		URL:      infoURL,
		Episodes: []types.Episode{},
		Genres:   []string{},
	}

	// Extract title
	info.Title = strings.TrimSpace(doc.Find("h2.film-name").Text())

	// Extract image
	if img, exists := doc.Find("img.film-poster-img").Attr("src"); exists {
		info.Image = img
	}

	// Extract description
	info.Description = strings.TrimSpace(doc.Find("div.film-description").Text())

	// Extract genres
	doc.Find("div.item-list a").Each(func(i int, s *goquery.Selection) {
		genre := strings.TrimSpace(s.Text())
		if genre != "" {
			info.Genres = append(info.Genres, genre)
		}
	})

	// Extract the anime data-id from the page (used for fetching episodes)
	// The ID is typically in format: anime-id-123, we need just the number part
	dataID := ""
	if idAttr, exists := doc.Find("#syncData").Attr("data-sync"); exists {
		// Parse the JSON to get the ID
		var syncData map[string]interface{}
		if err := json.Unmarshal([]byte(idAttr), &syncData); err == nil {
			if anilistID, ok := syncData["anilist_id"].(float64); ok {
				dataID = fmt.Sprintf("%.0f", anilistID)
			}
		}
	}

	// If we couldn't get data-id from syncData, extract it from the anime ID
	if dataID == "" {
		// Extract numeric ID from anime ID (e.g., "naruto-100" -> "100")
		re := regexp.MustCompile(`-(\d+)$`)
		if matches := re.FindStringSubmatch(id); len(matches) > 1 {
			dataID = matches[1]
		}
	}

	// Fetch episodes from AJAX endpoint
	if dataID != "" {
		episodes, err := h.fetchEpisodeList(dataID)
		if err == nil && len(episodes) > 0 {
			info.Episodes = episodes
			info.TotalEpisodes = len(episodes)
		}
	}

	h.infoCache.Store(id, info)
	return info, nil
}

// fetchEpisodeList fetches the episode list from the AJAX endpoint
func (h *HiAnime) fetchEpisodeList(animeDataID string) ([]types.Episode, error) {
	episodeURL := fmt.Sprintf("%s/ajax/v2/episode/list/%s", h.BaseURL, animeDataID)

	req, err := http.NewRequest("GET", episodeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create episode list request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", h.BaseURL)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch episode list: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read episode list response: %w", err)
	}

	// Parse JSON response
	var jsonResponse struct {
		Status bool   `json:"status"`
		HTML   string `json:"html"`
	}

	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return nil, fmt.Errorf("failed to parse episode list JSON: %w", err)
	}

	if !jsonResponse.Status || jsonResponse.HTML == "" {
		return nil, fmt.Errorf("invalid episode list response")
	}

	// Parse the HTML content
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(jsonResponse.HTML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse episode HTML: %w", err)
	}

	episodes := []types.Episode{}

	// Find all episode items
	doc.Find("a.ep-item").Each(func(i int, s *goquery.Selection) {
		epID, exists := s.Attr("data-id")
		if !exists {
			return
		}

		epNum, _ := s.Attr("data-number")
		epNum = strings.TrimSpace(epNum)
		epTitle, _ := s.Attr("title")
		epTitle = strings.TrimSpace(epTitle)

		// Parse episode number
		num := i + 1
		if parsedNum, err := strconv.Atoi(epNum); err == nil {
			num = parsedNum
		}

		episodes = append(episodes, types.Episode{
			ID:     epID,
			Number: num,
			Title:  epTitle,
		})
	})

	return episodes, nil
}

func (h *HiAnime) GetMediaDetails(ctx context.Context, id string) (*providers.MediaDetails, error) {
	info, err := h.GetInfo(id)
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

func (h *HiAnime) GetSeasons(ctx context.Context, mediaID string) ([]providers.Season, error) {
	// Anime typically has one season
	return []providers.Season{{
		ID:     mediaID,
		Number: 1,
		Title:  "Season 1",
	}}, nil
}

func (h *HiAnime) GetEpisodes(ctx context.Context, seasonID string) ([]providers.Episode, error) {
	mediaID := seasonID
	info, err := h.GetInfo(mediaID)
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

func (h *HiAnime) GetStreamURL(ctx context.Context, episodeID string, quality providers.Quality) (*providers.StreamURL, error) {
	res, err := h.GetSources(episodeID)
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

	// Find best quality match
	var selectedSource types.Source
	found := false

	targetQuality := string(quality)
	if quality == providers.QualityAuto {
		targetQuality = "auto"
	}

	for _, src := range v.Sources {
		if strings.EqualFold(src.Quality, targetQuality) {
			selectedSource = src
			found = true
			break
		}
	}

	if !found {
		selectedSource = v.Sources[0]
	}

	streamType := providers.StreamTypeHLS
	if !selectedSource.IsM3U8 {
		streamType = providers.StreamTypeMP4
	}

	streamURL := &providers.StreamURL{
		URL:     selectedSource.URL,
		Quality: providers.Quality(selectedSource.Quality),
		Type:    streamType,
		Referer: selectedSource.Referer,
		Headers: map[string]string{
			"Referer": selectedSource.Referer,
		},
	}

	for _, sub := range v.Subtitles {
		streamURL.Subtitles = append(streamURL.Subtitles, providers.Subtitle{
			Language: sub.Lang,
			URL:      sub.URL,
		})
	}

	return streamURL, nil
}

func (h *HiAnime) GetAvailableQualities(ctx context.Context, episodeID string) ([]providers.Quality, error) {
	res, err := h.GetSources(episodeID)
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

// GetSources fetches video sources for an episode
func (h *HiAnime) GetSources(episodeID string) (interface{}, error) {
	// Get available servers
	servers, err := h.GetServers(episodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch servers: %w", err)
	}

	if len(servers) == 0 {
		return &types.VideoSources{
			Sources:   []types.Source{},
			Subtitles: []types.Subtitle{},
		}, nil
	}

	// Try each server until we get valid sources
	var lastErr error
	for _, server := range servers {
		sources, err := h.extractSourcesFromServer(server)
		if err != nil {
			lastErr = err
			continue
		}

		if len(sources.Sources) > 0 {
			return sources, nil
		}
	}

	// If all servers failed, return the last error
	if lastErr != nil {
		return nil, fmt.Errorf("failed to extract sources from all servers: %w", lastErr)
	}

	return &types.VideoSources{
		Sources:   []types.Source{},
		Subtitles: []types.Subtitle{},
	}, nil
}

// extractSourcesFromServer extracts video sources from a specific server
func (h *HiAnime) extractSourcesFromServer(server types.EpisodeServer) (*types.VideoSources, error) {
	// The server.URL contains the server ID
	serverID := server.URL

	// Get the embed URL from the sources endpoint
	sourcesURL := fmt.Sprintf("%s/ajax/v2/episode/sources?id=%s", h.BaseURL, serverID)

	req, err := http.NewRequest("GET", sourcesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create sources request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", h.BaseURL)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch embed URL: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read sources response: %w", err)
	}

	// Parse JSON response to get embed URL
	// Response format: {"type":"iframe","link":"https://megacloud.blog/...","server":4,...}
	var jsonResponse struct {
		Type   string `json:"type"`
		Link   string `json:"link"`
		Server int    `json:"server"`
	}

	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return nil, fmt.Errorf("failed to parse sources JSON: %w", err)
	}

	if jsonResponse.Link == "" {
		return nil, fmt.Errorf("no embed link found in response")
	}

	embedURL := jsonResponse.Link

	// Use the extractor to get actual video sources
	extractor := extractors.GetExtractor(server.Name)
	extracted, err := extractor.Extract(embedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to extract from embed URL %s: %w", embedURL, err)
	}

	return extracted, nil
}

// GetServers fetches available servers for an episode
func (h *HiAnime) GetServers(episodeID string) ([]types.EpisodeServer, error) {
	serverURL := fmt.Sprintf("%s/ajax/v2/episode/servers?episodeId=%s", h.BaseURL, episodeID)

	req, err := http.NewRequest("GET", serverURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create servers request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", h.BaseURL)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch servers: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read servers response: %w", err)
	}

	// Parse JSON response
	var jsonResponse struct {
		Status bool   `json:"status"`
		HTML   string `json:"html"`
	}

	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return nil, fmt.Errorf("failed to parse servers JSON: %w", err)
	}

	if !jsonResponse.Status || jsonResponse.HTML == "" {
		return nil, fmt.Errorf("invalid servers response")
	}

	// Parse the HTML content
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(jsonResponse.HTML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse servers HTML: %w", err)
	}

	servers := []types.EpisodeServer{}

	// Find all server items
	doc.Find(".server-item").Each(func(i int, s *goquery.Selection) {
		serverID, exists := s.Attr("data-id")
		if !exists {
			return
		}

		serverName := strings.TrimSpace(s.Text())
		serverType, _ := s.Attr("data-type") // "sub" or "dub"
		serverType = strings.TrimSpace(serverType)

		// Include server type in name if available
		displayName := serverName
		if serverType != "" {
			displayName = fmt.Sprintf("%s (%s)", serverName, serverType)
		}

		servers = append(servers, types.EpisodeServer{
			Name: displayName,
			URL:  serverID, // Store server ID in URL field for later use
		})
	})

	return servers, nil
}

func (h *HiAnime) GetTrending(ctx context.Context) ([]providers.Media, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *HiAnime) GetRecent(ctx context.Context) ([]providers.Media, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *HiAnime) HealthCheck(ctx context.Context) error {
	return nil
}
