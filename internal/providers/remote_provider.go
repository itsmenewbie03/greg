package providers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/justchokingaround/greg/internal/config"
	"github.com/justchokingaround/greg/internal/providers/api"
)

// RemoteProvider adapts the local Provider interface to a remote API
type RemoteProvider struct {
	name      string
	pType     MediaType
	apiClient *api.Client
	logger    *slog.Logger
}

// NewRemoteProvider creates a new remote provider adapter
func NewRemoteProvider(name string, pType MediaType, cfg *config.Config, logger *slog.Logger) *RemoteProvider {
	return &RemoteProvider{
		name:      name,
		pType:     pType,
		apiClient: api.NewClient(cfg, logger),
		logger:    logger,
	}
}

func (p *RemoteProvider) Name() string {
	return p.name
}

func (p *RemoteProvider) Type() MediaType {
	return p.pType
}

func (p *RemoteProvider) Search(ctx context.Context, query string) ([]Media, error) {
	// Determine API media type based on provider type
	apiMediaType := "movies"
	if p.pType == MediaTypeAnime {
		apiMediaType = "anime"
	}

	resp, err := p.apiClient.Search(ctx, apiMediaType, p.name, query)
	if err != nil {
		return nil, err
	}

	var results []Media
	for _, r := range resp.Results {
		// Determine media type for the result
		mediaType := p.pType
		if p.pType == MediaTypeMovieTV || p.pType == MediaTypeAnimeMovieTV || p.pType == MediaTypeAll {
			// Try to infer from result type
			switch strings.ToLower(r.Type) {
			case "movie", "film":
				mediaType = MediaTypeMovie
			case "tv", "series":
				mediaType = MediaTypeTV
			case "anime":
				mediaType = MediaTypeAnime
			default:
				// Fallback based on provider type
				switch p.pType {
				case MediaTypeMovieTV:
					mediaType = MediaTypeMovie // Default
				case MediaTypeAnimeMovieTV:
					mediaType = MediaTypeAnime // Default
				}
			}
		}

		results = append(results, APIResultToMedia(r, mediaType))
	}

	return results, nil
}

func (p *RemoteProvider) GetTrending(ctx context.Context) ([]Media, error) {
	// Not implemented in generic remote yet
	return []Media{}, nil
}

func (p *RemoteProvider) GetRecent(ctx context.Context) ([]Media, error) {
	// Not implemented in generic remote yet
	return []Media{}, nil
}

func (p *RemoteProvider) GetMediaDetails(ctx context.Context, id string) (*MediaDetails, error) {
	apiMediaType := "movies"
	if p.pType == MediaTypeAnime {
		apiMediaType = "anime"
	}

	resp, err := p.apiClient.GetInfo(ctx, apiMediaType, p.name, id)
	if err != nil {
		return nil, err
	}

	// Determine media type
	mediaType := p.pType
	if p.pType == MediaTypeMovieTV || p.pType == MediaTypeAnimeMovieTV || p.pType == MediaTypeAll {
		switch strings.ToLower(resp.Type) {
		case "movie", "film":
			mediaType = MediaTypeMovie
		case "tv", "series":
			mediaType = MediaTypeTV
		case "anime":
			mediaType = MediaTypeAnime
		}
	}

	return APIInfoToMediaDetails(*resp, mediaType), nil
}

func (p *RemoteProvider) GetSeasons(ctx context.Context, mediaID string) ([]Season, error) {
	details, err := p.GetMediaDetails(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	return details.Seasons, nil
}

func (p *RemoteProvider) GetEpisodes(ctx context.Context, seasonID string) ([]Episode, error) {
	// This is tricky because GetEpisodes usually requires parsing the seasonID to get mediaID
	// But for remote, we might need to fetch info again or rely on cache
	// For now, we'll assume we can't easily implement this without more context or parsing logic
	// specific to the provider.
	// However, most providers use GetMediaDetails to get episodes anyway.

	// If we assume seasonID contains the mediaID, we can try to fetch info.
	// But generic remote doesn't know how to parse seasonID.

	return nil, fmt.Errorf("GetEpisodes not implemented for generic remote provider")
}

func (p *RemoteProvider) GetStreamURL(ctx context.Context, episodeID string, quality Quality) (*StreamURL, error) {
	apiMediaType := "movies"
	if p.pType == MediaTypeAnime {
		apiMediaType = "anime"
	}

	resp, err := p.apiClient.GetSources(ctx, apiMediaType, p.name, episodeID)
	if err != nil {
		return nil, err
	}

	var selectedSource *api.Source

	// Try to find exact quality match
	for i := range resp.Sources {
		q, err := ParseQuality(resp.Sources[i].Quality)
		if err == nil && q == quality {
			selectedSource = &resp.Sources[i]
			break
		}
	}

	// Fallback to auto
	if selectedSource == nil {
		for i := range resp.Sources {
			q, err := ParseQuality(resp.Sources[i].Quality)
			if err == nil && q == QualityAuto {
				selectedSource = &resp.Sources[i]
				break
			}
		}
	}

	// Fallback to first available
	if selectedSource == nil && len(resp.Sources) > 0 {
		selectedSource = &resp.Sources[0]
	}

	if selectedSource == nil {
		return nil, fmt.Errorf("no sources found")
	}

	referer := ""
	origin := ""
	if resp.Headers != nil {
		referer = resp.Headers.Referer
		origin = resp.Headers.Origin
	}

	return APISourceToStreamURL(*selectedSource, referer, origin), nil
}

func (p *RemoteProvider) GetAvailableQualities(ctx context.Context, episodeID string) ([]Quality, error) {
	apiMediaType := "movies"
	if p.pType == MediaTypeAnime {
		apiMediaType = "anime"
	}

	resp, err := p.apiClient.GetSources(ctx, apiMediaType, p.name, episodeID)
	if err != nil {
		return nil, err
	}

	qualities := make([]Quality, 0, len(resp.Sources))
	seen := make(map[Quality]bool)

	for _, source := range resp.Sources {
		q, err := ParseQuality(source.Quality)
		if err != nil {
			continue
		}
		if !seen[q] {
			qualities = append(qualities, q)
			seen[q] = true
		}
	}

	return qualities, nil
}

func (p *RemoteProvider) HealthCheck(ctx context.Context) error {
	apiMediaType := "movies"
	if p.pType == MediaTypeAnime {
		apiMediaType = "anime"
	}
	return p.apiClient.HealthCheck(ctx, p.name, apiMediaType)
}
