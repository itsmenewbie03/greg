package anilist

import (
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

const (
	tokenSettingKey = "anilist_token"
)

// TokenStorage handles persisting OAuth2 tokens to the database
type TokenStorage struct {
	db *gorm.DB
}

// NewTokenStorage creates a new token storage instance
func NewTokenStorage(db *gorm.DB) *TokenStorage {
	return &TokenStorage{db: db}
}

// SaveToken saves an OAuth2 token to the database
func (s *TokenStorage) SaveToken(token *oauth2.Token) error {
	if token == nil {
		// Delete the token
		return s.db.Exec("DELETE FROM settings WHERE key = ?", tokenSettingKey).Error
	}

	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	// Use raw SQL to do an upsert
	return s.db.Exec(`
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`, tokenSettingKey, string(data), time.Now()).Error
}

// LoadToken loads an OAuth2 token from the database
func (s *TokenStorage) LoadToken() (*oauth2.Token, error) {
	var value string
	err := s.db.Raw("SELECT value FROM settings WHERE key = ?", tokenSettingKey).Scan(&value).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load token: %w", err)
	}

	if value == "" {
		return nil, nil
	}

	var token oauth2.Token
	if err := json.Unmarshal([]byte(value), &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	return &token, nil
}
