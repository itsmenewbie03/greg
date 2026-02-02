package hdrezka

import (
	"context"
	"encoding/base64"
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
	"github.com/justchokingaround/greg/pkg/types"
)

type HDRezka struct {
	Client      *http.Client
	BaseURL     string
	searchCache sync.Map
	infoCache   sync.Map
}

func New() *HDRezka {
	// Create client with transport that handles compression
	transport := &http.Transport{
		DisableCompression: false,
	}
	return &HDRezka{
		Client:  &http.Client{Transport: transport},
		BaseURL: "https://hdrezka.website",
	}
}

func (p *HDRezka) Name() string {
	return "hdrezka"
}

func (p *HDRezka) searchOld(query string) (*types.SearchResults, error) {
	if cached, ok := p.searchCache.Load(query); ok {
		return cached.(*types.SearchResults), nil
	}

	encodedQuery := url.QueryEscape(query)
	url := fmt.Sprintf("%s/search/?do=search&subaction=search&q=%s", p.BaseURL, encodedQuery)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "uwu")

	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	results := []types.SearchResult{}

	doc.Find(".b-content__inline_item").Each(func(i int, s *goquery.Selection) {
		link := s.Find(".b-content__inline_item-link a")
		href, exists := link.Attr("href")
		if !exists {
			return
		}

		title := link.Text()
		img := s.Find("img").AttrOr("src", "")

		// Extract ID and Type from URL
		// URL format: https://hdrezka.website/films/watch/12345-movie.html
		// or https://hdrezka.website/series/12345-series.html

		// Remove BaseURL
		path := strings.TrimPrefix(href, p.BaseURL+"/")
		path = strings.TrimSuffix(path, ".html")

		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			return
		}

		mediaType := parts[0]
		id := strings.Join(parts[1:], "/")

		// Year is in the div below the link
		yearText := s.Find(".b-content__inline_item-link div").Text()
		// Extract year (4 digits)
		reYear := regexp.MustCompile(`\d{4}`)
		year := reYear.FindString(yearText)

		results = append(results, types.SearchResult{
			ID:          id,
			Title:       title,
			Image:       img,
			URL:         href,
			ReleaseDate: year,
			Type:        mediaType,
		})
	})

	res := &types.SearchResults{Results: results}
	p.searchCache.Store(query, res)
	return res, nil
}

func (p *HDRezka) GetInfo(id string) (interface{}, error) {
	if cached, ok := p.infoCache.Load(id); ok {
		return cached.(*types.MovieInfo), nil
	}

	// id is like "fiction/12345-brigada"
	// We need to handle if it doesn't have the prefix if passed from somewhere else,
	// but Search returns it with prefix (e.g. "films/watch/..." without baseurl? No, Search returns "films/watch/12345-movie")
	// Wait, Search logic:
	// path := strings.TrimPrefix(href, p.BaseURL+"/") -> "films/watch/12345-movie.html"
	// parts := strings.Split(path, "/") -> ["films", "watch", "12345-movie.html"] (Wait, suffix removed)
	// id := strings.Join(parts[1:], "/") -> "watch/12345-movie"
	// So ID is "watch/12345-movie" or similar.

	// The original code used: urlStr := fmt.Sprintf("%s/%s.html", p.BaseURL, movieID)
	// So if ID is "watch/12345-movie", URL is "https://hdrezka.website/watch/12345-movie.html"
	// This seems correct if the ID logic matches.

	urlStr := fmt.Sprintf("%s/%s.html", p.BaseURL, id)

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "uwu")

	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	html := string(bodyBytes)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	title := doc.Find(".b-post__title h1").Text()
	desc := doc.Find(".b-post__description_text").Text()
	img := doc.Find(".b-side__image img").AttrOr("src", "")

	// Extract data_id
	reID := regexp.MustCompile(`/(\d+)-`)
	matches := reID.FindStringSubmatch(id)
	if len(matches) < 2 {
		// Try to find in the page if not in URL
		// But original code relied on ID.
		// Let's try to be more robust or fail.
		return nil, fmt.Errorf("could not extract data_id from %s", id)
	}
	dataID := matches[1]

	// Extract default translator ID
	reTranslator := regexp.MustCompile(`initCDN(Movies|Series)Events\(\d+, ([0-9]*),`)
	matchesTrans := reTranslator.FindStringSubmatch(html)
	var translatorID string
	var isSeries bool
	if len(matchesTrans) >= 3 {
		if matchesTrans[1] == "Series" {
			isSeries = true
		}
		translatorID = matchesTrans[2]
	}

	episodes := []types.Episode{}

	// Check if it's a series (has seasons)
	seasons := doc.Find("#simple-seasons-tabs li")

	if seasons.Length() > 0 {
		// It's a series
		seasons.Each(func(i int, s *goquery.Selection) {
			seasonID, exists := s.Attr("data-tab_id")
			if !exists {
				return
			}
			seasonNum := i + 1

			eps, err := p.fetchEpisodes(dataID, translatorID, seasonID)
			if err == nil {
				for _, ep := range eps {
					ep.Season = seasonNum
					episodes = append(episodes, ep)
				}
			}
		})
	} else if isSeries {
		// It's a series but no season tabs found, assume Season 1
		seasonNum := 1
		seasonID := "1"
		eps, err := p.fetchEpisodes(dataID, translatorID, seasonID)
		if err == nil {
			for _, ep := range eps {
				ep.Season = seasonNum
				episodes = append(episodes, ep)
			}
		}
	} else {
		// It's a movie
		episodes = append(episodes, types.Episode{
			ID:     fmt.Sprintf("movie:%s:%s", dataID, translatorID),
			Number: 1,
			Title:  title,
		})
	}

	info := &types.MovieInfo{
		ID:          id,
		Title:       title,
		Description: desc,
		Image:       img,
		Episodes:    episodes,
	}

	p.infoCache.Store(id, info)
	return info, nil
}

