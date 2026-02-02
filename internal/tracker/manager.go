package tracker

import (
	"context"
	"fmt"
	"sync"

	"github.com/justchokingaround/greg/internal/config"
	"github.com/justchokingaround/greg/internal/providers"
	"gorm.io/gorm"
)

// Manager handles tracker instances and operations
type Manager struct {
	anilist Tracker
	cfg     *config.Config
	db      *gorm.DB
	mu      sync.RWMutex
}

// NewManager creates a new tracker manager
func NewManager(cfg *config.Config, db *gorm.DB) *Manager {
	return &Manager{
		cfg: cfg,
		db:  db,
	}
}

// SetAniListClient sets the AniList tracker implementation
func (m *Manager) SetAniListClient(client Tracker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.anilist = client
}

// GetAniList returns the AniList tracker
func (m *Manager) GetAniList() Tracker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.anilist
}

// IsAniListEnabled checks if AniList tracking is enabled
func (m *Manager) IsAniListEnabled() bool {
	return m.cfg.Tracker.AniList.Enabled
}

// IsAniListAuthenticated checks if AniList is authenticated
func (m *Manager) IsAniListAuthenticated() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.anilist != nil && m.anilist.IsAuthenticated()
}

// UpdateProgress updates progress on all enabled trackers
func (m *Manager) UpdateProgress(ctx context.Context, mediaID string, episode int, progress float64) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.anilist != nil && m.cfg.Tracker.AniList.Enabled && m.cfg.Tracker.AniList.AutoSync {
		// Check if we should sync based on threshold
		if progress >= m.cfg.Tracker.AniList.SyncThreshold {
			if err := m.anilist.UpdateProgress(ctx, mediaID, episode, progress); err != nil {
				// Don't fail the operation if sync fails, just log it
				return fmt.Errorf("anilist sync failed: %w", err)
			}
		} else {
			// Queue for later sync
			if err := m.queueSync(mediaID, episode, progress); err != nil {
				return fmt.Errorf("failed to queue sync: %w", err)
			}
		}
	}

	return nil
}

// queueSync adds an item to the sync queue
func (m *Manager) queueSync(mediaID string, episode int, progress float64) error {
	// Insert into sync_queue table
	return m.db.Exec(`
		INSERT INTO sync_queue (media_id, episode, progress, synced, created_at)
		VALUES (?, ?, ?, false, CURRENT_TIMESTAMP)
		ON CONFLICT(media_id, episode) DO UPDATE SET
			progress = excluded.progress,
			synced = false
	`, mediaID, episode, progress).Error
}

// ProcessSyncQueue processes pending sync items
func (m *Manager) ProcessSyncQueue(ctx context.Context) error {
	m.mu.RLock()
	anilist := m.anilist
	m.mu.RUnlock()

	if anilist == nil || !m.cfg.Tracker.AniList.Enabled {
		return nil
	}

	// Get pending sync items
	var items []struct {
		ID       uint
		MediaID  string
		Episode  int
		Progress float64
	}

	if err := m.db.Raw(`
		SELECT id, media_id, episode, progress
		FROM sync_queue
		WHERE synced = false
		ORDER BY created_at ASC
		LIMIT 100
	`).Scan(&items).Error; err != nil {
		return fmt.Errorf("failed to get sync queue: %w", err)
	}

	for _, item := range items {
		if err := anilist.UpdateProgress(ctx, item.MediaID, item.Episode, item.Progress); err != nil {
			// Mark as failed but continue
			continue
		}

		// Mark as synced
		if err := m.db.Exec(`
			UPDATE sync_queue
			SET synced = true, synced_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, item.ID).Error; err != nil {
			// Log but continue
			continue
		}
	}

	return nil
}

// SearchMedia searches for media on enabled trackers
func (m *Manager) SearchMedia(ctx context.Context, query string, mediaType providers.MediaType) ([]TrackedMedia, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.anilist != nil && m.cfg.Tracker.AniList.Enabled {
		return m.anilist.SearchMedia(ctx, query, mediaType)
	}

	return nil, fmt.Errorf("no trackers enabled")
}

// DeleteFromList removes a media from the tracker
func (m *Manager) DeleteFromList(ctx context.Context, mediaListID int) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.anilist != nil && m.cfg.Tracker.AniList.Enabled {
		return m.anilist.DeleteFromList(ctx, mediaListID)
	}

	return fmt.Errorf("no trackers enabled")
}

// GetUserLibrary retrieves the user's library from enabled trackers
func (m *Manager) GetUserLibrary(ctx context.Context, mediaType providers.MediaType) ([]TrackedMedia, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.anilist != nil && m.cfg.Tracker.AniList.Enabled {
		return m.anilist.GetUserLibrary(ctx, mediaType)
	}

	return nil, fmt.Errorf("no trackers enabled")
}
