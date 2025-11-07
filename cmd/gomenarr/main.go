package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/amaumene/gomenarr/internal/api"
	"github.com/amaumene/gomenarr/internal/config"
	"github.com/amaumene/gomenarr/internal/controllers"
	"github.com/amaumene/gomenarr/internal/models"
	"github.com/amaumene/gomenarr/internal/scheduler"
	"github.com/amaumene/gomenarr/internal/services/newznab"
	"github.com/amaumene/gomenarr/internal/services/torbox"
	"github.com/amaumene/gomenarr/internal/services/trakt"
	"github.com/amaumene/gomenarr/internal/utils"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// 2. Setup logger
	logger := utils.NewLogger(cfg.LogLevel)
	logger.Info("Starting Gomenarr")
	logger.WithField("config_dir", filepath.Dir(cfg.DatabaseFile)).Info("Configuration loaded")

	// 3. Initialize database
	db, err := models.NewDatabase(cfg.DatabaseFile)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer db.Close()
	logger.Info("Database initialized")

	// 4. Load blacklist
	blacklist, err := utils.LoadBlacklist(cfg.BlacklistFile)
	if err != nil {
		logger.WithError(err).Warn("Failed to load blacklist, continuing without it")
		blacklist = &utils.Blacklist{}
	} else {
		logger.Info("Blacklist loaded")
	}

	// 5. Initialize services
	traktClient, err := trakt.NewClient(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize Trakt client: %w", err)
	}
	logger.Info("Trakt client initialized")

	// Check if we need to authenticate
	_, err = traktClient.GetToken()
	if err != nil {
		logger.Info("Trakt authentication required")
		ctx := context.Background()
		if err := traktClient.Authenticate(ctx); err != nil {
			return fmt.Errorf("failed to authenticate with Trakt: %w", err)
		}
	}

	newznabClient, err := newznab.NewClient(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize Newznab client: %w", err)
	}
	logger.Info("Newznab client initialized")

	torboxClient, err := torbox.NewClient(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize TorBox client: %w", err)
	}
	logger.Info("TorBox client initialized")

	// 6. Initialize controllers
	cleanupCtrl := controllers.NewCleanupController(db, torboxClient, traktClient, cfg.TraktSyncDays, logger)
	syncCtrl := controllers.NewSyncController(db, traktClient, cleanupCtrl, logger)
	strategyCtrl := controllers.NewStrategyController(db, traktClient, logger)
	searchCtrl := controllers.NewSearchController(db, newznabClient, traktClient, blacklist, logger)
	downloadCtrl := controllers.NewDownloadController(db, torboxClient, newznabClient, logger)
	logger.Info("Controllers initialized")

	// 7. Initialize scheduler
	sched := scheduler.NewScheduler(syncCtrl, strategyCtrl, searchCtrl, downloadCtrl, cleanupCtrl, db, cfg.DownloadTimeoutMinutes, logger)
	if err := sched.Start(); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}
	defer sched.Stop()

	// 8. Initialize HTTP server
	server := api.NewServer(cfg, db, downloadCtrl, logger)

	// Start server in goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErrChan := make(chan error, 1)
	go func() {
		if err := server.Start(ctx); err != nil {
			serverErrChan <- err
		}
	}()

	// 9. Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	logger.Info("Gomenarr is running")

	select {
	case err := <-serverErrChan:
		return fmt.Errorf("server error: %w", err)
	case sig := <-sigChan:
		logger.WithField("signal", sig).Info("Received shutdown signal")
		cancel()
		if err := server.Shutdown(context.Background()); err != nil {
			logger.WithError(err).Error("Error during server shutdown")
		}
	}

	logger.Info("Gomenarr stopped")
	return nil
}
