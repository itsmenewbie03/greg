package database

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"github.com/justchokingaround/greg/internal/config"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the global database instance
var DB *gorm.DB

// Init initializes the database connection
func Init(cfg *config.DatabaseConfig) error {
	// Ensure directory exists
	dbDir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Configure GORM
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	// Open database connection
	db, err := gorm.Open(sqlite.Open(cfg.Path), gormConfig)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Get underlying SQL DB
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// Set connection pool settings
	sqlDB.SetMaxOpenConns(cfg.MaxConnections)
	sqlDB.SetMaxIdleConns(cfg.MaxConnections / 2)

	// Enable WAL mode for better concurrency
	if cfg.WALMode {
		if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
			return fmt.Errorf("failed to enable WAL mode: %w", err)
		}
	}

	// Enable foreign keys
	if err := db.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Auto vacuum
	if cfg.AutoVacuum {
		if err := db.Exec("PRAGMA auto_vacuum=INCREMENTAL").Error; err != nil {
			return fmt.Errorf("failed to enable auto vacuum: %w", err)
		}
	}

	// Run SQL migrations first (for backwards compatibility with existing databases)
	if err := RunMigrations(db); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Close and re-open connection to clear GORM's schema cache
	// This ensures GORM picks up the new columns from migrations
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close database connection: %w", err)
	}

	// Re-open with fresh GORM connection
	db, err = gorm.Open(sqlite.Open(cfg.Path), gormConfig)
	if err != nil {
		return fmt.Errorf("failed to re-open database: %w", err)
	}

	sqlDB, err = db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// Re-apply settings
	sqlDB.SetMaxOpenConns(cfg.MaxConnections)
	sqlDB.SetMaxIdleConns(cfg.MaxConnections / 2)

	if cfg.WALMode {
		if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
			return fmt.Errorf("failed to enable WAL mode: %w", err)
		}
	}

	if err := db.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if cfg.AutoVacuum {
		if err := db.Exec("PRAGMA auto_vacuum=INCREMENTAL").Error; err != nil {
			return fmt.Errorf("failed to enable auto vacuum: %w", err)
		}
	}

	// Run GORM AutoMigrate for schema changes
	if err := Migrate(db); err != nil {
		return fmt.Errorf("failed to run auto migrations: %w\n\n"+
			"Hint: If you have an old database from a previous version, delete it and restart:\n"+
			"  Linux/macOS: rm -f ~/.local/share/greg/greg.db\n"+
			"  Windows:    del \"%%USERPROFILE%%\\.local\\share\\greg\\greg.db\"\n"+
			"              OR in PowerShell: Remove-Item \"$env:USERPROFILE\\.local\\share\\greg\\greg.db\"", err)
	}

	DB = db
	return nil
}

// Close closes the database connection
func Close() error {
	if DB == nil {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

// GetDB returns the database instance
func GetDB() *gorm.DB {
	return DB
}
