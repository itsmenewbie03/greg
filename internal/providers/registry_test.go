package providers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock provider for testing
type mockProvider struct {
	name      string
	mediaType MediaType
}

func (m *mockProvider) Name() string                                              { return m.name }
func (m *mockProvider) Type() MediaType                                           { return m.mediaType }
func (m *mockProvider) Search(ctx context.Context, query string) ([]Media, error) { return nil, nil }
func (m *mockProvider) GetTrending(ctx context.Context) ([]Media, error)          { return nil, nil }
func (m *mockProvider) GetRecent(ctx context.Context) ([]Media, error)            { return nil, nil }
func (m *mockProvider) GetMediaDetails(ctx context.Context, id string) (*MediaDetails, error) {
	return nil, nil
}
func (m *mockProvider) GetSeasons(ctx context.Context, mediaID string) ([]Season, error) {
	return nil, nil
}
func (m *mockProvider) GetEpisodes(ctx context.Context, seasonID string) ([]Episode, error) {
	return nil, nil
}
func (m *mockProvider) GetStreamURL(ctx context.Context, episodeID string, quality Quality) (*StreamURL, error) {
	return nil, nil
}
func (m *mockProvider) GetAvailableQualities(ctx context.Context, episodeID string) ([]Quality, error) {
	return nil, nil
}
func (m *mockProvider) HealthCheck(ctx context.Context) error { return nil }

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	assert.NotNil(t, registry)
	assert.Equal(t, 0, registry.Count())
}

