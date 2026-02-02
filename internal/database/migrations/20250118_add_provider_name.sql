-- Migration: Add provider_name column to history table
-- Date: 2025-01-18
-- Description: Add ProviderName field to History model for tracking which provider was used for playback
-- Compatible with: All existing databases

-- Create a new table with the updated schema (without foreign key constraints - SQLite limitation)
CREATE TABLE IF NOT EXISTS history_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id TEXT NOT NULL,
    media_title TEXT NOT NULL,
    media_type TEXT NOT NULL,
    episode INTEGER DEFAULT 0,
    season INTEGER DEFAULT 0,
    page INTEGER DEFAULT 0,
    total_pages INTEGER DEFAULT 0,
    progress_seconds INTEGER NOT NULL,
    total_seconds INTEGER NOT NULL,
    progress_percent REAL NOT NULL,
    watched_at TEXT DEFAULT CURRENT_TIMESTAMP,
    completed BOOLEAN DEFAULT 0,
    anilist_id INTEGER,
    provider_name TEXT DEFAULT ''
);

-- Copy data from old table (provider_name will be empty string for existing rows)
INSERT INTO history_new (id, media_id, media_title, media_type, episode, season, page, total_pages, progress_seconds, total_seconds, progress_percent, watched_at, completed, anilist_id, provider_name)
SELECT id, media_id, media_title, media_type, episode, season, page, total_pages, progress_seconds, total_seconds, progress_percent, watched_at, completed, anilist_id, ''
FROM history;

-- Drop the old table
DROP TABLE history;

-- Rename new table to original name
ALTER TABLE history_new RENAME TO history;

-- Recreate indexes
CREATE INDEX IF NOT EXISTS idx_history_media_id ON history(media_id);
CREATE INDEX IF NOT EXISTS idx_history_media_type ON history(media_type);
CREATE INDEX IF NOT EXISTS idx_history_watched_at ON history(watched_at);