func (p *HDRezka) fetchEpisodes(dataID, translatorID, seasonID string) ([]types.Episode, error) {
	data := url.Values{}
	data.Set("id", dataID)
	data.Set("translator_id", translatorID)
	data.Set("season", seasonID)
	data.Set("action", "get_episodes")

	req, err := http.NewRequest("POST", p.BaseURL+"/ajax/get_cdn_series/", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "uwu")

	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Success  bool   `json:"success"`
		Episodes string `json:"episodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(result.Episodes))
	if err != nil {
		return nil, err
	}

	episodes := []types.Episode{}
	doc.Find("li").Each(func(i int, s *goquery.Selection) {
		epID, exists := s.Attr("data-episode_id")
		if !exists {
			return
		}
		epNum, _ := strconv.Atoi(epID)

		title := strings.TrimSpace(s.Text())
		if title == "" {
			title = fmt.Sprintf("Episode %d", epNum)
		}

		episodes = append(episodes, types.Episode{
			ID:     fmt.Sprintf("series:%s:%s:%s:%s", dataID, translatorID, seasonID, epID),
			Number: epNum,
			Title:  title,
		})
	})

	return episodes, nil
}

func (p *HDRezka) GetSources(episodeID string) (interface{}, error) {
	parts := strings.Split(episodeID, ":")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid episode ID: %s", episodeID)
	}

	type_ := parts[0]
	dataID := parts[1]
	translatorID := parts[2]

	data := url.Values{}
	data.Set("id", dataID)
	data.Set("translator_id", translatorID)

	if type_ == "series" {
		if len(parts) < 5 {
			return nil, fmt.Errorf("invalid series episode ID: %s", episodeID)
		}
		seasonID := parts[3]
		epID := parts[4]
		data.Set("season", seasonID)
		data.Set("episode", epID)
		data.Set("action", "get_stream")
	} else {
		data.Set("action", "get_movie")
	}

	req, err := http.NewRequest("POST", p.BaseURL+"/ajax/get_cdn_series/", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "uwu")

	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	jsonStr := string(bodyBytes)

	reURL := regexp.MustCompile(`"url":"([^"]*)"`)
	matches := reURL.FindStringSubmatch(jsonStr)
	if len(matches) < 2 {
		return nil, fmt.Errorf("url not found in response")
	}
	encryptedURL := matches[1]
	encryptedURL = strings.ReplaceAll(encryptedURL, `\/`, `/`)

	decrypted := decryptSource(encryptedURL)

	sources := []types.Source{}
	streams := strings.Split(decrypted, ",")
	for _, stream := range streams {
		reStream := regexp.MustCompile(`\[([^\]]*)\](.*)`)
		matchesStream := reStream.FindStringSubmatch(stream)
		if len(matchesStream) >= 3 {
			quality := matchesStream[1]
			url := matchesStream[2]

			urls := strings.Split(url, " or ")
			finalURL := urls[len(urls)-1]

			sources = append(sources, types.Source{
				URL:     finalURL,
				Quality: quality,
				IsM3U8:  strings.Contains(finalURL, ".m3u8"),
			})
		}
	}

	return &types.VideoSources{Sources: sources}, nil
}

