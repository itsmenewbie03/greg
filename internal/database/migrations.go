package database

import (
	"embed"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migration represents a database migration
type migration struct {
	filename string
	name     string
	sql      string
}

// RunMigrations runs all pending database migrations
func RunMigrations(db *gorm.DB) error {
	// Create migrations tracking table if it doesn't exist
	if err := createMigrationsTable(db); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get all migration files
	migrations, err := getMigrations()
	if err != nil {
		return fmt.Errorf("failed to get migrations: %w", err)
	}

	// Get already applied migrations
	applied, err := getAppliedMigrations(db)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Run pending migrations
	for _, m := range migrations {
		if applied[m.name] {
			continue // Already applied
		}

		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", m.filename, err)
		}
	}

	return nil
}

// createMigrationsTable creates the schema_migrations table if it doesn't exist
func createMigrationsTable(db *gorm.DB) error {
	return db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`).Error
}

// getMigrations reads all migration files from the migrations directory
func getMigrations() ([]migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		content, err := migrationsFS.ReadFile(path.Join("migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration %s: %w", entry.Name(), err)
		}

		name := extractMigrationName(entry.Name())
		migrations = append(migrations, migration{
			filename: entry.Name(),
			name:     name,
			sql:      string(content),
		})
	}

	// Sort migrations by name (date) to ensure order
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].name < migrations[j].name
	})

	return migrations, nil
}

// extractMigrationName extracts the migration name from filename
// Expected format: YYYYMMDD_description.sql
func extractMigrationName(filename string) string {
	re := regexp.MustCompile(`^(\d{8})_.+\.sql$`)
	matches := re.FindStringSubmatch(filename)
	if len(matches) < 2 {
		return filename
	}
	return matches[1]
}

// getAppliedMigrations returns a map of already applied migration names
func getAppliedMigrations(db *gorm.DB) (map[string]bool, error) {
	var rows []struct {
		Name string `gorm:"column:name"`
	}

	if err := db.Table("schema_migrations").Pluck("name", &rows).Error; err != nil {
		return nil, err
	}

	applied := make(map[string]bool)
	for _, row := range rows {
		applied[row.Name] = true
	}

	return applied, nil
}

// applyMigration runs a single migration and records it
func applyMigration(db *gorm.DB, m migration) error {
	// Begin transaction
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	// Check if this migration is for an existing table (skip if table doesn't exist)
	// This handles the case where migrations run before GORM creates the table
	if err := checkMigrationPrerequisites(tx, m); err != nil {
		tx.Rollback()
		// If prerequisites not met, mark migration as applied anyway (for fresh databases)
		// This prevents re-running the migration on next startup
		if ignoreErr := db.Exec("INSERT OR IGNORE INTO schema_migrations (name) VALUES (?)", m.name).Error; ignoreErr != nil {
			return fmt.Errorf("failed to record skipped migration %s: %w", m.filename, ignoreErr)
		}
		return nil
	}

	// Execute migration SQL
	if err := tx.Exec(m.sql).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Record the migration
	if err := tx.Exec("INSERT INTO schema_migrations (name) VALUES (?)", m.name).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Commit transaction
	return tx.Commit().Error
}

// checkMigrationPrerequisites checks if the migration can be applied
// Returns error if prerequisites are not met (e.g., table doesn't exist)
func checkMigrationPrerequisites(db *gorm.DB, m migration) error {
	var count int64
	var err error

	// Check if history table exists
	if err = db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='history'").Scan(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return errors.New("history table does not exist yet")
	}

	// Check if provider_name column already exists
	if err = db.Raw("SELECT COUNT(*) FROM pragma_table_info('history') WHERE name='provider_name'").Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("provider_name column already exists")
	}

	return nil
}
