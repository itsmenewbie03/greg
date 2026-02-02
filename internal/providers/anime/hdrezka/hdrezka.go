package hdrezka

import (
	"context"

	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/providers/movies/hdrezka"
	"github.com/justchokingaround/greg/pkg/types"
)

type HDRezka struct {
	*hdrezka.HDRezka
}

func New() *HDRezka {
	return &HDRezka{
		HDRezka: hdrezka.New(),
	}
}

func (p *HDRezka) Name() string {
	return "hdrezka_anime"
}

func (p *HDRezka) Type() providers.MediaType {
	return providers.MediaTypeAnime
}

func (p *HDRezka) Search(ctx context.Context, query string) ([]providers.Media, error) {
	return p.HDRezka.Search(ctx, query)
}

func (p *HDRezka) GetMediaDetails(ctx context.Context, id string) (*providers.MediaDetails, error) {
	return p.HDRezka.GetMediaDetails(ctx, id)
}

func (p *HDRezka) GetSeasons(ctx context.Context, mediaID string) ([]providers.Season, error) {
	return p.HDRezka.GetSeasons(ctx, mediaID)
}

func (p *HDRezka) GetEpisodes(ctx context.Context, seasonID string) ([]providers.Episode, error) {
	return p.HDRezka.GetEpisodes(ctx, seasonID)
}

func (p *HDRezka) GetStreamURL(ctx context.Context, episodeID string, quality providers.Quality) (*providers.StreamURL, error) {
	return p.HDRezka.GetStreamURL(ctx, episodeID, quality)
}

func (p *HDRezka) GetAvailableQualities(ctx context.Context, episodeID string) ([]providers.Quality, error) {
	return p.HDRezka.GetAvailableQualities(ctx, episodeID)
}

func (p *HDRezka) GetTrending(ctx context.Context) ([]providers.Media, error) {
	return p.HDRezka.GetTrending(ctx)
}

func (p *HDRezka) GetRecent(ctx context.Context) ([]providers.Media, error) {
	return p.HDRezka.GetRecent(ctx)
}

func (p *HDRezka) HealthCheck(ctx context.Context) error {
	return p.HDRezka.HealthCheck(ctx)
}

func (p *HDRezka) GetInfo(id string) (interface{}, error) {
	info, err := p.HDRezka.GetInfo(id)
	if err != nil {
		return nil, err
	}

	movieInfo, ok := info.(*types.MovieInfo)
	if !ok {
		return info, nil
	}

	return &types.AnimeInfo{
		ID:          movieInfo.ID,
		Title:       movieInfo.Title,
		Image:       movieInfo.Image,
		Description: movieInfo.Description,
		Episodes:    movieInfo.Episodes,
		Genres:      movieInfo.Genres,
		ReleaseDate: movieInfo.ReleaseDate,
		// Status and Type might need mapping if available in MovieInfo
	}, nil
}
