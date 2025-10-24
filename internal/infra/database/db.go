package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/amaumene/gomenarr/internal/core/domain"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// New creates a new database connection
func New(cfg config.DatabaseConfig) (*gorm.DB, error) {
	// Log database initialization
	log.Info().Str("path", cfg.Path).Msg("Initializing database")

	// Ensure parent directory exists before opening database
	dbDir := filepath.Dir(cfg.Path)
	log.Debug().Str("dir", dbDir).Msg("Creating database directory")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}
	log.Debug().Msg("Database directory created successfully")

	// Configure SQLite connection string
	dsn := cfg.Path
	if cfg.WALMode {
		dsn += "?_journal_mode=WAL"
	}

	// Open database
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Get underlying SQL DB
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying database: %w", err)
	}

	// Set connection pool settings
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Auto-migrate schemas
	log.Info().Msg("Running database auto-migration")
	if err := db.AutoMigrate(&domain.Media{}, &domain.NZB{}); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate: %w", err)
	}
	log.Info().Str("path", cfg.Path).Msg("Database initialized successfully")

	return db, nil
}

// HealthCheck checks if the database is healthy
func HealthCheck(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	return sqlDB.PingContext(ctx)
}
