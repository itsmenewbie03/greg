package mapping

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"github.com/justchokingaround/greg/internal/database"
	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/providers/utils"
)

// Manager handles AniList to provider mappings
type Manager struct {
	db                *gorm.DB
	preferredProvider string
	minMatchScore     float64 // Minimum similarity score for fuzzy matching (0.0-1.0)
	debug             bool
	logger            *slog.Logger
}

// NewManager creates a new mapping manager
func NewManager(db *gorm.DB, preferredProvider string, logger *slog.Logger) *Manager {
	return NewManagerWithDebug(db, preferredProvider, false, logger)
}

// NewManagerWithDebug creates a new mapping manager with debug flag
func NewManagerWithDebug(db *gorm.DB, preferredProvider string, debug bool, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		db:                db,
		preferredProvider: preferredProvider,
		minMatchScore:     0.7,   // Default threshold: 70% similarity
		debug:             false, // Always disable debug output to avoid TUI pollution
		logger:            logger,
	}
}

// SetMinMatchScore sets the minimum similarity score for fuzzy matching
func (m *Manager) SetMinMatchScore(score float64) {
	if score >= 0.0 && score <= 1.0 {
		m.minMatchScore = score
	}
}

// ProviderMapping represents a mapping result
type ProviderMapping struct {
	AniListID       int
	ProviderName    string
	ProviderMediaID string
	Media           *providers.Media // The actual media details from provider
	IsNew           bool             // True if this is a new mapping
}

// SearchResult represents a provider search result with similarity score
type SearchResult struct {
	ProviderName string
	Match        utils.MatchResult
}

// GetMapping retrieves an existing mapping for an AniList ID
func (m *Manager) GetMapping(ctx context.Context, anilistID int) (*ProviderMapping, error) {
	m.logger.Debug("querying database for mapping", "anilist_id", anilistID)

	var mapping database.AniListMapping
	err := m.db.Where("anilist_id = ?", anilistID).First(&mapping).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			m.logger.Debug("no mapping found in database")
			return nil, nil // No mapping exists
		}
		m.logger.Error("database query failed", "error", err)
		return nil, fmt.Errorf("failed to get mapping: %w", err)
	}

	m.logger.Debug("found mapping in database", "provider", mapping.ProviderName, "media_id", mapping.ProviderMediaID)

	return &ProviderMapping{
		AniListID:       mapping.AniListID,
		ProviderName:    mapping.ProviderName,
		ProviderMediaID: mapping.ProviderMediaID,
		IsNew:           false,
	}, nil
}

// SearchProviders searches for anime across providers using smart fallback strategy
// Returns all matches above the minimum similarity threshold, sorted by score
func (m *Manager) SearchProviders(ctx context.Context, title string, mediaType providers.MediaType) ([]SearchResult, error) {
	var allResults []SearchResult

	// Get providers for this media type
	providersList := providers.GetByType(mediaType)
	if len(providersList) == 0 {
		return nil, fmt.Errorf("no providers available for media type: %s", mediaType)
	}

	// Try preferred provider first if specified
	if m.preferredProvider != "" {
		provider, err := providers.Get(m.preferredProvider)
		if err == nil && provider.Type() == mediaType {
			results, err := m.searchProvider(ctx, provider, title)
			if err == nil && len(results) > 0 {
				for _, match := range results {
					allResults = append(allResults, SearchResult{
						ProviderName: provider.Name(),
						Match:        match,
					})
				}
				// If we found good matches from preferred provider, return early
				if len(results) > 0 && results[0].Score > 0.9 {
					return allResults, nil
				}
			}
		}
	}

	// Fallback: search all other providers
	for _, provider := range providersList {
		// Skip preferred provider if we already searched it
		if provider.Name() == m.preferredProvider {
			continue
		}

		results, err := m.searchProvider(ctx, provider, title)
		if err != nil {
			// Log error but continue with other providers
			continue
		}

		for _, match := range results {
			allResults = append(allResults, SearchResult{
				ProviderName: provider.Name(),
				Match:        match,
			})
		}
	}

	if len(allResults) == 0 {
		return nil, fmt.Errorf("no matches found for '%s' across any provider", title)
	}

	// Sort all results by score (descending)
	for i := 0; i < len(allResults); i++ {
		for j := i + 1; j < len(allResults); j++ {
			if allResults[j].Match.Score > allResults[i].Match.Score {
				allResults[i], allResults[j] = allResults[j], allResults[i]
			}
		}
	}

	return allResults, nil
}