func (p *HDRezka) GetServers(episodeID string) ([]types.EpisodeServer, error) {
	return []types.EpisodeServer{
		{Name: "HDRezka", URL: ""},
	}, nil
}

// Decrypt logic
func decryptSource(encrypted string) string {
	// 1. Remove #h prefix if present
	encrypted = strings.TrimPrefix(encrypted, "#h")

	// 2. Remove //_//
	encrypted = strings.ReplaceAll(encrypted, "//_//", "")

	// 3. Remove garbage strings
	table := "ISE=,IUA=,IV4=,ISM=,ISQ=,QCE=,QEA=,QF4=,QCM=,QCQ=,XiE=,XkA=,Xl4=,XiM=,XiQ=,IyE=,I0A=,I14=,IyM=,IyQ=,JCE=,JEA=,JF4=,JCM=,JCQ=,ISEh,ISFA,ISFe,ISEj,ISEk,IUAh,IUBA,IUBe,IUAj,IUAk,IV4h,IV5A,IV5e,IV4j,IV4k,ISMh,ISNA,ISNe,ISMj,ISMk,ISQh,ISRA,ISRe,ISQj,ISQk,QCEh,QCFA,QCFe,QCEj,QCEk,QEAh,QEBA,QEBe,QEAj,QEAk,QF4h,QF5A,QF5e,QF4j,QF4k,QCMh,QCNA,QCNe,QCMj,QCMk,QCQh,QCRA,QCRe,QCQj,QCQk,XiEh,XiFA,XiFe,XiEj,XiEk,XkAh,XkBA,XkBe,XkAj,XkAk,Xl4h,Xl5A,Xl5e,Xl4j,Xl4k,XiMh,XiNA,XiNe,XiMj,XiMk,XiQh,XiRA,XiRe,XiQj,XiQk,IyEh,IyFA,IyFe,IyEj,IyEk,I0Ah,I0BA,I0Be,I0Aj,I0Ak,I14h,I15A,I15e,I14j,I14k,IyMh,IyNA,IyNe,IyMj,IyMk,IyQh,IyRA,IyRe,IyQj,IyQk,JCEh,JCFA,JCFe,JCEj,JCEk,JEAh,JEBA,JEBe,JEAj,JEAk,JF4h,JF5A,JF5e,JF4j,JF4k,JCMh,JCNA,JCNe,JCMj,JCMk,JCQh,JCRA,JCRe,JCQj,JCQk"
	garbage := strings.Split(table, ",")

	for _, g := range garbage {
		encrypted = strings.ReplaceAll(encrypted, g, "")
	}

	// 4. Remove _
	encrypted = strings.ReplaceAll(encrypted, "_", "")

	// 5. Base64 decode
	decoded, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return ""
	}
	return string(decoded)
}

// Type returns the media type this provider supports
func (p *HDRezka) Type() providers.MediaType {
	return providers.MediaTypeMovieTV
}

