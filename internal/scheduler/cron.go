package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/amaumene/gomenarr/internal/controllers"
	"github.com/amaumene/gomenarr/internal/models"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
)

// Scheduler manages scheduled tasks
type Scheduler struct {
	cron                   *cron.Cron
	syncCtrl               *controllers.SyncController
	strategyCtrl           *controllers.StrategyController
	searchCtrl             *controllers.SearchController
	downloadCtrl           *controllers.DownloadController
	cleanupCtrl            *controllers.CleanupController
	db                     *models.Database
	logger                 *logrus.Logger
	downloadTimeoutMinutes int
}

// NewScheduler creates a new scheduler
func NewScheduler(
	syncCtrl *controllers.SyncController,
	strategyCtrl *controllers.StrategyController,
	searchCtrl *controllers.SearchController,
	downloadCtrl *controllers.DownloadController,
	cleanupCtrl *controllers.CleanupController,
	db *models.Database,
	downloadTimeoutMinutes int,
	logger *logrus.Logger,
) *Scheduler {
	return &Scheduler{
		cron:                   cron.New(),
		syncCtrl:               syncCtrl,
		strategyCtrl:           strategyCtrl,
		searchCtrl:             searchCtrl,
		downloadCtrl:           downloadCtrl,
		cleanupCtrl:            cleanupCtrl,
		db:                     db,
		downloadTimeoutMinutes: downloadTimeoutMinutes,
		logger:                 logger,
	}
}

// Start starts the scheduler
func (s *Scheduler) Start() error {
	s.logger.Info("Starting scheduler")

	// Every 6 hours: Sync from Trakt (also triggers immediate cleanup of removed items)
	_, err := s.cron.AddFunc("0 */6 * * *", func() {
		s.runSync()
	})
	if err != nil {
		return fmt.Errorf("failed to add sync job: %w", err)
	}

	// Every 30 minutes: Process pending medias (search + download)
	_, err = s.cron.AddFunc("*/30 * * * *", func() {
		s.runSearch()
	})
	if err != nil {
		return fmt.Errorf("failed to add search job: %w", err)
	}

	// Every hour: Cleanup watched medias
	_, err = s.cron.AddFunc("0 * * * *", func() {
		s.runCleanupWatched()
	})
	if err != nil {
		return fmt.Errorf("failed to add cleanup job: %w", err)
	}

	// Every 10 minutes: Check for stuck downloads
	_, err = s.cron.AddFunc("*/10 * * * *", func() {
		s.runStuckDownloadCheck()
	})
	if err != nil {
		return fmt.Errorf("failed to add stuck download check job: %w", err)
	}

	s.cron.Start()
	s.logger.Info("Scheduler started")

	// Run initial sync and search immediately
	go func() {
		s.runSync()
		// Wait a bit for sync to complete, then run search
		s.logger.Info("Running initial search after sync")
		s.runSearch()
	}()

	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.logger.Info("Stopping scheduler")
	s.cron.Stop()
}

// runSync executes the sync job
func (s *Scheduler) runSync() {
	s.logger.Info("Running scheduled sync")
	ctx := context.Background()

	if err := s.syncCtrl.SyncAll(ctx); err != nil {
		s.logger.WithError(err).Error("Sync job failed")
	} else {
		s.logger.Info("Sync job completed successfully")
	}
}

// runSearch executes the search and download job
func (s *Scheduler) runSearch() {
	s.logger.Info("Running scheduled search")
	ctx := context.Background()

	// Get pending medias
	medias, err := s.db.GetPendingMedias()
	if err != nil {
		s.logger.WithError(err).Error("Failed to get pending medias")
		return
	}

	if len(medias) == 0 {
		s.logger.Debug("No pending medias to process")
		return
	}

	s.logger.WithField("count", len(medias)).Info("Processing pending medias")

	for _, media := range medias {
		s.logger.WithFields(logrus.Fields{
			"media_id": media.ID,
			"title":    media.Title,
		}).Info("Processing media")

		// Update status to searching
		media.Status = models.StatusSearching
		if err := s.db.UpdateMedia(media); err != nil {
			s.logger.WithError(err).Error("Failed to update media status")
			continue
		}

		// Determine strategy
		strategy, err := s.strategyCtrl.DetermineStrategy(ctx, media)
		if err != nil {
			s.logger.WithError(err).Error("Failed to determine strategy")
			media.Status = models.StatusFailed
			s.db.UpdateMedia(media)
			continue
		}

		// Search for media
		nzbs, err := s.searchCtrl.SearchMedia(ctx, media, strategy)
		if err != nil {
			s.logger.WithError(err).Error("Search failed")
			media.Status = models.StatusFailed
			s.db.UpdateMedia(media)
			continue
		}

		if len(nzbs) == 0 {
			s.logger.Warn("No results found")
			media.Status = models.StatusPending // Keep as pending to retry later
			s.db.UpdateMedia(media)
			continue
		}

		// Find all selected NZBs and download them
		var selectedNZBs []*models.NZB
		for _, nzb := range nzbs {
			if nzb.Status == models.NZBStatusSelected {
				selectedNZBs = append(selectedNZBs, nzb)
			}
		}

		if len(selectedNZBs) == 0 {
			s.logger.Warn("No suitable NZB found (all blacklisted?)")
			media.Status = models.StatusFailed
			s.db.UpdateMedia(media)
			continue
		}

		s.logger.WithFields(logrus.Fields{
			"media_id": media.ID,
			"count":    len(selectedNZBs),
		}).Info("Found selected NZBs to download")

		// Download all selected NZBs
		downloadFailed := false
		for _, nzb := range selectedNZBs {
			s.logger.WithFields(logrus.Fields{
				"nzb_id":  nzb.ID,
				"title":   nzb.Title,
				"episode": nzb.Episode,
			}).Info("Downloading NZB")

			if err := s.downloadCtrl.DownloadNZB(nzb); err != nil {
				s.logger.WithError(err).Error("Download failed")
				downloadFailed = true
				// Continue with other downloads instead of stopping
			}
		}

		// Only mark as failed if ALL downloads failed
		if downloadFailed && len(selectedNZBs) == 1 {
			media.Status = models.StatusFailed
			s.db.UpdateMedia(media)
			continue
		}

		s.logger.WithFields(logrus.Fields{
			"media_id": media.ID,
			"count":    len(selectedNZBs),
		}).Info("Media downloads started")
	}

	s.logger.Info("Search job completed")
}

// runCleanupWatched executes the watched cleanup job
func (s *Scheduler) runCleanupWatched() {
	s.logger.Info("Running scheduled cleanup of watched content")
	ctx := context.Background()

	if err := s.cleanupCtrl.CleanupWatched(ctx); err != nil {
		s.logger.WithError(err).Error("Cleanup job failed")
	} else {
		s.logger.Info("Cleanup job completed successfully")
	}
}

// runStuckDownloadCheck executes the stuck download check job
func (s *Scheduler) runStuckDownloadCheck() {
	s.logger.Debug("Running stuck download check")

	timeout := time.Duration(s.downloadTimeoutMinutes) * time.Minute
	if err := s.downloadCtrl.CheckStuckDownloads(timeout); err != nil {
		s.logger.WithError(err).Error("Stuck download check failed")
	}
}