// searchProvider searches a single provider and returns fuzzy matches
func (m *Manager) searchProvider(ctx context.Context, provider providers.Provider, title string) ([]utils.MatchResult, error) {
	// Search provider
	results, err := provider.Search(ctx, title)
	if err != nil {
		return nil, err
	}

	// Apply fuzzy matching
	matches := utils.FindBestMatches(title, results, m.minMatchScore)
	return matches, nil
}

// GetOrCreateMapping retrieves an existing mapping or searches the provider
// Returns the mapping if it exists, or search results for user selection
func (m *Manager) GetOrCreateMapping(ctx context.Context, anilistID int, title string, mediaType providers.MediaType) (*ProviderMapping, []providers.Media, error) {
	m.logger.Debug("checking for mapping", "anilist_id", anilistID, "title", title)

	// First, check if mapping already exists
	existing, err := m.GetMapping(ctx, anilistID)
	if err != nil {
		m.logger.Error("get mapping failed", "error", err)
		return nil, nil, fmt.Errorf("failed to check existing mapping: %w", err)
	}

	if existing != nil {
		m.logger.Debug("found existing mapping", "provider", existing.ProviderName, "media_id", existing.ProviderMediaID)
		m.logger.Info("returning existing mapping")

		// We have a valid mapping - create a minimal Media object
		// We don't need full details since we already have the provider media ID
		// The TUI will fetch seasons/episodes using this ID

		existing.Media = &providers.Media{
			ID:    existing.ProviderMediaID,
			Title: title, // Use the AniList title
			Type:  providers.MediaTypeAnime,
		}

		return existing, nil, nil
	}

	m.logger.Debug("no existing mapping found, searching provider")

	// No existing mapping, search the preferred anime provider
	animeProviders := providers.GetByType(mediaType)
	if len(animeProviders) == 0 {
		return nil, nil, fmt.Errorf("no providers available for media type: %s", mediaType)
	}

	// Use preferred provider if available, otherwise use first provider
	provider := animeProviders[0] // Default to first provider
	availableProviders := make([]string, len(animeProviders))
	for i, p := range animeProviders {
		availableProviders[i] = p.Name()
	}
	m.logger.Debug("provider selection", "preferred_provider", m.preferredProvider, "default_provider", provider.Name(), "available_providers", availableProviders)

	if m.preferredProvider != "" {
		for _, p := range animeProviders {
			if p.Name() == m.preferredProvider {
				provider = p
				m.logger.Debug("using preferred provider", "provider", provider.Name())
				break
			}
		}
	} else {
		m.logger.Warn("no preferred provider set, using default", "provider", provider.Name())
	}

	// Try multiple search queries to handle variations in naming
	// e.g., "Cyberpunk: Edgerunners" vs "Cyberpunk Edgerunners"
	// IMPORTANT: Try variations without special characters FIRST, as they tend to work better
	var searchQueries []string

	// Priority 1: Remove all special characters (most reliable)
	specialCharReplaced := regexp.MustCompile(`[^\w\s]`).ReplaceAllString(title, " ")
	cleanedSpecial := utils.CleanText(specialCharReplaced)
	if cleanedSpecial != "" && cleanedSpecial != utils.CleanText(title) {
		searchQueries = append(searchQueries, cleanedSpecial)
	}

	// Priority 2: Alpha characters only
	alphaOnly := regexp.MustCompile(`[^a-zA-Z\s]`).ReplaceAllString(title, "")
	cleanedAlpha := utils.CleanText(alphaOnly)
	if cleanedAlpha != "" && cleanedAlpha != cleanedSpecial && cleanedAlpha != utils.CleanText(title) {
		searchQueries = append(searchQueries, cleanedAlpha)
	}

	// Priority 3: Replace colon with space (common issue)
	colonReplaced := strings.ReplaceAll(title, ":", " ")
	cleanedColon := utils.CleanText(colonReplaced)
	if cleanedColon != utils.CleanText(title) && cleanedColon != cleanedSpecial {
		searchQueries = append(searchQueries, cleanedColon)
	}

	// Priority 4: Replace dash with space
	dashReplaced := strings.ReplaceAll(title, "-", " ")
	cleanedDash := utils.CleanText(dashReplaced)
	if cleanedDash != utils.CleanText(title) && cleanedDash != cleanedSpecial && cleanedDash != cleanedColon {
		searchQueries = append(searchQueries, cleanedDash)
	}

	// Priority 5: Remove parentheses
	parenRemoved := regexp.MustCompile(`\([^)]*\)`).ReplaceAllString(title, " ")
	cleanedParen := utils.CleanText(parenRemoved)
	if cleanedParen != utils.CleanText(title) && cleanedParen != cleanedSpecial {
		searchQueries = append(searchQueries, cleanedParen)
	}

	// Priority 6 (last resort): Original title as-is
	originalCleaned := utils.CleanText(title)
	searchQueries = append(searchQueries, originalCleaned)

	// Try ALL search variations and collect all unique results
	// This way if one variation returns a bad result, others might return good ones
	allResults := make(map[string]providers.Media) // Use map to deduplicate by ID
	var lastErr error

	for _, query := range searchQueries {
		m.logger.Debug("trying search query", "query", query)

		queryResults, err := provider.Search(ctx, query)
		if err != nil {
			m.logger.Debug("search error for query", "query", query, "error", err)
			lastErr = err
			continue
		}

		if len(queryResults) > 0 {
			m.logger.Debug("found results for query", "count", len(queryResults), "query", query)
			// Add all results to our map (deduplicates by ID)
			for _, result := range queryResults {
				if _, exists := allResults[result.ID]; !exists {
					allResults[result.ID] = result
				}
			}
		} else {
			m.logger.Debug("no results found for query", "query", query)
		}
	}

	// Convert map to slice
	if len(allResults) > 0 {
		results := make([]providers.Media, 0, len(allResults))
		for _, media := range allResults {
			results = append(results, media)
		}

		m.logger.Debug("total unique results across queries", "count", len(results))

		return nil, results, nil
	}

	// If we get here, all variations failed - return the last error if any, or a general error
	if lastErr != nil {
		return nil, nil, fmt.Errorf("failed to search provider with all variations: %w", lastErr)
	}

	return nil, nil, fmt.Errorf("no results found for '%s' or any of its variations", title)
}