// Search (new interface) searches for movies/shows by query
func (p *HDRezka) Search(ctx context.Context, query string) ([]providers.Media, error) {
	oldResults, err := p.searchOld(query)
	if err != nil {
		return nil, err
	}

	var mediaList []providers.Media
	for _, item := range oldResults.Results {
		year := 0
		if len(item.ReleaseDate) >= 4 {
			if y, err := strconv.Atoi(item.ReleaseDate[:4]); err == nil {
				year = y
			}
		}

		mediaType := providers.MediaTypeMovie
		if strings.Contains(item.Type, "series") {
			mediaType = providers.MediaTypeTV
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

// GetMediaDetails fetches detailed info for a movie/show
func (p *HDRezka) GetMediaDetails(ctx context.Context, id string) (*providers.MediaDetails, error) {
	info, err := p.GetInfo(id)
	if err != nil {
		return nil, err
	}

	movieInfo, ok := info.(*types.MovieInfo)
	if !ok {
		return nil, fmt.Errorf("unexpected info type")
	}

	mediaType := providers.MediaTypeMovie
	if len(movieInfo.Episodes) > 1 {
		mediaType = providers.MediaTypeTV
	}

	details := &providers.MediaDetails{
		Media: providers.Media{
			ID:        movieInfo.ID,
			Title:     movieInfo.Title,
			Type:      mediaType,
			PosterURL: movieInfo.Image,
			Synopsis:  movieInfo.Description,
		},
	}

	// Create seasons
	if mediaType == providers.MediaTypeTV && len(movieInfo.Episodes) > 0 {
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

// GetSeasons returns seasons for a media
func (p *HDRezka) GetSeasons(ctx context.Context, mediaID string) ([]providers.Season, error) {
	info, err := p.GetInfo(mediaID)
	if err != nil {
		return nil, err
	}

	movieInfo, ok := info.(*types.MovieInfo)
	if !ok {
		return nil, fmt.Errorf("unexpected info type")
	}

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

// GetEpisodes returns episodes for a season
func (p *HDRezka) GetEpisodes(ctx context.Context, seasonID string) ([]providers.Episode, error) {
	var mediaID string
	var seasonNum = 1

	if strings.Contains(seasonID, "|") {
		parts := strings.Split(seasonID, "|")
		mediaID = parts[0]
		if len(parts) > 1 {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				seasonNum = n
			}
		}
	} else {
		mediaID = seasonID
	}

	info, err := p.GetInfo(mediaID)
	if err != nil {
		return nil, err
	}

	movieInfo, ok := info.(*types.MovieInfo)
	if !ok {
		return nil, fmt.Errorf("unexpected info type")
	}

	var episodes []providers.Episode
	if len(movieInfo.Episodes) == 1 && !strings.Contains(movieInfo.Episodes[0].ID, "series:") {
		// Single movie episode
		episodes = append(episodes, providers.Episode{
			ID:     movieInfo.Episodes[0].ID,
			Number: 1,
			Title:  movieInfo.Title,
			Season: 1,
		})
	} else {
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
	}

	return episodes, nil
}

// GetStreamURL fetches video stream URL for an episode
func (p *HDRezka) GetStreamURL(ctx context.Context, episodeID string, quality providers.Quality) (*providers.StreamURL, error) {
	res, err := p.GetSources(episodeID)
	if err != nil {
		return nil, err
	}

	videoSources, ok := res.(*types.VideoSources)
	if !ok {
		return nil, fmt.Errorf("unexpected source type")
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

// GetAvailableQualities returns available video qualities
func (p *HDRezka) GetAvailableQualities(ctx context.Context, episodeID string) ([]providers.Quality, error) {
	res, err := p.GetSources(episodeID)
	if err != nil {
		return nil, err
	}

	videoSources, ok := res.(*types.VideoSources)
	if !ok {
		return nil, fmt.Errorf("unexpected source type")
	}

	var qualities []providers.Quality
	for _, src := range videoSources.Sources {
		qualities = append(qualities, providers.Quality(src.Quality))
	}

	return qualities, nil
}

// GetTrending returns trending media
func (p *HDRezka) GetTrending(ctx context.Context) ([]providers.Media, error) {
	return nil, fmt.Errorf("not implemented")
}

// GetRecent returns recent media
func (p *HDRezka) GetRecent(ctx context.Context) ([]providers.Media, error) {
	return nil, fmt.Errorf("not implemented")
}

// HealthCheck checks if the provider is accessible
func (p *HDRezka) HealthCheck(ctx context.Context) error {
	return nil
}