func TestRegistry_Register(t *testing.T) {
	t.Run("registers provider successfully", func(t *testing.T) {
		registry := NewRegistry()
		provider := &mockProvider{name: "test", mediaType: MediaTypeAnime}

		err := registry.Register(provider)
		assert.NoError(t, err)
		assert.Equal(t, 1, registry.Count())
	})

	t.Run("prevents duplicate registration", func(t *testing.T) {
		registry := NewRegistry()
		provider := &mockProvider{name: "test", mediaType: MediaTypeAnime}

		err := registry.Register(provider)
		require.NoError(t, err)

		err = registry.Register(provider)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("rejects nil provider", func(t *testing.T) {
		registry := NewRegistry()
		err := registry.Register(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil provider")
	})

	t.Run("rejects provider without name", func(t *testing.T) {
		registry := NewRegistry()
		provider := &mockProvider{name: "", mediaType: MediaTypeAnime}

		err := registry.Register(provider)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must have a name")
	})

	t.Run("registers provider by type", func(t *testing.T) {
		registry := NewRegistry()
		provider := &mockProvider{name: "anime-provider", mediaType: MediaTypeAnime}

		err := registry.Register(provider)
		require.NoError(t, err)

		providers := registry.GetByType(MediaTypeAnime)
		assert.Len(t, providers, 1)
		assert.Equal(t, "anime-provider", providers[0].Name())
	})

	t.Run("registers MovieTV provider to both types", func(t *testing.T) {
		registry := NewRegistry()
		provider := &mockProvider{name: "movietv-provider", mediaType: MediaTypeMovieTV}

		err := registry.Register(provider)
		require.NoError(t, err)

		movieProviders := registry.GetByType(MediaTypeMovie)
		tvProviders := registry.GetByType(MediaTypeTV)

		assert.Len(t, movieProviders, 1)
		assert.Len(t, tvProviders, 1)
		assert.Equal(t, "movietv-provider", movieProviders[0].Name())
		assert.Equal(t, "movietv-provider", tvProviders[0].Name())
	})

	t.Run("registers All provider to all types", func(t *testing.T) {
		registry := NewRegistry()
		provider := &mockProvider{name: "all-provider", mediaType: MediaTypeAll}

		err := registry.Register(provider)
		require.NoError(t, err)

		assert.Len(t, registry.GetByType(MediaTypeAnime), 1)
		assert.Len(t, registry.GetByType(MediaTypeMovie), 1)
		assert.Len(t, registry.GetByType(MediaTypeTV), 1)
		assert.Len(t, registry.GetByType(MediaTypeMovieTV), 1)
	})
}

func TestRegistry_Unregister(t *testing.T) {
	t.Run("unregisters provider successfully", func(t *testing.T) {
		registry := NewRegistry()
		provider := &mockProvider{name: "test", mediaType: MediaTypeAnime}

		err := registry.Register(provider)
		require.NoError(t, err)

		err = registry.Unregister("test")
		assert.NoError(t, err)
		assert.Equal(t, 0, registry.Count())
	})

	t.Run("returns error for non-existent provider", func(t *testing.T) {
		registry := NewRegistry()
		err := registry.Unregister("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not registered")
	})

	t.Run("removes provider from type map", func(t *testing.T) {
		registry := NewRegistry()
		provider := &mockProvider{name: "test", mediaType: MediaTypeAnime}

		err := registry.Register(provider)
		require.NoError(t, err)

		err = registry.Unregister("test")
		require.NoError(t, err)

		providers := registry.GetByType(MediaTypeAnime)
		assert.Empty(t, providers)
	})
}

func TestRegistry_Get(t *testing.T) {
	t.Run("gets provider by name", func(t *testing.T) {
		registry := NewRegistry()
		provider := &mockProvider{name: "test", mediaType: MediaTypeAnime}

		err := registry.Register(provider)
		require.NoError(t, err)

		retrieved, err := registry.Get("test")
		assert.NoError(t, err)
		assert.Equal(t, "test", retrieved.Name())
	})

	t.Run("returns error for non-existent provider", func(t *testing.T) {
		registry := NewRegistry()
		_, err := registry.Get("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRegistry_GetByType(t *testing.T) {
	t.Run("gets providers by type", func(t *testing.T) {
		registry := NewRegistry()

		anime1 := &mockProvider{name: "anime1", mediaType: MediaTypeAnime}
		anime2 := &mockProvider{name: "anime2", mediaType: MediaTypeAnime}
		movie := &mockProvider{name: "movie1", mediaType: MediaTypeMovie}

		_ = registry.Register(anime1)
		_ = registry.Register(anime2)
		_ = registry.Register(movie)

		animeProviders := registry.GetByType(MediaTypeAnime)
		movieProviders := registry.GetByType(MediaTypeMovie)

		assert.Len(t, animeProviders, 2)
		assert.Len(t, movieProviders, 1)
	})

	t.Run("returns empty slice for type with no providers", func(t *testing.T) {
		registry := NewRegistry()
		providers := registry.GetByType(MediaTypeAnime)
		assert.Empty(t, providers)
	})

	t.Run("returns copy of provider slice", func(t *testing.T) {
		registry := NewRegistry()
		provider := &mockProvider{name: "test", mediaType: MediaTypeAnime}

		_ = registry.Register(provider)

		providers1 := registry.GetByType(MediaTypeAnime)
		providers2 := registry.GetByType(MediaTypeAnime)

		// Modifying one slice shouldn't affect the other
		providers1[0] = &mockProvider{name: "modified", mediaType: MediaTypeAnime}
		assert.NotEqual(t, providers1[0].Name(), providers2[0].Name())
	})
}

func TestRegistry_GetAll(t *testing.T) {
	t.Run("gets all providers", func(t *testing.T) {
		registry := NewRegistry()

		anime := &mockProvider{name: "anime", mediaType: MediaTypeAnime}
		movie := &mockProvider{name: "movie", mediaType: MediaTypeMovie}

		_ = registry.Register(anime)
		_ = registry.Register(movie)

		all := registry.GetAll()
		assert.Len(t, all, 2)
	})

	t.Run("returns empty slice when no providers", func(t *testing.T) {
		registry := NewRegistry()
		all := registry.GetAll()
		assert.Empty(t, all)
	})
}

func TestRegistry_List(t *testing.T) {
	t.Run("lists provider names", func(t *testing.T) {
		registry := NewRegistry()

		anime := &mockProvider{name: "anime", mediaType: MediaTypeAnime}
		movie := &mockProvider{name: "movie", mediaType: MediaTypeMovie}

		_ = registry.Register(anime)
		_ = registry.Register(movie)

		names := registry.List()
		assert.Len(t, names, 2)
		assert.Contains(t, names, "anime")
		assert.Contains(t, names, "movie")
	})

	t.Run("returns empty slice when no providers", func(t *testing.T) {
		registry := NewRegistry()
		names := registry.List()
		assert.Empty(t, names)
	})
}

func TestRegistry_Count(t *testing.T) {
	t.Run("counts registered providers", func(t *testing.T) {
		registry := NewRegistry()

		assert.Equal(t, 0, registry.Count())

		_ = registry.Register(&mockProvider{name: "p1", mediaType: MediaTypeAnime})
		assert.Equal(t, 1, registry.Count())

		_ = registry.Register(&mockProvider{name: "p2", mediaType: MediaTypeMovie})
		assert.Equal(t, 2, registry.Count())

		_ = registry.Unregister("p1")
		assert.Equal(t, 1, registry.Count())
	})
}

func TestRegistry_Clear(t *testing.T) {
	t.Run("clears all providers", func(t *testing.T) {
		registry := NewRegistry()

		_ = registry.Register(&mockProvider{name: "p1", mediaType: MediaTypeAnime})
		_ = registry.Register(&mockProvider{name: "p2", mediaType: MediaTypeMovie})

		assert.Equal(t, 2, registry.Count())

		registry.Clear()

		assert.Equal(t, 0, registry.Count())
		assert.Empty(t, registry.GetAll())
		assert.Empty(t, registry.List())
	})
}

func TestGlobalRegistry(t *testing.T) {
	// Clean up global registry after test
	defer Clear()

	t.Run("global Register function", func(t *testing.T) {
		Clear() // Start clean

		provider := &mockProvider{name: "global-test", mediaType: MediaTypeAnime}
		err := Register(provider)

		assert.NoError(t, err)
		assert.Equal(t, 1, Count())
	})

	t.Run("global Get function", func(t *testing.T) {
		Clear()

		provider := &mockProvider{name: "global-test", mediaType: MediaTypeAnime}
		_ = Register(provider)

		retrieved, err := Get("global-test")
		assert.NoError(t, err)
		assert.Equal(t, "global-test", retrieved.Name())
	})

	t.Run("global GetByType function", func(t *testing.T) {
		Clear()

		provider := &mockProvider{name: "global-test", mediaType: MediaTypeAnime}
		_ = Register(provider)

		providers := GetByType(MediaTypeAnime)
		assert.Len(t, providers, 1)
	})
}
