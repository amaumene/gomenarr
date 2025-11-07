package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/amaumene/gomenarr/internal/models"
	"github.com/amaumene/gomenarr/internal/services/trakt"
	"github.com/sirupsen/logrus"
)

// SyncController handles synchronization with Trakt
type SyncController struct {
	db          *models.Database
	traktClient *trakt.Client
	cleanupCtrl *CleanupController
	logger      *logrus.Logger
}

// NewSyncController creates a new sync controller
func NewSyncController(db *models.Database, traktClient *trakt.Client, cleanupCtrl *CleanupController, logger *logrus.Logger) *SyncController {
	return &SyncController{
		db:          db,
		traktClient: traktClient,
		cleanupCtrl: cleanupCtrl,
		logger:      logger,
	}
}

// SyncAll synchronizes all data from Trakt
func (c *SyncController) SyncAll(ctx context.Context) error {
	c.logger.Info("Starting Trakt sync")

	// Step 1: Mark ALL existing medias as NOT in Trakt
	if err := c.db.MarkAllMediasNotInTrakt(); err != nil {
		c.logger.WithError(err).Error("Failed to mark medias as not in Trakt, skipping cleanup")
		// Don't return error, continue with sync but skip cleanup
	}

	syncFailed := false

	// Step 2: Sync favorites (TV shows)
	if err := c.syncFavorites(ctx, "shows"); err != nil {
		c.logger.WithError(err).Error("Failed to sync TV favorites")
		syncFailed = true
	}

	// Step 3: Sync favorites (movies)
	if err := c.syncFavorites(ctx, "movies"); err != nil {
		c.logger.WithError(err).Error("Failed to sync movie favorites")
		syncFailed = true
	}

	// Step 4: Sync watchlist (TV shows)
	if err := c.syncWatchlist(ctx, "shows"); err != nil {
		c.logger.WithError(err).Error("Failed to sync TV watchlist")
		syncFailed = true
	}

	// Step 5: Sync watchlist (movies)
	if err := c.syncWatchlist(ctx, "movies"); err != nil {
		c.logger.WithError(err).Error("Failed to sync movie watchlist")
		syncFailed = true
	}

	// Step 6: Sync watched status
	if err := c.syncWatched(ctx); err != nil {
		c.logger.WithError(err).Error("Failed to sync watched status")
		syncFailed = true
	}

	// Step 7: Update episode watched status in season packs
	if err := c.updateEpisodeWatchedStatus(ctx); err != nil {
		c.logger.WithError(err).Error("Failed to update episode watched status")
	}

	// Step 8: IMMEDIATELY trigger cleanup of removed items (only if sync succeeded)
	if !syncFailed {
		if err := c.cleanupCtrl.CleanupRemovedFromTrakt(ctx); err != nil {
			c.logger.WithError(err).Error("Failed to cleanup removed items")
		}
	} else {
		c.logger.Warn("Skipping cleanup due to sync failures")
	}

	c.logger.Info("Trakt sync completed")
	return nil
}

// syncFavorites syncs favorites from Trakt
func (c *SyncController) syncFavorites(ctx context.Context, mediaType string) error {
	c.logger.WithField("type", mediaType).Info("Syncing favorites")

	items, err := c.traktClient.GetFavorites(ctx, mediaType)
	if err != nil {
		return fmt.Errorf("failed to get favorites: %w", err)
	}

	c.logger.WithField("count", len(items)).Debug("Retrieved favorites")

	for _, item := range items {
		var imdbID string
		var title string
		var year int
		var mType models.MediaType

		if mediaType == "movies" && item.Movie != nil {
			imdbID = item.Movie.IDs.IMDB
			title = item.Movie.Title
			year = item.Movie.Year
			mType = models.MediaTypeMovie
		} else if mediaType == "shows" && item.Show != nil {
			imdbID = item.Show.IDs.IMDB
			title = item.Show.Title
			year = item.Show.Year
			mType = models.MediaTypeTV
		} else {
			continue
		}

		if imdbID == "" {
			c.logger.WithField("title", title).Warn("Missing IMDB ID, skipping")
			continue
		}

		// Check if media already exists
		existingMedia, err := c.db.GetMediaByIMDBID(imdbID, mType, nil, nil)
		if err == nil {
			// Update existing media
			existingMedia.IMDBId = imdbID
			existingMedia.InTrakt = true
			existingMedia.LastSeenInTrakt = time.Now()
			existingMedia.Source = models.SourceFavorites

			// Do NOT reset completed downloads - we don't want to re-download them!
			// Only reset failed downloads to give them another chance
			if existingMedia.Status == models.StatusFailed {
				existingMedia.Status = models.StatusPending
				c.logger.WithFields(logrus.Fields{
					"title":      title,
					"old_status": "failed",
				}).Debug("Resetting failed media status to pending for retry")
			}

			if err := c.db.UpdateMedia(existingMedia); err != nil {
				c.logger.WithError(err).Error("Failed to update media")
			}
		} else {
			// Create new media
			media := &models.Media{
				IMDBId:          imdbID,
				MediaType:       mType,
				Title:           title,
				Year:            year,
				Source:          models.SourceFavorites,
				Status:          models.StatusPending,
				Watched:         false,
				InTrakt:         true,
				LastSeenInTrakt: time.Now(),
			}

			if err := c.db.CreateMedia(media); err != nil {
				c.logger.WithError(err).Error("Failed to create media")
			} else {
				c.logger.WithFields(logrus.Fields{
					"title": title,
					"type":  mType,
				}).Info("Added new media from favorites")
			}
		}
	}

	return nil
}

