package registry

import (
	"fmt"
	"strings"

	"github.com/justchokingaround/greg/internal/config"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/providers/anime/allanime"
	"github.com/justchokingaround/greg/internal/providers/anime/hdrezka"
	"github.com/justchokingaround/greg/internal/providers/anime/hianime"
	"github.com/justchokingaround/greg/internal/providers/manga/comix"
	"github.com/justchokingaround/greg/internal/providers/movies/flixhq"
	hdrezkamovie "github.com/justchokingaround/greg/internal/providers/movies/hdrezka"
	"github.com/justchokingaround/greg/internal/providers/movies/sflix"
	"github.com/justchokingaround/greg/internal/providers/remote"
)

type Registry struct {
	providers map[string]providers.Provider
}

func New() *Registry {
	return &Registry{
		providers: make(map[string]providers.Provider),
	}
}

func (r *Registry) Load(cfg *config.Config) {
	// Helper to register a provider
	register := func(name string, settings config.ProviderSettings, localFactory func() providers.Provider, mediaType string) {
		if !settings.Enabled {
			return
		}

		if settings.Mode == "remote" {
			// Use remote client
			// If RemoteURL is not set, we can't use it
			if settings.RemoteURL != "" {
				url := settings.RemoteURL
				// If URL is generic (no type/name path), append them
				// This assumes greg-api structure: /type/name
				if !strings.Contains(url, "/"+name) {
					url = fmt.Sprintf("%s/%s/%s", strings.TrimRight(url, "/"), mediaType, name)
				}
				r.providers[name] = remote.New(name, url)
			}
		} else {
			// Use local factory
			if localFactory != nil {
				r.providers[name] = localFactory()
			}
		}
	}

	// Register known providers
	register("hianime", cfg.Providers.HiAnime, func() providers.Provider { return hianime.New() }, "anime")
	register("allanime", cfg.Providers.AllAnime, func() providers.Provider { return allanime.New() }, "anime")
	register("sflix", cfg.Providers.SFlix, func() providers.Provider { return sflix.New() }, "movies")
	register("flixhq", cfg.Providers.FlixHQ, func() providers.Provider { return flixhq.New() }, "movies")
	register("hdrezka", cfg.Providers.HDRezka, func() providers.Provider { return hdrezkamovie.New() }, "movies")
	register("hdrezka_anime", cfg.Providers.HDRezka, func() providers.Provider { return hdrezka.New() }, "anime") // Special case for anime wrapper
	register("comix", cfg.Providers.Comix, func() providers.Provider { return comix.New() }, "manga")
}

func (r *Registry) Get(name string) (providers.Provider, error) {
	if p, ok := r.providers[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("provider not found: %s", name)
}

func (r *Registry) List() []string {
	keys := make([]string, 0, len(r.providers))
	for k := range r.providers {
		keys = append(keys, k)
	}
	return keys
}
