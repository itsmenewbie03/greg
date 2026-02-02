package database

import (
	"errors"

	"gorm.io/gorm"
)

// GetAudioPreference retrieves per-show audio preference by AniList ID
// Returns empty string if no preference stored (not an error)
func GetAudioPreference(db *gorm.DB, anilistID int) (string, error) {
	var pref AudioPreference
	err := db.Where("anilist_id = ?", anilistID).First(&pref).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil // No preference stored - use config default
		}
		return "", err // Database error
	}
	return pref.Preference, nil
}

// SaveAudioPreference stores or updates per-show audio preference
// Upserts using GORM Save (updates if exists, inserts if new)
func SaveAudioPreference(db *gorm.DB, anilistID int, preference string, trackIndex *int) error {
	// Validate preference value
	if preference != "dub" && preference != "sub" {
		return errors.New("invalid audio preference: must be 'dub' or 'sub'")
	}

	pref := AudioPreference{
		AniListID:  anilistID,
		Preference: preference,
		TrackIndex: trackIndex, // Optional - for reference only
	}

	// GORM Save performs upsert based on unique index (anilist_id)
	return db.Save(&pref).Error
}

// ClearAudioPreference removes per-show audio preference
// Called when show marked complete/dropped per CONTEXT.md
func ClearAudioPreference(db *gorm.DB, anilistID int) error {
	result := db.Where("anilist_id = ?", anilistID).Delete(&AudioPreference{})
	if result.Error != nil {
		return result.Error
	}
	// Note: No error if record doesn't exist (idempotent)
	return nil
}

// ClearCompletedPreferences batch-deletes preferences for AniList IDs
// Used by tracker sync when shows marked complete/dropped
func ClearCompletedPreferences(db *gorm.DB, anilistIDs []int) error {
	if len(anilistIDs) == 0 {
		return nil
	}
	return db.Where("anilist_id IN ?", anilistIDs).Delete(&AudioPreference{}).Error
}