// syncWatchlist syncs watchlist from Trakt
func (c *SyncController) syncWatchlist(ctx context.Context, mediaType string) error {
	c.logger.WithField("type", mediaType).Info("Syncing watchlist")

	items, err := c.traktClient.GetWatchlist(ctx, mediaType)
	if err != nil {
		return fmt.Errorf("failed to get watchlist: %w", err)
	}

	c.logger.WithField("count", len(items)).Debug("Retrieved watchlist")

	for _, item := range items {
		var imdbID string
		var title string
		var year int
		var mType models.MediaType

		if mediaType == "movies" && item.Movie != nil {
			imdbID = item.Movie.IDs.IMDB
			title = item.Movie.Title
			year = item.Movie.Year
			mType = models.MediaTypeMovie
		} else if mediaType == "shows" && item.Show != nil {
			imdbID = item.Show.IDs.IMDB
			title = item.Show.Title
			year = item.Show.Year
			mType = models.MediaTypeTV
		} else {
			continue
		}

		if imdbID == "" {
			c.logger.WithField("title", title).Warn("Missing IMDB ID, skipping")
			continue
		}

		// Check if media already exists
		existingMedia, err := c.db.GetMediaByIMDBID(imdbID, mType, nil, nil)
		if err == nil {
			// Update existing media
			existingMedia.IMDBId = imdbID
			existingMedia.InTrakt = true
			existingMedia.LastSeenInTrakt = time.Now()
			existingMedia.Source = models.SourceWatchlist

			// Do NOT reset completed downloads - we don't want to re-download them!
			// Only reset failed downloads to give them another chance
			if existingMedia.Status == models.StatusFailed {
				existingMedia.Status = models.StatusPending
				c.logger.WithFields(logrus.Fields{
					"title":      title,
					"old_status": "failed",
				}).Debug("Resetting failed media status to pending for retry")
			}

			if err := c.db.UpdateMedia(existingMedia); err != nil {
				c.logger.WithError(err).Error("Failed to update media")
			}
		} else {
			// Create new media
			media := &models.Media{
				IMDBId:          imdbID,
				MediaType:       mType,
				Title:           title,
				Year:            year,
				Source:          models.SourceWatchlist,
				Status:          models.StatusPending,
				Watched:         false,
				InTrakt:         true,
				LastSeenInTrakt: time.Now(),
			}

			if err := c.db.CreateMedia(media); err != nil {
				c.logger.WithError(err).Error("Failed to create media")
			} else {
				c.logger.WithFields(logrus.Fields{
					"title": title,
					"type":  mType,
				}).Info("Added new media from watchlist")
			}
		}
	}

	return nil
}

// syncWatched syncs watched status from Trakt
func (c *SyncController) syncWatched(ctx context.Context) error {
	c.logger.Info("Syncing watched status")

	// Get watched items from last 3 days (configurable)
	items, err := c.traktClient.GetRecentlyWatched(ctx, 3)
	if err != nil {
		return fmt.Errorf("failed to get watched items: %w", err)
	}

	c.logger.WithField("count", len(items)).Debug("Retrieved watched items")

	for _, item := range items {
		if item.MediaType == "movie" {
			media, err := c.db.GetMediaByIMDBID(item.IMDBId, models.MediaTypeMovie, nil, nil)
			if err == nil {
				media.Watched = true
				c.db.UpdateMedia(media)
			}
		}
		// Episode watched status is handled in updateEpisodeWatchedStatus
	}

	return nil
}

// updateEpisodeWatchedStatus updates watched status for episodes in season packs
func (c *SyncController) updateEpisodeWatchedStatus(ctx context.Context) error {
	c.logger.Info("Updating episode watched status")

	// Get recently watched episodes
	watchedItems, err := c.traktClient.GetRecentlyWatched(ctx, 3)
	if err != nil {
		return fmt.Errorf("failed to get watched items: %w", err)
	}

	// Get all medias
	allMedias, err := c.db.GetAllMedias()
	if err != nil {
		return err
	}

	// Update episode status in season packs
	for _, media := range allMedias {
		if media.MediaType != models.MediaTypeTV {
			continue
		}

		nzbs, err := c.db.GetNZBsByMediaID(media.ID)
		if err != nil {
			continue
		}

		for _, nzb := range nzbs {
			if !nzb.IsSeasonPack {
				continue
			}

			updated := false
			for _, watchedItem := range watchedItems {
				if watchedItem.MediaType != "episode" || watchedItem.IMDBId != media.IMDBId {
					continue
				}

				// Update episode watched status
				for i := range nzb.Episodes {
					if nzb.Episodes[i].EpisodeNumber == watchedItem.Episode {
						nzb.Episodes[i].Watched = true
						watchedAt := watchedItem.WatchedAt
						nzb.Episodes[i].WatchedAt = &watchedAt
						updated = true
					}
				}
			}

			if updated {
				if err := c.db.UpdateNZB(nzb); err != nil {
					c.logger.WithError(err).Error("Failed to update NZB")
				}
			}
		}
	}

	return nil
}
