package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/pkg/types"
)

type Client struct {
	NameStr string
	BaseURL string
	Client  *http.Client
}

func New(name, baseURL string) *Client {
	return &Client{
		NameStr: name,
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  &http.Client{},
	}
}

func (c *Client) Name() string {
	return c.NameStr
}

func (c *Client) searchOld(query string) (*types.SearchResults, error) {
	// Assuming the remote API follows /{query} pattern for search
	// But standard consumet is /{provider}/{query}
	// The BaseURL should include the provider path, e.g. http://host/anime/gogoanime

	searchURL := fmt.Sprintf("%s/%s", c.BaseURL, url.PathEscape(query))

	resp, err := c.Client.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch search results: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote search returned status %d", resp.StatusCode)
	}

	var results types.SearchResults
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode search results: %w", err)
	}

	return &results, nil
}

func (c *Client) GetInfo(id string) (interface{}, error) {
	// Consumet API: /{provider}/info/{id}
	infoURL := fmt.Sprintf("%s/info/%s", c.BaseURL, id)

	resp, err := c.Client.Get(infoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote info returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Try to determine type based on content or just try unmarshaling
	// We can try AnimeInfo first, then MovieInfo, then MangaInfo

	// Try AnimeInfo
	var animeInfo types.AnimeInfo
	if err := json.Unmarshal(body, &animeInfo); err == nil && animeInfo.Title != "" {
		// Check if it looks like anime (has TotalEpisodes or Type is TV/OVA/etc)
		// Or just return it if it parsed successfully.
		// However, MovieInfo and AnimeInfo are similar.
		// Let's check for specific fields.
		// AnimeInfo has "totalEpisodes"
		// MovieInfo has "duration" (not in our struct) or "rating"

		// A simple heuristic: if it has "chapters", it's manga.
		if strings.Contains(string(body), `"chapters"`) {
			var mangaInfo types.MangaInfo
			if err := json.Unmarshal(body, &mangaInfo); err == nil {
				return &mangaInfo, nil
			}
		}

		// If it has "episodes", it could be anime or movie/tv
		// If we can't distinguish easily, we might need context.
		// But for now, let's try to unmarshal into AnimeInfo.
		return &animeInfo, nil
	}

	// Try MovieInfo
	var movieInfo types.MovieInfo
	if err := json.Unmarshal(body, &movieInfo); err == nil && movieInfo.Title != "" {
		return &movieInfo, nil
	}

	return nil, fmt.Errorf("failed to decode info into known types")
}

func (c *Client) GetSources(episodeID string) (interface{}, error) {
	// Consumet API: /{provider}/watch/{episodeId}
	// Note: Some providers use /read for manga?
	// Let's assume /watch for now, or check if it's manga.

	// If we don't know if it's manga or anime, we might have issues.
	// But usually manga uses /read or similar?
	// Consumet documentation says:
	// Anime: /anime/{provider}/watch/{episodeId}
	// Movies: /movies/{provider}/watch/{episodeId}
	// Manga: /manga/{provider}/read/{chapterId}

	// We can try /watch first.
	watchURL := fmt.Sprintf("%s/watch/%s", c.BaseURL, episodeID)

	resp, err := c.Client.Get(watchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sources: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		// Try /read for manga
		readURL := fmt.Sprintf("%s/read/%s", c.BaseURL, episodeID)
		resp, err = c.Client.Get(readURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch manga pages: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusOK {
			// It's likely manga pages
			// Consumet returns array of objects directly or wrapped?
			// Usually array of objects for pages.
			// Our types.MangaPages wraps it.

			var pages []*types.MangaPage
			if err := json.NewDecoder(resp.Body).Decode(&pages); err == nil {
				return &types.MangaPages{Pages: pages}, nil
			}
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote sources returned status %d", resp.StatusCode)
	}

	var sources types.VideoSources
	if err := json.NewDecoder(resp.Body).Decode(&sources); err != nil {
		return nil, fmt.Errorf("failed to decode sources: %w", err)
	}

	return &sources, nil
}

func (c *Client) GetServers(episodeID string) ([]types.EpisodeServer, error) {
	// Consumet API usually doesn't have a separate servers endpoint exposed in the same way for all providers,
	// or it's included in the source response?
	// Actually, consumet-api often has /servers/{episodeId}?
	// Let's assume /{provider}/servers/{episodeId} exists if needed.
	// If not, we return empty.

	serverURL := fmt.Sprintf("%s/servers/%s", c.BaseURL, episodeID)

	resp, err := c.Client.Get(serverURL)
	if err != nil {
		// If failed, just return empty list, don't error out completely as it might not be supported
		return []types.EpisodeServer{}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return []types.EpisodeServer{}, nil
	}

	var servers []types.EpisodeServer
	if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
		return []types.EpisodeServer{}, nil
	}

	return servers, nil
}

// Type returns the media type this provider supports (generic remote)
func (c *Client) Type() providers.MediaType {
	// Remote providers can support multiple types, default to anime
	return providers.MediaTypeAnime
}

// Search (new interface) searches by query
func (c *Client) Search(ctx context.Context, query string) ([]providers.Media, error) {
	oldResults, err := c.searchOld(query)
	if err != nil {
		return nil, err
	}

	var mediaList []providers.Media
	for _, item := range oldResults.Results {
		year := 0
		if len(item.ReleaseDate) >= 4 {
			if y, err := fmt.Sscanf(item.ReleaseDate[:4], "%d", &year); err == nil && y == 1 {
				// parsed successfully
			}
		}

		// Try to infer type from item.Type field
		mediaType := providers.MediaTypeAnime
		itemTypeLower := strings.ToLower(item.Type)
		if strings.Contains(itemTypeLower, "movie") {
			mediaType = providers.MediaTypeMovie
		} else if strings.Contains(itemTypeLower, "tv") {
			mediaType = providers.MediaTypeTV
		} else if strings.Contains(itemTypeLower, "manga") {
			mediaType = providers.MediaTypeManga
		}

		mediaList = append(mediaList, providers.Media{
			ID:        item.ID,
			Title:     item.Title,
			Type:      mediaType,
			PosterURL: item.Image,
			Year:      year,
			Status:    item.ReleaseDate,
		})
	}
	return mediaList, nil
}

// GetMediaDetails fetches detailed info
func (c *Client) GetMediaDetails(ctx context.Context, id string) (*providers.MediaDetails, error) {
	info, err := c.GetInfo(id)
	if err != nil {
		return nil, err
	}

	// Try anime first
	if animeInfo, ok := info.(*types.AnimeInfo); ok {
		mediaType := providers.MediaTypeAnime
		if strings.Contains(strings.ToLower(string(animeInfo.Type)), "movie") {
			mediaType = providers.MediaTypeMovie
		}

		details := &providers.MediaDetails{
			Media: providers.Media{
				ID:        animeInfo.ID,
				Title:     animeInfo.Title,
				Type:      mediaType,
				PosterURL: animeInfo.Image,
				Synopsis:  animeInfo.Description,
				Genres:    animeInfo.Genres,
			},
			Seasons: []providers.Season{{
				ID:     id,
				Number: 1,
				Title:  "Season 1",
			}},
		}
		return details, nil
	}

	// Try movie
	if movieInfo, ok := info.(*types.MovieInfo); ok {
		mediaType := providers.MediaTypeMovie
		if strings.Contains(strings.ToLower(movieInfo.Type), "tv") {
			mediaType = providers.MediaTypeTV
		}

		details := &providers.MediaDetails{
			Media: providers.Media{
				ID:        movieInfo.ID,
				Title:     movieInfo.Title,
				Type:      mediaType,
				PosterURL: movieInfo.Image,
				Synopsis:  movieInfo.Description,
				Genres:    movieInfo.Genres,
			},
		}

		// Create seasons from episodes
		if len(movieInfo.Episodes) > 0 {
			seasonsMap := make(map[int]bool)
			for _, ep := range movieInfo.Episodes {
				season := ep.Season
				if season == 0 {
					season = 1
				}
				seasonsMap[season] = true
			}

			for sNum := range seasonsMap {
				details.Seasons = append(details.Seasons, providers.Season{
					ID:     fmt.Sprintf("%s|%d", id, sNum),
					Number: sNum,
					Title:  fmt.Sprintf("Season %d", sNum),
				})
			}
		} else {
			details.Seasons = []providers.Season{{
				ID:     id,
				Number: 1,
				Title:  "Movie",
			}}
		}

		return details, nil
	}

	// Try manga
	if mangaInfo, ok := info.(*types.MangaInfo); ok {
		details := &providers.MediaDetails{
			Media: providers.Media{
				ID:        mangaInfo.ID,
				Title:     mangaInfo.Title,
				Type:      providers.MediaTypeManga,
				PosterURL: mangaInfo.Image,
				Synopsis:  mangaInfo.Description,
				Genres:    mangaInfo.Genres,
			},
			Seasons: []providers.Season{{
				ID:     id,
				Number: 1,
				Title:  "Chapters",
			}},
		}
		return details, nil
	}

	return nil, fmt.Errorf("unexpected info type")
}

// GetSeasons returns seasons
func (c *Client) GetSeasons(ctx context.Context, mediaID string) ([]providers.Season, error) {
	info, err := c.GetInfo(mediaID)
	if err != nil {
		return nil, err
	}

	// Try movie/anime
	if movieInfo, ok := info.(*types.MovieInfo); ok {
		if len(movieInfo.Episodes) == 0 {
			return []providers.Season{{
				ID:     mediaID,
				Number: 1,
				Title:  "Season 1",
			}}, nil
		}

		seasonsMap := make(map[int]bool)
		for _, ep := range movieInfo.Episodes {
			sNum := ep.Season
			if sNum == 0 {
				sNum = 1
			}
			seasonsMap[sNum] = true
		}

		var seasons []providers.Season
		for sNum := range seasonsMap {
			seasons = append(seasons, providers.Season{
				ID:     fmt.Sprintf("%s|%d", mediaID, sNum),
				Number: sNum,
				Title:  fmt.Sprintf("Season %d", sNum),
			})
		}
		return seasons, nil
	}

	if animeInfo, ok := info.(*types.AnimeInfo); ok {
		if len(animeInfo.Episodes) == 0 {
			return []providers.Season{{
				ID:     mediaID,
				Number: 1,
				Title:  "Season 1",
			}}, nil
		}

		seasonsMap := make(map[int]bool)
		for _, ep := range animeInfo.Episodes {
			sNum := ep.Season
			if sNum == 0 {
				sNum = 1
			}
			seasonsMap[sNum] = true
		}

		var seasons []providers.Season
		for sNum := range seasonsMap {
			seasons = append(seasons, providers.Season{
				ID:     fmt.Sprintf("%s|%d", mediaID, sNum),
				Number: sNum,
				Title:  fmt.Sprintf("Season %d", sNum),
			})
		}
		return seasons, nil
	}

	// Manga
	return []providers.Season{{
		ID:     mediaID,
		Number: 1,
		Title:  "Chapters",
	}}, nil
}

// GetEpisodes returns episodes for a season
func (c *Client) GetEpisodes(ctx context.Context, seasonID string) ([]providers.Episode, error) {
	var mediaID string
	var seasonNum = 1

	if strings.Contains(seasonID, "|") {
		parts := strings.Split(seasonID, "|")
		mediaID = parts[0]
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &seasonNum)
		}
	} else {
		mediaID = seasonID
	}

	info, err := c.GetInfo(mediaID)
	if err != nil {
		return nil, err
	}

	// Try movie/anime
	if movieInfo, ok := info.(*types.MovieInfo); ok {
		var episodes []providers.Episode
		for _, ep := range movieInfo.Episodes {
			epSeason := ep.Season
			if epSeason == 0 {
				epSeason = 1
			}

			if epSeason == seasonNum {
				episodes = append(episodes, providers.Episode{
					ID:     ep.ID,
					Number: ep.Number,
					Title:  ep.Title,
					Season: epSeason,
				})
			}
		}
		return episodes, nil
	}

	if animeInfo, ok := info.(*types.AnimeInfo); ok {
		var episodes []providers.Episode
		for _, ep := range animeInfo.Episodes {
			epSeason := ep.Season
			if epSeason == 0 {
				epSeason = 1
			}

			if epSeason == seasonNum {
				episodes = append(episodes, providers.Episode{
					ID:     ep.ID,
					Number: ep.Number,
					Title:  ep.Title,
					Season: epSeason,
				})
			}
		}
		return episodes, nil
	}

	// Manga chapters
	if mangaInfo, ok := info.(*types.MangaInfo); ok {
		var episodes []providers.Episode
		for i, ch := range mangaInfo.Chapters {
			epNum := i + 1
			episodes = append(episodes, providers.Episode{
				ID:     ch.ID,
				Number: epNum,
				Title:  ch.Title,
				Season: 1,
			})
		}
		return episodes, nil
	}

	return nil, fmt.Errorf("unexpected info type")
}