// SaveMapping persists a mapping to the database
func (m *Manager) SaveMapping(ctx context.Context, mapping *ProviderMapping) error {
	dbMapping := database.AniListMapping{
		AniListID:       mapping.AniListID,
		ProviderName:    mapping.ProviderName,
		ProviderMediaID: mapping.ProviderMediaID,
		Title:           "",
	}

	if mapping.Media != nil {
		dbMapping.Title = mapping.Media.Title
	}

	// Upsert: Update if exists, create if not
	result := m.db.Where("ani_list_id = ?", mapping.AniListID).Assign(dbMapping).FirstOrCreate(&dbMapping)
	if result.Error != nil {
		return fmt.Errorf("failed to save mapping: %w", result.Error)
	}

	return nil
}

// RemapMedia searches for a new provider mapping and updates the database
// Used when user wants to manually change the mapping
func (m *Manager) RemapMedia(ctx context.Context, anilistID int, title string, mediaType providers.MediaType) ([]providers.Media, error) {
	// Delete existing mapping
	if err := m.DeleteMapping(ctx, anilistID); err != nil {
		return nil, fmt.Errorf("failed to delete existing mapping: %w", err)
	}

	// Search the anime provider
	animeProviders := providers.GetByType(mediaType)
	if len(animeProviders) == 0 {
		return nil, fmt.Errorf("no providers available for media type: %s", mediaType)
	}

	provider := animeProviders[0]
	results, err := provider.Search(ctx, title)
	if err != nil {
		return nil, fmt.Errorf("failed to search provider: %w", err)
	}

	return results, nil
}

// DeleteMapping removes a mapping from the database
func (m *Manager) DeleteMapping(ctx context.Context, anilistID int) error {
	result := m.db.Where("ani_list_id = ?", anilistID).Delete(&database.AniListMapping{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete mapping: %w", result.Error)
	}
	return nil
}

// SelectMapping saves a user-selected mapping from search results
func (m *Manager) SelectMapping(ctx context.Context, anilistID int, providerName string, selected providers.Media) error {
	mapping := &ProviderMapping{
		AniListID:       anilistID,
		ProviderName:    providerName,
		ProviderMediaID: selected.ID,
		Media:           &selected,
		IsNew:           true,
	}

	return m.SaveMapping(ctx, mapping)
}