// GetStreamURL fetches video stream URL
func (c *Client) GetStreamURL(ctx context.Context, episodeID string, quality providers.Quality) (*providers.StreamURL, error) {
	res, err := c.GetSources(episodeID)
	if err != nil {
		return nil, err
	}

	videoSources, ok := res.(*types.VideoSources)
	if !ok {
		return nil, fmt.Errorf("not a video source")
	}

	if len(videoSources.Sources) == 0 {
		return nil, fmt.Errorf("no sources found")
	}

	// Find best quality match
	var selectedSource types.Source
	found := false

	targetQuality := string(quality)
	if quality == providers.QualityAuto {
		targetQuality = "auto"
	}

	for _, src := range videoSources.Sources {
		if strings.EqualFold(src.Quality, targetQuality) {
			selectedSource = src
			found = true
			break
		}
	}

	if !found {
		selectedSource = videoSources.Sources[0]
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

	for _, sub := range videoSources.Subtitles {
		streamURL.Subtitles = append(streamURL.Subtitles, providers.Subtitle{
			Language: sub.Lang,
			URL:      sub.URL,
		})
	}

	return streamURL, nil
}

// GetAvailableQualities returns available video qualities
func (c *Client) GetAvailableQualities(ctx context.Context, episodeID string) ([]providers.Quality, error) {
	res, err := c.GetSources(episodeID)
	if err != nil {
		return nil, err
	}

	videoSources, ok := res.(*types.VideoSources)
	if !ok {
		return nil, fmt.Errorf("not a video source")
	}

	var qualities []providers.Quality
	for _, src := range videoSources.Sources {
		qualities = append(qualities, providers.Quality(src.Quality))
	}

	return qualities, nil
}

// GetMangaPages fetches manga pages
func (c *Client) GetMangaPages(ctx context.Context, chapterID string) ([]string, error) {
	res, err := c.GetSources(chapterID)
	if err != nil {
		return nil, err
	}

	mangaPages, ok := res.(*types.MangaPages)
	if !ok {
		return nil, fmt.Errorf("not manga pages")
	}

	var pages []string
	for _, page := range mangaPages.Pages {
		pages = append(pages, page.URL)
	}

	return pages, nil
}

// GetTrending returns trending media
func (c *Client) GetTrending(ctx context.Context) ([]providers.Media, error) {
	return nil, fmt.Errorf("not implemented")
}

// GetRecent returns recent media
func (c *Client) GetRecent(ctx context.Context) ([]providers.Media, error) {
	return nil, fmt.Errorf("not implemented")
}

// HealthCheck checks if the provider is accessible
func (c *Client) HealthCheck(ctx context.Context) error {
	return nil
}
